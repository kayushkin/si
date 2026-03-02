package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	si "github.com/kayushkin/si"
	"github.com/kayushkin/si/adapter/tui"
	"github.com/kayushkin/si/adapter/websocket"
	"github.com/kayushkin/si/feed"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Echo feed for testing (replace with real inber feed later)
	f := feed.NewEcho()

	// Router
	router := si.NewRouter(f)

	// TUI adapter (always on for now)
	tuiAdapter := tui.New("slava")
	router.AddAdapter(tuiAdapter)

	// WebSocket adapter for Claxon Android
	wsAdapter := websocket.New(":8090")
	router.AddAdapter(wsAdapter)

	// TODO: matterbridge adapter (enable via config)
	// mbAdapter := matterbridge.New("http://localhost:4242/api", "gateway1", "inber")
	// router.AddAdapter(mbAdapter)

	log.Println("[sí] starting...")

	if err := router.Run(ctx); err != nil {
		log.Printf("[sí] %v", err)
	}
}
