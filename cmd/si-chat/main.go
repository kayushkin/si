package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type message struct {
	Text      string    `json:"text"`
	Author    string    `json:"author"`
	Channel   string    `json:"channel,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	addr := "localhost:8090"
	author := "slava"

	if v := os.Getenv("SI_CHAT_ADDR"); v != "" {
		addr = v
	}
	if v := os.Getenv("SI_CHAT_AUTHOR"); v != "" {
		author = v
	}
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	u := url.URL{Scheme: "ws", Host: addr, Path: "/ws"}
	fmt.Printf("connecting to %s...\n", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("connect failed: %v", err)
	}
	defer conn.Close()

	fmt.Printf("connected as %s. type to chat, ctrl-c to quit.\n\n", author)

	// Handle ctrl-c
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		fmt.Println("\nbye")
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		os.Exit(0)
	}()

	// Read incoming messages
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				fmt.Printf("\ndisconnected: %v\n", err)
				os.Exit(1)
			}

			var msg message
			if err := json.Unmarshal(data, &msg); err != nil {
				fmt.Printf("\n< %s\n", string(data))
				continue
			}

			// Don't echo back our own messages
			if msg.Author == author {
				continue
			}

			name := msg.Author
			if name == "" {
				name = msg.Channel
			}
			if name == "" {
				name = "?"
			}

			fmt.Printf("\r\033[K%s: %s\n%s> ", name, msg.Text, author)
		}
	}()

	// Read stdin and send
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("%s> ", author)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			fmt.Printf("%s> ", author)
			continue
		}

		msg := message{
			Text:      text,
			Author:    author,
			Channel:   "websocket",
			Timestamp: time.Now(),
		}

		data, _ := json.Marshal(msg)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Fatalf("send failed: %v", err)
		}

		fmt.Printf("%s> ", author)
	}
}
