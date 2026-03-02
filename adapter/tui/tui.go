package tui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	si "github.com/kayushkin/si"
)

// Adapter provides a simple terminal interface for interacting with inber.
type Adapter struct {
	incoming chan si.Message
	username string
}

// New creates a TUI adapter.
func New(username string) *Adapter {
	return &Adapter{
		incoming: make(chan si.Message, 16),
		username: username,
	}
}

func (a *Adapter) Name() string { return "tui" }

// Start reads lines from stdin and sends them as messages.
func (a *Adapter) Start(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)

	go func() {
		for scanner.Scan() {
			text := scanner.Text()
			if text == "" {
				continue
			}
			a.incoming <- si.Message{
				ID:        uuid.New().String(),
				Text:      text,
				Author:    a.username,
				Channel:   "tui",
				Timestamp: time.Now(),
			}
		}
	}()

	<-ctx.Done()
	close(a.incoming)
	return nil
}

// Send prints a message to stdout.
func (a *Adapter) Send(msg si.Message) error {
	fmt.Printf("\033[36m%s\033[0m: %s\n", msg.Author, msg.Text)
	return nil
}

// Receive returns the channel of inbound messages.
func (a *Adapter) Receive() <-chan si.Message {
	return a.incoming
}
