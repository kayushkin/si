// test-client simulates inber connecting to sí's feed API.
// It receives messages and echoes them back with a prefix.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

func main() {
	addr := "ws://localhost:8091/feed"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	fmt.Println("connected to sí feed — waiting for messages...")

	// Handle ctrl+c
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		conn.Close()
		os.Exit(0)
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Fatalf("read: %v", err)
		}

		var msg si.Message
		json.Unmarshal(data, &msg)
		fmt.Printf("[from %s] %s: %s\n", msg.Channel, msg.Author, msg.Text)

		// Echo back (no channel = route to last origin)
		reply := si.Message{
			Text: fmt.Sprintf("inber heard you say %q 🦀", msg.Text),
		}
		replyData, _ := json.Marshal(reply)
		if err := conn.WriteMessage(websocket.TextMessage, replyData); err != nil {
			log.Fatalf("write: %v", err)
		}
		fmt.Printf("[sent] %s\n", reply.Text)
	}
}
