package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	si "github.com/kayushkin/si"
	"github.com/kayushkin/si/adapter/discord"
	"github.com/kayushkin/si/adapter/websocket"
	"github.com/kayushkin/si/feed"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// SI_FEED=bus (default) or echo (test).
	feedMode := os.Getenv("SI_FEED")
	if feedMode == "" {
		feedMode = "bus"
	}

	var f si.Feed
	switch feedMode {
	case "echo":
		log.Println("[sí] using echo feed (test mode)")
		f = feed.NewEcho()

	case "bus":
		natsURL := os.Getenv("NATS_URL")
		if natsURL == "" {
			natsURL = "nats://localhost:4222"
		}
		natsFeed := feed.NewNATSFeed(feed.NATSFeedConfig{
			NATSURL:  natsURL,
			Consumer: "si",
		})
		if err := natsFeed.Start(feed.NATSFeedConfig{
			NATSURL:  natsURL,
			Consumer: "si",
		}); err != nil {
			log.Fatalf("[sí] failed to connect to NATS: %v", err)
		}
		f = natsFeed
		log.Printf("[sí] using NATS feed via %s", natsURL)

	default:
		log.Fatalf("unknown SI_FEED mode: %s (use: bus, echo)", feedMode)
	}

	// Router — stateless, no history.
	router := si.NewRouter(f)

	// WebSocket adapter (dashboard, Claxon Android).
	wsAddr := os.Getenv("SI_WS_ADDR")
	if wsAddr == "" {
		wsAddr = ":8090"
	}
	wsAdapter := websocket.New(wsAddr)
	wsAdapter.SetRouter(router)
	router.AddAdapter(wsAdapter)

	// Discord adapter (if token provided).
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
