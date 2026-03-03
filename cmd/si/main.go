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
	// SI_FEED=echo   - echo mode (test)
	// SI_FEED=api    - WebSocket API (inber connects to si)
	// SI_FEED=inber  - direct inber calls (si calls inber CLI)
	feedMode := os.Getenv("SI_FEED")
	if feedMode == "" {
		feedMode = "inber" // default: direct inber with opus46
	}

	var f si.Feed
	switch feedMode {
	case "echo":
		log.Println("[sí] using echo feed (test mode)")
		f = feed.NewEcho()

	case "api":
		// API feed — inber connects via WebSocket
		apiAddr := os.Getenv("SI_API_ADDR")
		if apiAddr == "" {
			apiAddr = ":8091"
		}
		apiFeed := feed.NewAPI(apiAddr)
		go apiFeed.Start(ctx)
		f = apiFeed

	case "inber":
		// Direct inber — si calls inber CLI with opus46 orchestrator
		inberFeed := feed.NewInberDirect(feed.InberDirectConfig{
			Agent: getEnvOrDefault("SI_INBER_AGENT", "task-manager"),
			Model: os.Getenv("SI_INBER_MODEL"), // optional override
		})
		inberFeed.Start()
		f = inberFeed
		log.Println("[sí] using inber direct feed with task-manager (opus46)")

	default:
		log.Fatalf("unknown SI_FEED mode: %s (use: echo, api, inber)", feedMode)
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

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
