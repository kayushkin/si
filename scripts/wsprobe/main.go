// wsprobe is the WebSocket client used by scripts/e2e-smoke.sh.
//
// si's whole job is to route a message from an adapter, through the feed, and
// back out to the adapter's clients. That path is only reachable over a real
// WebSocket — si serves exactly two routes (/ws and /api/status), and curl
// cannot drive the first. So the smoke needs a WS client, and it lives here, in
// si's own module, so that it speaks the same gorilla/websocket version as the
// server it is testing rather than a hand-rolled handshake that could drift.
//
// It connects, sends one message, and waits for the reply that came back around
// through the feed — then prints that reply as JSON on stdout for the smoke to
// assert on.
//
// The one trap it exists to handle: a client sees its OWN message echoed back
// first as event_type "inbound" (router.go publishes EventInbound before it
// writes to the feed), and only then the feed's reply as "outbound". A probe
// that reads a single frame and asserts on it would be asserting against its own
// request. So: read frames until event_type == "outbound", or time out.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type envelope struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	Message   json.RawMessage `json:"message"`
}

func main() {
	url := flag.String("url", "", "websocket url, e.g. ws://127.0.0.1:19130/ws")
	text := flag.String("text", "ping", "message text to send")
	timeout := flag.Duration("timeout", 10*time.Second, "overall deadline")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "wsprobe: -url is required")
		os.Exit(2)
	}

	deadline := time.Now().Add(*timeout)

	conn, _, err := websocket.DefaultDialer.Dial(*url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wsprobe: dial %s: %v\n", *url, err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]string{"text": *text}); err != nil {
		fmt.Fprintf(os.Stderr, "wsprobe: write: %v\n", err)
		os.Exit(1)
	}

	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			fmt.Fprintf(os.Stderr, "wsprobe: set deadline: %v\n", err)
			os.Exit(1)
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			fmt.Fprintf(os.Stderr, "wsprobe: no outbound reply within %s: %v\n", *timeout, err)
			os.Exit(1)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			// si must not be writing anything but JSON envelopes onto this socket.
			fmt.Fprintf(os.Stderr, "wsprobe: frame is not a JSON envelope: %s\n", raw)
			os.Exit(1)
		}

		// Skip our own request coming back as "inbound" — see the package comment.
		if env.EventType != "outbound" {
			continue
		}
		if len(env.Message) == 0 {
			fmt.Fprintf(os.Stderr, "wsprobe: outbound envelope carries no message: %s\n", raw)
			os.Exit(1)
		}
		fmt.Println(string(env.Message))
		return
	}
}
