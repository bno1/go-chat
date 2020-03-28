package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type void struct{}

var defaultConfig = Config{
	MaxSocketMessageLen: 4096,
	MaxMessagesPerSec:   1.0,
	MaxMessagesBurst:    3,
}

type Context struct {
	configStore ConfigStore  // live configuration
	clients     sync.Map     // connected clients
	broadcast   chan Message // broadcast channel

	upgrader websocket.Upgrader
}

// Message object
type Message struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

func (ctx *Context) handleConnection(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	ws, err := ctx.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error %v", err)
		return
	}
	// Make sure we close the connection when the function returns
	defer ws.Close()

	configHandle := ctx.configStore.GetHandle()
	config, changed := configHandle.GetConfig()

	ws.SetReadLimit(int64(config.MaxSocketMessageLen))

	// Register our new client
	ctx.clients.Store(ws, void{})

	bucket, err := NewTokenBucket(
		config.MaxMessagesBurst, config.MaxMessagesBurst,
		config.MaxMessagesPerSec)
	if err != nil {
		log.Printf("error %v", err)
		return
	}

	last_message_time := time.Now()

	for {
		config, changed = configHandle.GetConfig()
		if changed {
			ws.SetReadLimit(int64(config.MaxSocketMessageLen))
			bucket.UpdateParams(config.MaxMessagesBurst,
				config.MaxMessagesPerSec)
		}

		// Discard reference to config
		config = nil

		var msg Message

		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			ctx.clients.Delete(ws)
			break
		}

		// Update bucket
		now := time.Now()
		bucket.Update(now.Sub(last_message_time))
		last_message_time = now

		if bucket.TryConsumeToken() {
			// Send the newly received message to the broadcast channel
			ctx.broadcast <- msg
		} else {
			// User tries to send too fast
			ws.WriteJSON(Message{
				Email:    "",
				Username: "system",
				Message:  "Please wait before sending another message",
			})
		}
	}
}

func (ctx *Context) handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		msg := <-ctx.broadcast
		// Send it out to every client that is currently connected
		ctx.clients.Range(func(key, value interface{}) bool {
			client := key.(*websocket.Conn)
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()

				ctx.clients.Delete(client)
			}

			return true
		})
	}
}

func main() {
	configStore := NewConfigStore(defaultConfig)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	ctx := Context{
		broadcast:   make(chan Message),
		configStore: configStore,
		upgrader:    upgrader,
	}

	// Use binary asset FileServer
	http.Handle("/", http.FileServer(AssetFile()))

	// Configure websocket route
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ctx.handleConnection(w, r)
	})

	// Start listening for incoming chat messages
	go ctx.handleMessages()

	// Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :8000")
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
