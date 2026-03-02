package feed

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	si "github.com/kayushkin/si"
)

// Echo is a test feed that echoes messages back with a canned response.
// Simulates an inber session for testing the adapter/router pipeline.
type Echo struct {
	outbound chan si.Message
	inbound  chan si.Message
	done     chan struct{}
}

func NewEcho() *Echo {
	e := &Echo{
		outbound: make(chan si.Message, 64),
		inbound:  make(chan si.Message, 64),
		done:     make(chan struct{}),
	}
	go e.loop()
	return e
}

func (e *Echo) loop() {
	for {
		select {
		case msg, ok := <-e.outbound:
			if !ok {
				return
			}
			// Simulate thinking
			time.Sleep(200 * time.Millisecond)

			response := generateResponse(msg.Text)
			e.inbound <- si.Message{
				ID:        uuid.New().String(),
				Text:      response,
				Author:    "inber",
				Channel:   "", // broadcast to all adapters
				Timestamp: time.Now(),
			}
		case <-e.done:
			return
		}
	}
}

func generateResponse(input string) string {
	lower := strings.ToLower(input)

	switch {
	case strings.Contains(lower, "hello") || strings.Contains(lower, "hi"):
		return "hey! sí echo feed here — messages are flowing 🦀"
	case strings.Contains(lower, "ping"):
		return "pong"
	case strings.Contains(lower, "how are you"):
		return "i'm a test feed, i don't have feelings. but the router works!"
	case strings.Contains(lower, "bye") || strings.Contains(lower, "quit"):
		return "later! ctrl+c to actually quit though"
	default:
		return fmt.Sprintf("echo: %q — router ↔ adapter pipeline is working", input)
	}
}

func (e *Echo) Write(msg si.Message) error {
	e.outbound <- msg
	return nil
}

func (e *Echo) Read() <-chan si.Message {
	return e.inbound
}

func (e *Echo) Close() error {
	close(e.done)
	return nil
}
