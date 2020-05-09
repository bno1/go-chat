package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const MAX_MESSAGEBUFFER_SIZE = 4096
const JANITOR_PERIOD = 10 * time.Second

type PreparedMessage struct {
	sourceIP    net.IP
	originalMsg interface{}
	msg         *websocket.PreparedMessage
}

type Client struct {
	username string
	conn     *websocket.Conn

	realIP net.IP

	closed uint32

	writeChan  chan *websocket.PreparedMessage
	readCursor uint64

	configHandle VersionedBoxHandle

	tokenBucket     TokenBucket
	lastMessageTime time.Time

	onceClose sync.Once
}

type ServerContext struct {
	configBox     *VersionedBox // live configuration
	messageBuffer MessageBuffer // buffer for broadcast messages

	ipBansHandle VersionedBoxHandle // ip bans

	usernamesLock sync.RWMutex
	users         map[string]*Client // connected users
	usernameList  []string           // list of usernames

	nClients      uint32 // number active clients
	nStaleClients uint32 // number of clients that have to be removed

	configLock *sync.RWMutex
	configCond *sync.Cond

	logConfig LogConfiguration
}

func NewServerContext(
	configBox *VersionedBox,
	ipBansHandle VersionedBoxHandle,
) ServerContext {
	configLock := sync.RWMutex{}

	return ServerContext{
		configBox:     configBox,
		messageBuffer: NewMessageBuffer(MAX_MESSAGEBUFFER_SIZE),
		ipBansHandle:  ipBansHandle,
		users:         make(map[string]*Client, 128),
		usernameList:  make([]string, 0),
		configLock:    &configLock,
		configCond:    sync.NewCond(configLock.RLocker()),
		logConfig:     NewLogConfiguration(configBox.GetHandle()),
	}
}

func (ctx *ServerContext) NotifyConfigUpdate() {
	ctx.configCond.Broadcast()
	ctx.logConfig.Update()
}

func (ctx *ServerContext) refreshUserList() {
	ctx.usernameList = ctx.usernameList[:0]

	for k := range ctx.users {
		ctx.usernameList = append(ctx.usernameList, k)
	}
}

func (ctx *ServerContext) GetStats() ([]byte, error) {
	msg := HelloMessage{
		UserCount: atomic.LoadUint32(&ctx.nClients),
	}

	return json.Marshal(&msg)
}

func (ctx *ServerContext) connectClient(
	client *Client,
	username string,
) (error, bool) {
	ctx.usernamesLock.Lock()

	if ctx.checkBan(client) {
		ctx.usernamesLock.Unlock()
		return fmt.Errorf("You have been banned"), false
	}

	_, present := ctx.users[username]

	if present {
		ctx.usernamesLock.Unlock()
		return fmt.Errorf("Username already used"), true
	}

	client.username = username
	ctx.users[username] = client

	ctx.refreshUserList()

	userCount := atomic.AddUint32(&ctx.nClients, 1)

	emsg, err := NewUserChangeMessage(
		client.username, "connect", userCount, ctx.usernameList)
	if err != nil {
		atomic.AddUint32(&ctx.nClients, ^uint32(0))
		delete(ctx.users, username)
		ctx.refreshUserList()
		ctx.usernamesLock.Unlock()

		return err, false
	}

	ctx.usernamesLock.Unlock()

	ctx.broadcastMessage(client, emsg)
	return nil, false
}

func (ctx *ServerContext) disconnectClient(
	client *Client,
	reason string,
	lockUserMap bool,
) error {
	var err error = nil
	var emsg EncodedMessage
	executed := false

	client.onceClose.Do(func() {
		if lockUserMap {
			ctx.usernamesLock.Lock()
		}

		if len(client.username) == 0 {
			if lockUserMap {
				ctx.usernamesLock.Unlock()
			}
			return
		}

		atomic.AddUint32(&client.closed, 1)

		atomic.AddUint32(&ctx.nStaleClients, 1)

		delete(ctx.users, client.username)
		client.username = ""
		ctx.refreshUserList()

		userCount := atomic.AddUint32(&ctx.nClients, ^uint32(0))

		emsg, err = NewUserChangeMessage(
			client.username, reason, userCount, ctx.usernameList)

		if lockUserMap {
			ctx.usernamesLock.Unlock()
		}

		executed = true
	})

	if !executed {
		return nil
	}

	if err == nil {
		ctx.broadcastMessage(client, emsg)
	}

	return err
}

