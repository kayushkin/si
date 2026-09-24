package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/kayushkin/llm-bridge/servicesettings"
	si "github.com/kayushkin/si"
	"github.com/kayushkin/si/adapter/discord"
	"github.com/kayushkin/si/adapter/websocket"
	"github.com/kayushkin/si/feed"
	"github.com/kayushkin/si/internal/config"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	settings, err := config.NewSettingsRegistry(servicesettings.ProcessEnvironment())
	if err != nil {
		log.Fatalf("[sí] %v", err)
	}

	// SI_FEED=nats (default), bus (legacy), or echo (test).
	feedMode := settings.String(config.SettingFeedMode)

	var f si.Feed
	switch feedMode {
	case "echo":
		log.Println("[sí] using echo feed (test mode)")
		f = feed.NewEcho()

	case "nats":
		natsURL := settings.String(config.SettingNATSURL)
		natsFeed, err := feed.NewNatsFeed(feed.NatsFeedConfig{
			NatsURL: natsURL,
		})
		if err != nil {
			log.Fatalf("[sí] failed to connect to nats: %v", err)
		}
		if err := natsFeed.Start(); err != nil {
			log.Fatalf("[sí] failed to start nats feed: %v", err)
		}
		f = natsFeed
		log.Printf("[sí] using nats feed via %s", natsURL)

	case "bus":
		natsURL := settings.String(config.SettingNATSURL)
		natsFeed, err := feed.NewNatsFeed(feed.NatsFeedConfig{
			NatsURL: natsURL,
		})
		if err != nil {
			log.Fatalf("[sí] failed to connect to nats: %v", err)
		}
		if err := natsFeed.Start(); err != nil {
			log.Fatalf("[sí] failed to start nats feed: %v", err)
		}
		f = natsFeed
		log.Printf("[sí] using nats feed via %s (bus mode)", natsURL)

	default:
		log.Fatalf("unknown SI_FEED mode: %s (use: nats, bus, echo)", feedMode)
	}

	// Router — stateless, no history.
	router := si.NewRouter(f, settings.String(config.SettingLogstackURL))

	// WebSocket adapter (dashboard, Claxon Android).
	wsAdapter := websocket.New(settings.String(config.SettingWebSocketListenAddress), websocket.Credentials{
		LegacyBearerToken: settings.String(config.SettingWebSocketBearerToken),
		JWTSecret:         settings.String(config.SettingWebSocketJWTSecret),
	})
	wsAdapter.SetRouter(router)
	wsAdapter.SetSettingsHandler(servicesettings.Handler(settings, "/settings"))
	router.AddAdapter(wsAdapter)

	// Discord adapter (if token provided).
	if token := settings.String(config.SettingDiscordBotToken); token != "" {
		discordAdapter := discord.New(token, settings.String(config.SettingDiscordChannelID))
		router.AddAdapter(discordAdapter)
	}

	log.Println("[sí] starting...")

	if err := router.Run(ctx); err != nil {
		log.Printf("[sí] %v", err)
	}
}
