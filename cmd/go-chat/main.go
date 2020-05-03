package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	. "go-chat/generated"
	. "go-chat/internal"

	"github.com/gorilla/websocket"
)

type void struct{}

const defaultConfigFilePath = "config.ini"

var defaultConfig = Config{
	// Messages
	MaxSocketMessageLen: 4096,
	MaxMessagesPerSec:   1.0,
	MaxMessagesBurst:    3,
	BacklogLength:       20,

	// Users
	MinUsernameLength: 3,
	MaxUsernameLength: 16,

	// IPBan
	BlacklistPath: "ip_blacklist.txt",
	WhitelistPath: "ip_whitelist.txt",

	// Logging
	ErrorLog: "",
	ChatLog:  "",
}

type Context struct {
	configFilePath      string
	configBox           *VersionedBox
	ipBansBox           *VersionedBox
	signalChannel       chan os.Signal
	reloadConfigChannel chan void

	serverCtx ServerContext
	upgrader  websocket.Upgrader
}

func (ctx *Context) handleSignals() {
	for {
		sig := <-ctx.signalChannel

		switch sig {
		case syscall.SIGUSR1:
			// Reload config
			ctx.reloadConfigChannel <- void{}
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

	ctx.configBox.UpdateValue(&config)

	ipBans, err := LoadIPBans(config.BlacklistPath, config.WhitelistPath)
	if err != nil {
		log.Printf("Failed to load ip bans: %v", err)
		return
	}

	log.Printf("Got ip bans: %v", ipBans)

	ctx.ipBansBox.UpdateValue(&ipBans)

	ctx.serverCtx.NotifyConfigUpdate()
}

func (ctx *Context) handleConfigUpdates() {
	for {
		<-ctx.reloadConfigChannel
		ctx.reloadConfig()
	}
}

func (ctx *Context) handleConnection(w http.ResponseWriter, r *http.Request) {
	realIPStr := r.Header.Get("X-Real-IP")
	realIP := net.ParseIP(realIPStr)

	// Upgrade initial GET request to a websocket
	ws, err := ctx.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error %v", err)
		return
	}

	ctx.serverCtx.HandleConnection(ws, realIP)
}

func main() {
	// Init config with defaultConfig
	var emptyIPBans = EmptyIPBans()

	configBox := NewVersionedBox(&defaultConfig)
	ipBansBox := NewVersionedBox(&emptyIPBans)

	// Websocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Context
	ctx := Context{
		configFilePath:      defaultConfigFilePath,
		configBox:           &configBox,
		ipBansBox:           &ipBansBox,
		signalChannel:       make(chan os.Signal, 10),
		reloadConfigChannel: make(chan void),

		upgrader: upgrader,
	}

	ctx.serverCtx = NewServerContext(&configBox, ipBansBox.GetHandle())

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

	// GET stats
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		msg, err := ctx.serverCtx.GetStats()
		if err != nil {
			log.Printf("Error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("content-length", fmt.Sprintf("%d", len(msg)))
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(msg)
	})

	// Start config updater
	go ctx.handleConfigUpdates()

	// Start signal handler
	go ctx.handleSignals()

	// Start listening for incoming chat messages
	ctx.serverCtx.Start()

	// Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :8000")
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