func (ctx *ServerContext) handleConnectionInit(
	client *Client,
	config *Config,
) (error, bool) {
	_, data, err := client.conn.ReadMessage()
	if err != nil {
		return err, false
	}

	_, v, err := ParseMessage(data, INIT_MESSAGE)
	if err != nil {
		return err, true
	}

	msg := v.(*InitMessage)

	if uint(len(msg.Username)) < config.MinUsernameLength {
		return fmt.Errorf("Username too short"), true
	}

	if uint(len(msg.Username)) > config.MaxUsernameLength {
		return fmt.Errorf("Username too long"), true
	}

	return ctx.connectClient(client, msg.Username)
}

func makePreparedMessage(data []byte) (*websocket.PreparedMessage, error) {
	return websocket.NewPreparedMessage(websocket.TextMessage, data)
}

func (ctx *ServerContext) broadcastMessage(
	source *Client,
	emsg EncodedMessage,
) error {
	pmsg, err := makePreparedMessage(emsg.msg)
	if err != nil {
		return err
	}

	ctx.messageBuffer.Put(PreparedMessage{
		sourceIP:    source.realIP,
		originalMsg: emsg.originalMsg,
		msg:         pmsg,
	})

	return nil
}

func (client *Client) writeMessage(data []byte) error {
	msg, err := makePreparedMessage(data)
	if err != nil {
		return err
	}

	client.writeChan <- msg

	return nil
}

func (client *Client) writeErrorMessage(message string) error {
	m, err := NewErrorMessage(message)
	if err != nil {
		return err
	}

	return client.writeMessage(m.msg)
}

func getAddrIP(addr net.Addr) (net.IP, error) {
	switch taddr := addr.(type) {
	case *net.IPNet:
		return taddr.IP, nil
	case *net.IPAddr:
		return taddr.IP, nil
	case *net.UDPAddr:
		return taddr.IP, nil
	case *net.TCPAddr:
		return taddr.IP, nil
	default:
		return nil, fmt.Errorf("Unknown address format \"%v\"", addr)
	}
}

func (client *Client) checkBan(ipBans *IPBans) bool {
	banned, err := ipBans.IsBanned(client.realIP)
	if err != nil {
		log.Printf("error: %v", err)
		return false
	}

	return banned
}

func (ctx *ServerContext) checkBan(client *Client) bool {
	// Make a copy
	ipBansHandle := ctx.ipBansHandle

	ipBansPtr, _ := ipBansHandle.GetValue()
	ipBans := ipBansPtr.(*IPBans)

	return client.checkBan(ipBans)
}

func (ctx *ServerContext) setupClient(client *Client) error {
	var err error

	// Check to discard connection early
	if ctx.checkBan(client) {
		client.writeErrorMessage("You have been banned")
		return fmt.Errorf("User banned")
	}

	client.configHandle = ctx.configBox.GetHandle()

	configPtr, _ := client.configHandle.GetValue()

	config := configPtr.(*Config)

	client.conn.SetReadLimit(int64(config.MaxSocketMessageLen))

	client.readCursor = ctx.messageBuffer.GetBacklogCursor(
		uint64(config.BacklogLength))

	client.tokenBucket, err = NewTokenBucket(
		config.MaxMessagesBurst,
		config.MaxMessagesBurst,
		config.MaxMessagesPerSec)
	if err != nil {
		return err
	}

	for {
		initErr, canRetry := ctx.handleConnectionInit(client, config)
		if initErr == nil {
			m, err := NewHelloMessage(atomic.LoadUint32(&ctx.nClients))
			if err != nil {
				return err
			}

			err = client.writeMessage(m.msg)
			if err != nil {
				return err
			}

			break
		} else if websocket.IsUnexpectedCloseError(initErr) {
			return initErr
		} else {
			client.writeErrorMessage(initErr.Error())

			if canRetry {
				continue
			} else {
				break
			}
		}
	}

	return nil
}

func (client *Client) updateConfig() {
	configPtr, changed := client.configHandle.GetValue()

	config := configPtr.(*Config)

	if changed {
		client.conn.SetReadLimit(int64(config.MaxSocketMessageLen))
		client.tokenBucket.UpdateParams(
			config.MaxMessagesBurst,
			config.MaxMessagesPerSec)
	}
}

func (ctx *ServerContext) handleSendMessage(
	client *Client,
	msg *IncomingMessage,
) {
	// Update bucket
	now := time.Now()
	client.tokenBucket.Update(now.Sub(client.lastMessageTime))
	client.lastMessageTime = now

	if client.tokenBucket.TryConsumeToken() {
		m, err := NewOutgoingMessage(client.username, msg.Message)
		if err == nil {
			err = ctx.broadcastMessage(client, m)
		}

		if err != nil {
			log.Printf("error: %v", err)
			client.writeErrorMessage("Failed to encode message")
		}
	} else {
		// User tries to send too fast
		client.writeErrorMessage(
			"Please wait before sending another message",
		)
	}
}

