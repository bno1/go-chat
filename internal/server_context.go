package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const MAX_MESSAGEBUFFER_SIZE = 4096
const JANITOR_PERIOD = 10 * time.Second

type Client struct {
	username string
	conn     *websocket.Conn

	closed bool

	writeChan  chan *websocket.PreparedMessage
	readCursor uint64

	configHandle ConfigHandle

	tokenBucket TokenBucket

	onceClose sync.Once
}

type ServerContext struct {
	configStore   *ConfigStore  // live configuration
	messageBuffer MessageBuffer // buffer for broadcast messages

	usernames sync.Map // usernames of connected users

	nClients      uint32 // number active clients
	nStaleClients uint32 // number of clients that have to be removed
}

func NewServerContext(configStore *ConfigStore) ServerContext {
	return ServerContext{
		configStore:   configStore,
		messageBuffer: NewMessageBuffer(MAX_MESSAGEBUFFER_SIZE),
	}
}

func (ctx *ServerContext) closeClient(client *Client) {
	client.onceClose.Do(func() {
		client.closed = true
		close(client.writeChan)

		atomic.AddUint32(&ctx.nStaleClients, 1)

		if len(client.username) > 0 {
			ctx.usernames.Delete(client.username)
			client.username = ""

			// Decrement numer of clients
			atomic.AddUint32(&ctx.nClients, ^uint32(0))
		}
	})
}

func (ctx *ServerContext) handleConnectionInit(
	client *Client,
	config *Config,
) error {
	var msg InitMessage

	err := client.conn.ReadJSON(&msg)
	if err != nil {
		log.Printf("error: %v", err)
		return err
	}

	if uint(len(msg.Username)) < config.MinUsernameLength {
		log.Printf("Username too short")
		return fmt.Errorf("Username too short")
	}

	if uint(len(msg.Username)) > config.MaxUsernameLength {
		log.Printf("Username too long")
		return fmt.Errorf("Username too long")
	}

	_, loaded := ctx.usernames.LoadOrStore(msg.Username, client)

	if loaded {
		log.Printf("Username already used")
		return fmt.Errorf("Username already used")
	}

	client.username = msg.Username
	atomic.AddUint32(&ctx.nClients, 1)

	return nil
}

func makePreparedJSONMessage(
	v interface{},
) (*websocket.PreparedMessage, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	msg, err := websocket.NewPreparedMessage(websocket.TextMessage, bytes)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func (client *Client) writeJSON(v interface{}) error {
	msg, err := makePreparedJSONMessage(v)
	if err != nil {
		return err
	}

	client.writeChan <- msg

	return nil
}

func (ctx *ServerContext) setupClient(client *Client) error {
	var err error

	client.configHandle = ctx.configStore.GetHandle()

	config, _ := client.configHandle.GetConfig()

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
		err = ctx.handleConnectionInit(client, config)
		if err == nil {
			client.writeJSON(StatusMessage{
				Status:    "Connected",
				UserCount: ctx.nClients,
			})
			break
		}

		if websocket.IsUnexpectedCloseError(err) {
			return err
		}

		client.writeJSON(ErrorMessage{
			Error: err.Error(),
		})
	}

	return nil
}

func (client *Client) updateConfig() {
	config, changed := client.configHandle.GetConfig()

	if changed {
		client.conn.SetReadLimit(int64(config.MaxSocketMessageLen))
		client.tokenBucket.UpdateParams(
			config.MaxMessagesBurst,
			config.MaxMessagesPerSec)
	}
}

func (ctx *ServerContext) clientReader(client *Client) {
	last_message_time := time.Now()

	for {
		if client.closed {
			break
		}

		client.updateConfig()

		var msg IncomingMessage

		// Read in a new message as JSON and map it to a Message object
		err := client.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err) {
				log.Printf("connection closed: %v", err)
				break
			}

			log.Printf("error: %v", err)
			continue
		}

		// Update bucket
		now := time.Now()
		client.tokenBucket.Update(now.Sub(last_message_time))
		last_message_time = now

		if client.tokenBucket.TryConsumeToken() {
			pmsg, err := makePreparedJSONMessage(OutgoingMessage{
				Username: client.username,
				Message:  msg.Message,
			})

			if err != nil {
				client.writeJSON(ErrorMessage{
					Error: "Failed to encode message",
				})
			}

			ctx.messageBuffer.Put(pmsg)
		} else {
			// User tries to send too fast
			client.writeJSON(ErrorMessage{
				Error: "Please wait before sending another message",
			})
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
				ctx.closeClient(client)
			} else {
				log.Printf("error: %v", err)
			}
		}
	}

	client.conn.Close()
}

func (ctx *ServerContext) clientListener(client *Client) {
	for {
		cursor, msg := ctx.messageBuffer.Get(
			client.readCursor, &client.closed)

		if client.closed {
			break
		}

		client.writeChan <- msg.(*websocket.PreparedMessage)

		client.readCursor = cursor + 1
	}
}

func (ctx *ServerContext) HandleConnection(ws *websocket.Conn) {
	var client Client

	client.conn = ws
	client.writeChan = make(chan *websocket.PreparedMessage, 10)
	defer ctx.closeClient(&client)

	// the connection will be closed by ctx.clientWriter
	go ctx.clientWriter(&client)

	err := ctx.setupClient(&client)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

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

func (ctx *ServerContext) Start() {
	go ctx.janitorJob()
}
