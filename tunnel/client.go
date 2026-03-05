package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

// Client runs on WSL, connects outbound to tunnel server, forwards to local inber.
type Client struct {
	serverURL   string
	authToken   string
	clientID    string
	inberBin    string
	inberDir    string
	conn        *websocket.Conn
	mu          sync.Mutex
	reconnectMs int
}

// ClientConfig configures the tunnel client.
type ClientConfig struct {
	ServerURL   string // e.g., "wss://kayushkin.com:8091/tunnel"
	AuthToken   string // shared secret
	ClientID    string // optional client identifier
	InberBin    string // path to inber binary
	InberDir    string // working directory for inber
	ReconnectMs int    // reconnect delay (default 5000)
}

// NewClient creates a tunnel client.
func NewClient(cfg ClientConfig) *Client {
	if cfg.ReconnectMs == 0 {
		cfg.ReconnectMs = 5000
	}
	if cfg.InberBin == "" {
		cfg.InberBin = os.ExpandEnv("$HOME/bin/inber")
	}
	if cfg.InberDir == "" {
		cfg.InberDir = os.ExpandEnv("$HOME/life/repos/inber")
	}

	return &Client{
		serverURL:   cfg.ServerURL,
		authToken:   cfg.AuthToken,
		clientID:    cfg.ClientID,
		inberBin:    cfg.InberBin,
		inberDir:    cfg.InberDir,
		reconnectMs: cfg.ReconnectMs,
	}
}

// Run connects to the tunnel server and processes messages.
// Blocks until context is cancelled.
func (c *Client) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.connectAndProcess(ctx); err != nil {
			log.Printf("[tunnel-client] connection error: %v, reconnecting in %dms...", err, c.reconnectMs)
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(c.reconnectMs) * time.Millisecond):
			}
		}
	}
}

// connectAndProcess establishes a connection and handles messages.
func (c *Client) connectAndProcess(ctx context.Context) error {
	// Build URL with auth
	url := c.serverURL
	if c.authToken != "" {
		url += "?token=" + c.authToken
	}
	if c.clientID != "" {
		if c.authToken != "" {
			url += "&client_id=" + c.clientID
		} else {
			url += "?client_id=" + c.clientID
		}
	}

	log.Printf("[tunnel-client] connecting to %s...", c.serverURL)

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	header := http.Header{}
	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return err
	}
	c.conn = conn
	defer conn.Close()

	log.Printf("[tunnel-client] connected to tunnel server")

		// Start heartbeat goroutine to keep connection alive
	heartbeatChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.mu.Lock()
				// Send heartbeat as a special message
				heartbeat := si.Message{Text: "__heartbeat__"}
				data, _ := json.Marshal(heartbeat)
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					c.mu.Unlock()
					close(heartbeatChan)
					return
				}
				c.mu.Unlock()
			}
		}
	}()

	// Read messages from tunnel server, process with local inber
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeatChan:
			return fmt.Errorf("heartbeat failed")
		default:
		}

		// Set read deadline for keepalive detection (longer since we send heartbeats)
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))

		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg si.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[tunnel-client] unmarshal error: %v", err)
			continue
		}

		// Skip heartbeat messages
		if msg.Text == "__heartbeat__" || msg.Text == "__heartbeat_ack__" {
			continue
		}

		log.Printf("[tunnel-client] received: %s", truncate(msg.Text, 50))

		// Process with local inber
		response := c.processWithInber(ctx, msg)

		// Send response back through tunnel
		respData, err := json.Marshal(response)
		if err != nil {
			log.Printf("[tunnel-client] marshal response error: %v", err)
			continue
		}

		c.mu.Lock()
		err = conn.WriteMessage(websocket.TextMessage, respData)
		c.mu.Unlock()

		if err != nil {
			return err
		}

		log.Printf("[tunnel-client] sent response: %s", truncate(response.Text, 50))
	}
}

// processWithInber runs inber locally and returns the response.
func (c *Client) processWithInber(ctx context.Context, msg si.Message) si.Message {
	// Build context prefix
	contextPrefix := ""
	if msg.Channel != "" {
		contextPrefix = "[from " + msg.Channel
		if msg.Author != "" {
			contextPrefix += " by " + msg.Author
		}
		contextPrefix += "] "
	}

	// Determine agent
	agent := "task-manager"
	if msg.Agent != "" {
		agent = msg.Agent
	}

	// Run inber with --continue to resume the most recent session
	// This maintains context across all dashboard messages
	args := []string{"run", "--agent", agent, "--continue"}

	cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, c.inberBin, args...)
	cmd.Dir = c.inberDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return si.Message{Text: "error: " + err.Error(), Channel: msg.Channel}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return si.Message{Text: "error: " + err.Error(), Channel: msg.Channel}
	}

	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return si.Message{Text: "error: " + err.Error(), Channel: msg.Channel}
	}

	// Send input
	input := contextPrefix + msg.Text
	stdin.Write([]byte(input))
	stdin.Close()

	// Read output
	output := make([]byte, 0, 4096)
	buf := make([]byte, 1024)
	for {
		n, err := stdout.Read(buf)
		if err != nil {
			break
		}
		output = append(output, buf[:n]...)
	}

	// Check for errors
	errData, _ := io.ReadAll(stderr)
	cmd.Wait()

	response := si.Message{
		Text:    string(output),
		Channel: msg.Channel,
		Author:  agent,
	}

	if len(output) == 0 && len(errData) > 0 {
		response.Text = string(errData)
	}

	return response
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
