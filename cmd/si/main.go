package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	si "github.com/kayushkin/si"
	"github.com/kayushkin/si/adapter/discord"
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

	// TUI adapter (always on)
	tuiAdapter := tui.New("slava")
	router.AddAdapter(tuiAdapter)

	// WebSocket adapter for Claxon Android
	wsAdapter := websocket.New(":8090")
	router.AddAdapter(wsAdapter)

	// Discord adapter (if token provided)
	if token := os.Getenv("SI_DISCORD_TOKEN"); token != "" {
		channelID := os.Getenv("SI_DISCORD_CHANNEL")
		if channelID == "" {
			channelID = "143132977210195968" // default: Pretend server
		}
		discordAdapter := discord.New(token, channelID)
		router.AddAdapter(discordAdapter)
	}

	log.Println("[sí] starting...")

	if err := router.Run(ctx); err != nil {
		log.Printf("[sí] %v", err)
	}
}
