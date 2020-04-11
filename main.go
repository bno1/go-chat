package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
)

type void struct{}

const defaultConfigFilePath = "config.ini"

var defaultConfig = Config{
	// Messages
	MaxSocketMessageLen: 4096,
	MaxMessagesPerSec:   1.0,
	MaxMessagesBurst:    3,

	// Users
	MinUsernameLength: 3,
	MaxUsernameLength: 16,
}

type Client struct {
	username string
	conn     *websocket.Conn

	needRemove bool

	onceClose sync.Once

	nextClient *Client
}

type ClientContext struct {
	client Client
}

type Context struct {
	configFilePath string

	configStore   ConfigStore          // live configuration
	broadcast     chan OutgoingMessage // broadcast channel
	configUpdate  chan void            // config update notifications
	signalChannel chan os.Signal       // os signals channel

	usernames sync.Map // usernames of connected users

	clients    *Client // linked list of clients
	newClients *Client // linked list of new clients

	nClients      uint32 // number active clients
	nStaleClients uint32 // number of clients that have to be removed

	upgrader websocket.Upgrader
}

func clientListPrepend(root **Client, new *Client) {
	rootPtr := (*unsafe.Pointer)(unsafe.Pointer(root))

	for {
		next := atomic.LoadPointer(rootPtr)
		new.nextClient = (*Client)(next)
		swapped := atomic.CompareAndSwapPointer(
			rootPtr, next, unsafe.Pointer(new))
		if swapped {
			break
		}
	}
}

func clientPointersSwap(ptr **Client, new *Client) *Client {
	return (*Client)(atomic.SwapPointer(
		(*unsafe.Pointer)(unsafe.Pointer(ptr)),
		unsafe.Pointer(new)))
}

func (ctx *Context) closeClient(client *Client) {
	client.onceClose.Do(func() {
		client.needRemove = true
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

func (ctx *Context) handleConnectionInit(
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

func (ctx *Context) handleConnection(w http.ResponseWriter, r *http.Request) {
	var clientContext ClientContext

	// Upgrade initial GET request to a websocket
	ws, err := ctx.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error %v", err)
		return
	}

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

	clientListPrepend(&ctx.newClients, &clientContext.client)

	last_message_time := time.Now()

	for {
		if clientContext.client.needRemove {
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

func (ctx *Context) sendMessage(client *Client, msg *OutgoingMessage) {
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

func (ctx *Context) handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		msg := <-ctx.broadcast

		prevPtr := &ctx.clients

		// Iterate over ctx.clients
		// 1. If client doesn't have needRemove then send the message to them
		// 2. If client has needRemove update *prevPtr to point to the next
		//    client
		// 3. Else update prevPtr to point to the pointer to the next client
		//
		// Then, check if there are new clients in ctx.newClients and place
		// them at the end of ctx.clients using prevPtr and an atomic swap.
		// Iterate over ctx.clients once more.
		for i := 0; i < 2; i++ {
			clientPtr := *prevPtr

			for clientPtr != nil {
				log.Printf("Handling client %v", clientPtr.username)

				if !clientPtr.needRemove {
					ctx.sendMessage(clientPtr, &msg)
				}

				if clientPtr.needRemove {
					*prevPtr = clientPtr.nextClient
				} else {
					prevPtr = &clientPtr.nextClient
				}

				clientPtr = clientPtr.nextClient
			}

			// Move clients in ctx.newClients to the end of ctx.clients by
			// moving the to prevPtr
			if ctx.newClients != nil {
				*prevPtr = clientPointersSwap(&ctx.newClients, nil)
			}
		}
	}
}

func (ctx *Context) reloadConfig() {
	config, err := ReadConfig(ctx.configFilePath, &defaultConfig)
	if err != nil {
		log.Printf("Failed to reload config: %v", err)
		return
	}

	log.Printf("Got config: %v", config)

	ctx.configStore.UpdateConfig(config)
}

func (ctx *Context) handleConfigUpdates() {
	for {
		<-ctx.configUpdate
		ctx.reloadConfig()
	}
}

func (ctx *Context) handleSignal() {
	for {
		sig := <-ctx.signalChannel

		switch sig {
		case syscall.SIGUSR1:
			// Reload config
			ctx.configUpdate <- void{}
		}
	}
}

func main() {
	// Init config with defaultConfig
	configStore := NewConfigStore(defaultConfig)

	// Websocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Server context
	ctx := Context{
		configFilePath: defaultConfigFilePath,
		broadcast:      make(chan OutgoingMessage),
		configUpdate:   make(chan void),
		signalChannel:  make(chan os.Signal, 10),
		configStore:    configStore,
		upgrader:       upgrader,
	}

	// Load configuration
	ctx.reloadConfig()

	// Install handler for signal SIGUSR1
	signal.Notify(ctx.signalChannel, syscall.SIGUSR1)

	// Use binary asset FileServer
	http.Handle("/", http.FileServer(AssetFile()))

	// Configure websocket route
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ctx.handleConnection(w, r)
	})

	// Start config updater
	go ctx.handleConfigUpdates()

	// Start signal handler
	go ctx.handleSignal()

	// Start listening for incoming chat messages
	go ctx.handleMessages()

	// Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :8000")
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
