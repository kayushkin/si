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

		// Process async — don't block the read loop
		go func(m si.Message) {
			response := c.processWithInber(ctx, m)

			respData, err := json.Marshal(response)
			if err != nil {
				log.Printf("[tunnel-client] marshal response error: %v", err)
				return
			}

			c.mu.Lock()
			err = conn.WriteMessage(websocket.TextMessage, respData)
			c.mu.Unlock()

			if err != nil {
				log.Printf("[tunnel-client] send response error: %v", err)
				return
			}

			log.Printf("[tunnel-client] sent response: %s", truncate(response.Text, 50))
		}(msg)
	}
}

// processWithInber runs inber locally and returns the response.
func (c *Client) processWithInber(ctx context.Context, msg si.Message) si.Message {
	start := time.Now()

	// Build context prefix
	contextPrefix := ""
	if msg.Channel != "" {
		contextPrefix = "[from " + msg.Channel
		if msg.Author != "" {
			contextPrefix += " by " + msg.Author
		}
		contextPrefix += "] "
	}

	// Determine agent (empty = use default from agents.json)
	agent := ""
	if msg.Agent != "" {
		agent = msg.Agent
	}

	// Run inber — resume default session for context continuity
	args := []string{"run"}
	if agent != "" {
		args = append(args, "--agent", agent)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
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

	// Check for errors and parse metadata from stderr
	errData, _ := io.ReadAll(stderr)
	cmd.Wait()

	elapsed := time.Since(start)

	response := si.Message{
		Text:    string(output),
		Channel: msg.Channel,
		Author:  agent,
	}

	if len(output) == 0 && len(errData) > 0 {
		response.Text = string(errData)
	}

	// Parse metadata from stderr (token counts, cost, etc.)
	meta := parseStderrMeta(string(errData))
	if meta == nil {
		meta = &si.MessageMeta{}
	}
	meta.DurationMs = elapsed.Milliseconds()
	// Extract model from stderr if present
	if model := extractModel(string(errData)); model != "" {
		meta.Model = model
	}
	response.Meta = meta

	return response
}

// parseStderrMeta extracts token/cost stats from inber's stderr output.
// Expected format:
//
//	┌─ Tokens ──────────────────────
//	│ in=123  out=456  total=579  tools=2
//	│ cache: 100 read, 50 created
//	│ cost=$0.0042
//	└───────────────────────────────
func parseStderrMeta(stderr string) *si.MessageMeta {
	if stderr == "" {
		return nil
	}
	meta := &si.MessageMeta{}
	hasData := false

	for _, line := range splitLines(stderr) {
		line = trimSpace(line)
		// Token line: "│ in=123  out=456  total=579  tools=2"
		if containsStr(line, "in=") && containsStr(line, "out=") {
			fmt.Sscanf(extractAfter(line, "in="), "%d", &meta.InputTokens)
			fmt.Sscanf(extractAfter(line, "out="), "%d", &meta.OutputTokens)
			fmt.Sscanf(extractAfter(line, "tools="), "%d", &meta.ToolCalls)
			hasData = true
		}
		// Cache line: "│ cache: 100 read, 50 created"
		if containsStr(line, "cache:") {
			fmt.Sscanf(extractAfter(line, "cache: "), "%d", &meta.CacheReadTokens)
			if idx := indexStr(line, "read, "); idx >= 0 {
				fmt.Sscanf(line[idx+6:], "%d", &meta.CacheCreationTokens)
			}
		}
		// Cost line: "│ cost=$0.0042"
		if containsStr(line, "cost=$") {
			fmt.Sscanf(extractAfter(line, "cost=$"), "%f", &meta.Cost)
			hasData = true
		}
	}

	if !hasData {
		return nil
	}
	return meta
}

// extractModel parses the model name from stderr.
// Expected: "model: claude-sonnet-4-5 (provider=anthropic, openai=false)"
func extractModel(stderr string) string {
	for _, line := range splitLines(stderr) {
		line = trimSpace(line)
		if hasPrefix(line, "model:") {
			// Extract just the model name before the parentheses
			rest := trimSpace(line[6:])
			if idx := indexStr(rest, " ("); idx > 0 {
				return rest[:idx]
			}
			return rest
		}
	}
	return ""
}

// extractAfter returns the substring after the first occurrence of prefix.
func extractAfter(s, prefix string) string {
	idx := indexStr(s, prefix)
	if idx < 0 {
		return ""
	}
	return s[idx+len(prefix):]
}

// Helper functions to avoid importing strings package twice
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

func containsStr(s, substr string) bool {
	return indexStr(s, substr) >= 0
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
