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
	addr := flag.String("addr", ":8091", "address to listen on")
	token := flag.String("token", os.Getenv("TUNNEL_TOKEN"), "auth token (or TUNNEL_TOKEN env)")
	flag.Parse()

	if *token == "" {
		*token = "change-me-in-production"
		log.Println("[tunnel-server] WARNING: using default token, set TUNNEL_TOKEN for production")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("[tunnel-server] shutting down...")
		cancel()
	}()

	server := tunnel.NewServer(*addr, *token)
	if err := server.Start(ctx); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}