func (ctx *ServerContext) clientReader(client *Client) {
	client.lastMessageTime = time.Now()

	for {
		if atomic.LoadUint32(&client.closed) > 0 {
			break
		}

		client.updateConfig()

		_, msgData, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) {
				log.Printf("connection closed: %v", err)
			} else {
				log.Printf("error: %v", err)
			}

			break
		}

		msgType, msg, err := ParseMessage(msgData, "")
		if err != nil {
			client.writeErrorMessage(err.Error())
			continue
		}

		switch m := msg.(type) {
		case *IncomingMessage:
			ctx.handleSendMessage(client, m)

		default:
			client.writeErrorMessage(
				fmt.Sprintf("Cannot accept message of type \"%s\"", msgType))
		}
	}
}

func (ctx *ServerContext) clientWriter(client *Client) {
	for {
		pmsg, more := <-client.writeChan

		if !more {
			break
		}

		err := client.conn.WritePreparedMessage(pmsg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) {
				log.Printf("connection closed: %v", err)
				ctx.disconnectClient(client, "disconnect", true)
			} else {
				log.Printf("error: %v", err)
			}
		}
	}

	log.Printf("User \"%s\" disconnected, remote addr %v, local addr %v",
		client.username, client.realIP, client.conn.LocalAddr())

	client.conn.Close()
}

func (ctx *ServerContext) clientListener(client *Client) {
	for {
		cursor, msg := ctx.messageBuffer.Get(
			client.readCursor, &client.closed)

		if atomic.LoadUint32(&client.closed) > 0 {
			break
		}

		client.writeChan <- msg.(PreparedMessage).msg

		client.readCursor = cursor + 1
	}

	close(client.writeChan)
}

func (ctx *ServerContext) HandleConnection(ws *websocket.Conn, realIP net.IP) {
	var client Client

	client.conn = ws
	client.writeChan = make(chan *websocket.PreparedMessage, 10)
	defer ctx.disconnectClient(&client, "disconnect", true)

	if realIP != nil {
		client.realIP = realIP
	} else {
		realIP, err := getAddrIP(ws.RemoteAddr())
		if err != nil {
			log.Printf("error: %v", err)
			ws.Close()
			return
		}

		if realIP == nil {
			log.Printf("Failed to parse IP: %v", ws.RemoteAddr())
			ws.Close()
			return
		}

		client.realIP = realIP
	}

	// the connection will be closed by ctx.clientWriter
	// the channel will be closed by ctx.clientListener
	go ctx.clientWriter(&client)

	err := ctx.setupClient(&client)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	log.Printf("User \"%s\" connected, remote addr %v, local addr %v",
		client.username, client.realIP, ws.LocalAddr())

	go ctx.clientListener(&client)

	ctx.clientReader(&client)
}

func (ctx *ServerContext) janitorJob() {
	for {
		staleClients := atomic.LoadUint32(&ctx.nStaleClients)
		if staleClients > 0 {
			ctx.messageBuffer.WakeListeners()
			atomic.AddUint32(&ctx.nStaleClients, -staleClients)
		}

		time.Sleep(JANITOR_PERIOD)
	}
}

func (ctx *ServerContext) ipBansChecker() {
	var ipBans *IPBans

	// Make a copy
	ipBansHandle := ctx.ipBansHandle

	// Wait for changes to ipBansHandle and iterate over all connected clients
	// and remove banned users
	for {
		// Forget reference to ipBans
		ipBans = nil

		ctx.configLock.RLock()
		for {
			ipBansPtr, changed := ipBansHandle.GetValue()
			if changed {
				ipBans = ipBansPtr.(*IPBans)
				break
			}

			ctx.configCond.Wait()
		}
		ctx.configLock.RUnlock()

		ctx.usernamesLock.Lock()
		for _, client := range ctx.users {
			if client.checkBan(ipBans) {
				client.writeErrorMessage("You have beeen banned")

				err := ctx.disconnectClient(client, "ban", false)
				if err != nil {
					log.Printf("error: %v", err)
				}
			}
		}
		ctx.usernamesLock.Unlock()
	}
}

func (ctx *ServerContext) chatLogger() {
	var readCursor uint64 = 0

	for {
		cursor, msgPtr := ctx.messageBuffer.Get(readCursor, nil)

		msg := msgPtr.(PreparedMessage)

		switch m := msg.originalMsg.(type) {
		case *OutgoingMessage:
			t := time.Unix(int64(m.Timestmap), 0).Local()

			ctx.logConfig.chatLogger.Printf(
				"%04d/%02d/%02d %02d:%02d:%02d | %v | %s: %s",
				t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(),
				msg.sourceIP, m.Username, m.Message)
		}

		readCursor = cursor + 1
	}
}

func (ctx *ServerContext) Start() {
	go ctx.janitorJob()
	go ctx.ipBansChecker()
	go ctx.chatLogger()
}
