package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kayushkin/si/tunnel"
)

func main() {
	serverURL := flag.String("server", os.Getenv("TUNNEL_SERVER"), "tunnel server URL (e.g., wss://kayushkin.com:8091/tunnel)")
	token := flag.String("token", os.Getenv("TUNNEL_TOKEN"), "auth token (or TUNNEL_TOKEN env)")
	clientID := flag.String("id", os.Getenv("TUNNEL_CLIENT_ID"), "client ID (or TUNNEL_CLIENT_ID env)")
	inberBin := flag.String("inber", os.Getenv("TUNNEL_INBER_BIN"), "path to inber binary")
	inberDir := flag.String("dir", os.Getenv("TUNNEL_INBER_DIR"), "inber working directory")
	flag.Parse()

	if *serverURL == "" {
		*serverURL = "ws://127.0.0.1:8091/tunnel"
		log.Println("[tunnel-client] WARNING: using localhost, set TUNNEL_SERVER for production")
	}

	if *token == "" {
		*token = "change-me-in-production"
		log.Println("[tunnel-client] WARNING: using default token, set TUNNEL_TOKEN for production")
	}

	if *clientID == "" {
		*clientID = "wsl-main"
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("[tunnel-client] shutting down...")
		cancel()
	}()

	client := tunnel.NewClient(tunnel.ClientConfig{
		ServerURL: *serverURL,
		AuthToken: *token,
		ClientID:  *clientID,
		InberBin:  *inberBin,
		InberDir:  *inberDir,
	})

	if err := client.Run(ctx); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}
