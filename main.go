package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const MAX_MESSAGE_LEN = 4096
const MAX_MESSAGES_PER_SEC = 1.0
const MESSAGES_MAX_BURST = 3

var clients = make(map[*websocket.Conn]bool) // connected clients
var broadcast = make(chan Message)           // broadcast channel

// Configure the upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Message object
type Message struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

func main() {
	// Use binary asset FileServer
	http.Handle("/", http.FileServer(AssetFile()))

	// Configure websocket route
	http.HandleFunc("/ws", handleConnections)

	// Start listening for incoming chat messages
	go handleMessages()

	// Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :8000")
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error %v", err)
		return
	}
	// Make sure we close the connection when the function returns
	defer ws.Close()

	ws.SetReadLimit(MAX_MESSAGE_LEN)

	// Register our new client
	clients[ws] = true

	bucket, err := NewTokenBucket(
		MESSAGES_MAX_BURST, MESSAGES_MAX_BURST, MAX_MESSAGES_PER_SEC)
	if err != nil {
		log.Printf("error %v", err)
		return
	}

	last_message_time := time.Now()

	for {
		var msg Message
		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, ws)
			break
		}

		// Update bucket
		now := time.Now()
		bucket.Update(now.Sub(last_message_time))
		last_message_time = now

		if bucket.TryConsumeToken() {
			// Send the newly received message to the broadcast channel
			broadcast <- msg
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

func handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		msg := <-broadcast
		// Send it out to every client that is currently connected
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}
