package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

// Adapter connects to OpenClaw gateway as a WebSocket client.
type Adapter struct {
	url       string
	token     string
	incoming  chan si.Message
	conn      *websocket.Conn
	connMu    sync.RWMutex
	connected bool
	ctx       context.Context
}

// New creates an OpenClaw adapter.
func New(url, token string) *Adapter {
	return &Adapter{
		url:      url,
		token:    token,
		incoming: make(chan si.Message, 64),
	}
}

func (a *Adapter) Name() string { return "openclaw" }

// Start connects to the gateway and handles reconnection.
func (a *Adapter) Start(ctx context.Context) error {
	a.ctx = ctx

	for {
		select {
		case <-ctx.Done():
			a.disconnect()
			close(a.incoming)
			return nil
		default:
			if err := a.connect(); err != nil {
				log.Printf("[openclaw] connection failed: %v, retrying in 5s...", err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Connected successfully, handle messages
			a.handleMessages()

			// If we get here, connection was lost
			log.Printf("[openclaw] disconnected, reconnecting in 5s...")
			time.Sleep(5 * time.Second)
		}
	}
}

func (a *Adapter) connect() error {
	// Derive Origin from WebSocket URL
	origin := strings.Replace(a.url, "ws://", "http://", 1)
	origin = strings.Replace(origin, "wss://", "https://", 1)
	origin = strings.TrimSuffix(origin, "/ws")
	origin = strings.TrimSuffix(origin, "/")

	header := http.Header{}
	header.Set("Origin", origin)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(a.url, header)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	a.connMu.Lock()
	a.conn = conn
	a.connected = false
	a.connMu.Unlock()

	log.Printf("[openclaw] websocket opened, waiting for challenge...")

	// Wait for connect.challenge event
	for {
		var msg GatewayMessage
		if err := conn.ReadJSON(&msg); err != nil {
			conn.Close()
			return fmt.Errorf("read challenge failed: %w", err)
		}

		if msg.Type == "event" && msg.Event == "connect.challenge" {
			log.Printf("[openclaw] received challenge, sending connect request")
			if err := a.sendConnectRequest(conn); err != nil {
				conn.Close()
				return fmt.Errorf("connect request failed: %w", err)
			}
			break
		}
	}

	// Wait for hello-ok response
	for {
		var msg GatewayMessage
		if err := conn.ReadJSON(&msg); err != nil {
			conn.Close()
			return fmt.Errorf("read hello-ok failed: %w", err)
		}

		if msg.Type == "res" {
			if msg.OK && msg.Payload != nil {
				var payload map[string]interface{}
				if err := json.Unmarshal(msg.Payload, &payload); err == nil {
					if payload["type"] == "hello-ok" {
						log.Printf("[openclaw] connected successfully! Protocol: %v", payload["protocol"])
						a.connMu.Lock()
						a.connected = true
						a.connMu.Unlock()
						return nil
					}
				}
			}

			// Connection failed
			errMsg := "unknown error"
			if msg.Error != nil {
				var errData map[string]interface{}
				if err := json.Unmarshal(msg.Error, &errData); err == nil {
					if m, ok := errData["message"].(string); ok {
						errMsg = m
					}
				}
			}
			conn.Close()
			return fmt.Errorf("connect failed: %s", errMsg)
		}
	}
}

func (a *Adapter) sendConnectRequest(conn *websocket.Conn) error {
	req := map[string]interface{}{
		"type":   "req",
		"id":     uuid.New().String(),
		"method": "connect",
		"params": map[string]interface{}{
			"minProtocol": 3,
			"maxProtocol": 3,
			"client": map[string]interface{}{
				"id":       "openclaw-control-ui",
				"version":  "1.0.0",
				"platform": "go",
				"mode":     "webchat",
			},
			"role":   "operator",
			"scopes": []string{"operator.admin", "operator.read", "operator.write"},
			"caps":   []string{},
			"commands": []string{},
			"permissions": map[string]interface{}{},
			"auth": map[string]interface{}{
				"token": a.token,
			},
			"locale":    "en-US",
			"userAgent": "si-comms/1.0.0",
		},
	}

	return conn.WriteJSON(req)
}

func (a *Adapter) handleMessages() {
	defer a.disconnect()

	responseBuffer := strings.Builder{}

	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		a.connMu.RLock()
		conn := a.conn
		a.connMu.RUnlock()

		if conn == nil {
			return
		}

		var msg GatewayMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("[openclaw] read error: %v", err)
			return
		}

		// Handle agent events
		if msg.Type == "event" && msg.Event == "agent" {
			if msg.Payload == nil {
				continue
			}

			var payload AgentEventPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[openclaw] failed to parse agent event: %v", err)
				continue
			}

			switch payload.Stream {
			case "assistant":
				if payload.Data != nil && payload.Data.Delta != "" {
					responseBuffer.WriteString(payload.Data.Delta)
				}

			case "lifecycle":
				if payload.Data != nil {
					phase := payload.Data.Phase
					if phase == "end" || phase == "error" {
						fullResponse := strings.TrimSpace(responseBuffer.String())
						responseBuffer.Reset()

						if fullResponse != "" {
							a.incoming <- si.Message{
								ID:        uuid.New().String(),
								Text:      fullResponse,
								Author:    "openclaw",
								Channel:   "openclaw",
								Timestamp: time.Now(),
							}
						}
					}
				}
			}
		}
	}
}

func (a *Adapter) disconnect() {
	a.connMu.Lock()
	defer a.connMu.Unlock()

	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	a.connected = false
}

// Send delivers a message to OpenClaw gateway.
func (a *Adapter) Send(msg si.Message) error {
	a.connMu.RLock()
	conn := a.conn
	connected := a.connected
	a.connMu.RUnlock()

	if conn == nil || !connected {
		return fmt.Errorf("not connected")
	}

	req := map[string]interface{}{
		"type":   "req",
		"id":     uuid.New().String(),
		"method": "agent",
		"params": map[string]interface{}{
			"message":        msg.Text,
			"agentId":        "main",
			"channel":        "webchat",
			"idempotencyKey": uuid.New().String(),
		},
	}

	if err := conn.WriteJSON(req); err != nil {
		log.Printf("[openclaw] send error: %v", err)
		return err
	}

	return nil
}

// Receive returns the channel of inbound messages.
func (a *Adapter) Receive() <-chan si.Message {
	return a.incoming
}

// GatewayMessage represents the base gateway message structure.
type GatewayMessage struct {
	Type    string          `json:"type"`
	Event   string          `json:"event,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// AgentEventPayload represents agent event data.
type AgentEventPayload struct {
	Stream string          `json:"stream"`
	Data   *AgentEventData `json:"data,omitempty"`
}

// AgentEventData represents the data field in agent events.
type AgentEventData struct {
	Delta string `json:"delta,omitempty"`
	Phase string `json:"phase,omitempty"`
}

// compile-time check
var _ si.Adapter = (*Adapter)(nil)
