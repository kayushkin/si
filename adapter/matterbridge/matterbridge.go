package matterbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	si "github.com/kayushkin/si"
)

// Matterbridge API message format.
type apiMessage struct {
	Text     string `json:"text"`
	Username string `json:"username"`
	Gateway  string `json:"gateway"`
	Channel  string `json:"channel,omitempty"`
	UserID   string `json:"userid,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Account  string `json:"account,omitempty"`
}

// Adapter connects to matterbridge's REST API.
type Adapter struct {
	apiURL   string // e.g. "http://localhost:4242/api"
	gateway  string // matterbridge gateway name
	botName  string // name to send as
	incoming chan si.Message
	pollMs   int
}

// New creates a matterbridge adapter.
func New(apiURL, gateway, botName string) *Adapter {
	return &Adapter{
		apiURL:   apiURL,
		gateway:  gateway,
		botName:  botName,
		incoming: make(chan si.Message, 64),
		pollMs:   1000,
	}
}

func (a *Adapter) Name() string { return "matterbridge" }

// Start polls the matterbridge API for new messages.
func (a *Adapter) Start(ctx context.Context) error {
	log.Printf("[matterbridge] connecting to %s (gateway: %s)", a.apiURL, a.gateway)

	ticker := time.NewTicker(time.Duration(a.pollMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(a.incoming)
			return nil
		case <-ticker.C:
			a.poll()
		}
	}
}

func (a *Adapter) poll() {
	resp, err := http.Get(a.apiURL + "/messages")
	if err != nil {
		return // Silent — matterbridge might not be running
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var msgs []apiMessage
	if err := json.Unmarshal(body, &msgs); err != nil {
		return
	}

	for _, m := range msgs {
		// Skip our own messages
		if m.Username == a.botName {
			continue
		}

		a.incoming <- si.Message{
			ID:        uuid.New().String(),
			Text:      m.Text,
			Author:    m.Username,
			Channel:   fmt.Sprintf("matterbridge:%s:%s", m.Account, m.Channel),
			Timestamp: time.Now(),
		}
	}
}

// Send posts a message to matterbridge.
func (a *Adapter) Send(msg si.Message) error {
	apiMsg := apiMessage{
		Text:     msg.Text,
		Username: a.botName,
		Gateway:  a.gateway,
	}

	data, err := json.Marshal([]apiMessage{apiMsg})
	if err != nil {
		return err
	}

	resp, err := http.Post(a.apiURL+"/message", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("matterbridge send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("matterbridge send: status %d", resp.StatusCode)
	}

	return nil
}

// Receive returns the channel of inbound messages.
func (a *Adapter) Receive() <-chan si.Message {
	return a.incoming
}
