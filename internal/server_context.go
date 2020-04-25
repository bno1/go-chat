package internal

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	username string
	conn     *websocket.Conn
	node     *Node

	onceClose sync.Once
}

type ClientContext struct {
	client Client
}

type ServerContext struct {
	configStore *ConfigStore         // live configuration
	broadcast   chan OutgoingMessage // broadcast channel

	usernames sync.Map // usernames of connected users

	clients ConcurrentList // list of clients

	nClients      uint32 // number active clients
	nStaleClients uint32 // number of clients that have to be removed
}

func NewServerContext(configStore *ConfigStore) ServerContext {
	return ServerContext{
		configStore: configStore,
		broadcast:   make(chan OutgoingMessage),
	}
}

func (ctx *ServerContext) closeClient(client *Client) {
	client.onceClose.Do(func() {
		if client.node != nil {
			client.node.Remove()
		}

		atomic.AddUint32(&ctx.nStaleClients, 1)

		if client.conn != nil {
			client.conn.Close()
		}

		if len(client.username) > 0 {
			ctx.usernames.Delete(client.username)
			client.username = ""

			// Decrement numer of clients
			atomic.AddUint32(&ctx.nClients, ^uint32(0))
		}
	})
}

func (ctx *ServerContext) handleConnectionInit(
	clientContext *ClientContext,
	config *Config,
) error {
	var msg InitMessage

	err := clientContext.client.conn.ReadJSON(&msg)
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

	_, loaded := ctx.usernames.LoadOrStore(msg.Username, &clientContext.client)

	if loaded {
		log.Printf("Username already used")
		return fmt.Errorf("Username already used")
	}

	clientContext.client.username = msg.Username
	atomic.AddUint32(&ctx.nClients, 1)

	return nil
}

func (ctx *ServerContext) HandleConnection(ws *websocket.Conn) {
	var clientContext ClientContext

	clientContext.client.conn = ws
	defer ctx.closeClient(&clientContext.client)

	configHandle := ctx.configStore.GetHandle()
	config, changed := configHandle.GetConfig()

	ws.SetReadLimit(int64(config.MaxSocketMessageLen))

	bucket, err := NewTokenBucket(
		config.MaxMessagesBurst,
		config.MaxMessagesBurst,
		config.MaxMessagesPerSec)
	if err != nil {
		log.Printf("error %v", err)
		return
	}

	for {
		err = ctx.handleConnectionInit(&clientContext, config)
		if err == nil {
			ws.WriteJSON(StatusMessage{
				Status:    "Connected",
				UserCount: ctx.nClients,
			})
			break
		}

		if websocket.IsUnexpectedCloseError(err) {
			log.Printf("connection closed: %v", err)
			return
		}

		ws.WriteJSON(ErrorMessage{
			Error: err.Error(),
		})
	}

	listNode := NewNode(&clientContext.client)
	clientContext.client.node = &listNode
	ctx.clients.Add(&listNode)

	last_message_time := time.Now()

	for {
		if clientContext.client.node.Removed() {
			break
		}

		config, changed = configHandle.GetConfig()
		if changed {
			// log.Printf("updating config")
			ws.SetReadLimit(int64(config.MaxSocketMessageLen))
			bucket.UpdateParams(
				config.MaxMessagesBurst,
				config.MaxMessagesPerSec)
		}

		// Discard reference to config
		config = nil

		var msg IncomingMessage

		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
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
		bucket.Update(now.Sub(last_message_time))
		last_message_time = now

		if bucket.TryConsumeToken() {
			// Send the newly received message to the broadcast channel
			ctx.broadcast <- OutgoingMessage{
				Username: clientContext.client.username,
				Message:  msg.Message,
			}
		} else {
			// User tries to send too fast
			ws.WriteJSON(ErrorMessage{
				Error: "Please wait before sending another message",
			})
		}
	}
}

func (ctx *ServerContext) sendMessage(client *Client, msg *OutgoingMessage) {
	err := client.conn.WriteJSON(msg)
	if err != nil {
		if websocket.IsUnexpectedCloseError(err) {
			log.Printf("connection closed: %v", err)
			ctx.closeClient(client)
		} else {
			log.Printf("error: %v", err)
		}
	}
}

func (ctx *ServerContext) handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		msg := <-ctx.broadcast

		ctx.clients.Interate(func(node *Node) {
			client := node.Data.(*Client)

			ctx.sendMessage(client, &msg)
		})
	}
}

func (ctx *ServerContext) Start() {
	go ctx.handleMessages()
}
