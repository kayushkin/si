package feed

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

// TunnelFeed connects si to the tunnel server (which routes to WSL).
type TunnelFeed struct {
	serverURL string
	authToken string
	conn      *websocket.Conn
	inbound   chan si.Message
	outbound  chan si.Message
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// TunnelFeedConfig configures the tunnel feed.
type TunnelFeedConfig struct {
	ServerURL string // e.g., "ws://127.0.0.1:8091/feed"
	AuthToken string // shared secret
}

// NewTunnelFeed creates a feed that connects to the tunnel server.
func NewTunnelFeed(cfg TunnelFeedConfig) *TunnelFeed {
	ctx, cancel := context.WithCancel(context.Background())
	return &TunnelFeed{
		serverURL: cfg.ServerURL,
		authToken: cfg.AuthToken,
		inbound:   make(chan si.Message, 64),
		outbound:  make(chan si.Message, 64),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start connects to the tunnel server and begins processing.
func (f *TunnelFeed) Start() error {
	url := f.serverURL
	if f.authToken != "" {
		url += "?token=" + f.authToken
	}

	log.Printf("[feed/tunnel] connecting to %s...", f.serverURL)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	header := http.Header{}
	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return err
	}
	f.conn = conn

	log.Printf("[feed/tunnel] connected to tunnel server")

	// Read responses from tunnel server (these came from WSL)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[feed/tunnel] read error: %v", err)
				return
			}

			var msg si.Message
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("[feed/tunnel] unmarshal error: %v", err)
				continue
			}

			log.Printf("[feed/tunnel] response from WSL: %s", func(s string, n int) string {
				if len(s) <= n {
					return s
				}
				return s[:n] + "..."
			}(msg.Text, 50))
			f.inbound <- msg
		}
	}()

	// Write messages to tunnel server (to be forwarded to WSL)
	go func() {
		for {
			select {
			case <-f.ctx.Done():
				return
			case msg, ok := <-f.outbound:
				if !ok {
					return
				}

				data, err := json.Marshal(msg)
				if err != nil {
					log.Printf("[feed/tunnel] marshal error: %v", err)
					continue
				}

				f.mu.Lock()
				err = conn.WriteMessage(websocket.TextMessage, data)
				f.mu.Unlock()

				if err != nil {
					log.Printf("[feed/tunnel] write error: %v", err)
					return
				}

				log.Printf("[feed/tunnel] sent to tunnel: %s", func(s string, n int) string {
					if len(s) <= n {
						return s
					}
					return s[:n] + "..."
				}(msg.Text, 50))
			}
		}
	}()

	return nil
}

// Write sends a message to the tunnel server.
func (f *TunnelFeed) Write(msg si.Message) error {
	select {
	case f.outbound <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}
}

// Read returns the channel of responses from WSL.
func (f *TunnelFeed) Read() <-chan si.Message {
	return f.inbound
}

// Close shuts down the feed.
func (f *TunnelFeed) Close() error {
	f.cancel()
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}
