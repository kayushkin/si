package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	si "github.com/kayushkin/si"
	"github.com/kayushkin/si/adapter/discord"
	"github.com/kayushkin/si/adapter/openclaw"
	"github.com/kayushkin/si/adapter/tui"
	"github.com/kayushkin/si/adapter/websocket"
	"github.com/kayushkin/si/feed"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Determine feed mode
	useEcho := os.Getenv("SI_ECHO") == "1"

	var f si.Feed
	if useEcho {
		log.Println("[sí] using echo feed (test mode)")
		f = feed.NewEcho()
	} else {
		// API feed — inber connects via WebSocket
		apiAddr := os.Getenv("SI_API_ADDR")
		if apiAddr == "" {
			apiAddr = ":8091"
		}
		apiFeed := feed.NewAPI(apiAddr)
		go apiFeed.Start(ctx)
		f = apiFeed
	}

	// Router
	router := si.NewRouter(f)

	// TUI adapter (always on)
	tuiAdapter := tui.New("slava")
	router.AddAdapter(tuiAdapter)

	// WebSocket adapter for Claxon Android
	wsAddr := os.Getenv("SI_WS_ADDR")
	if wsAddr == "" {
		wsAddr = ":8090"
	}
	wsAdapter := websocket.New(wsAddr)
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

	// OpenClaw adapter (if token provided)
	if token := os.Getenv("SI_OPENCLAW_TOKEN"); token != "" {
		url := os.Getenv("SI_OPENCLAW_URL")
		if url == "" {
			url = "ws://localhost:18789"
		}
		openclawAdapter := openclaw.New(url, token)
		router.AddAdapter(openclawAdapter)
	}

	log.Println("[sí] starting...")

	if err := router.Run(ctx); err != nil {
		log.Printf("[sí] %v", err)
	}
}
