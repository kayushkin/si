package feed

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

// LogstackClient sends logs to logstack service
type LogstackClient struct {
	baseURL string
	enabled bool
	client  *http.Client
}

// LogEntry matches logstack's expected format
type LogEntry struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`
	Agent     string                 `json:"agent,omitempty"`
	Channel   string                 `json:"channel,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Model     string                 `json:"model,omitempty"`
	Level     string                 `json:"level"`
	Type      string                 `json:"type"`
	Content   interface{}            `json:"content"`
	TokensIn  int                    `json:"tokens_in,omitempty"`
	TokensOut int                    `json:"tokens_out,omitempty"`
	LatencyMs int64                  `json:"latency_ms,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewLogstackClient creates a new logstack client
func NewLogstackClient() *LogstackClient {
	url := os.Getenv("LOGSTACK_URL")
	if url == "" {
		url = "http://localhost:8081"
	}

	// Check if logstack is available
	enabled := true
	client := &http.Client{Timeout: 2 * time.Second}

	// Quick health check
	resp, err := client.Get(url + "/api/v1/health")
	if err != nil {
		log.Printf("[logstack] service not available at %s, logging disabled", url)
		enabled = false
	} else {
		resp.Body.Close()
		log.Printf("[logstack] connected to %s", url)
	}

	return &LogstackClient{
		baseURL: url,
		enabled: enabled,
		client:  client,
	}
}

// Log sends a log entry to logstack (non-blocking, ignores errors)
func (c *LogstackClient) Log(entry LogEntry) {
	if !c.enabled {
		return
	}

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.Source == "" {
		entry.Source = "si"
	}

	// Send asynchronously
	go func() {
		data, err := json.Marshal(entry)
		if err != nil {
			return
		}

		resp, err := c.client.Post(c.baseURL+"/api/v1/logs", "application/json", bytes.NewReader(data))
		if err != nil {
			return
		}
		resp.Body.Close()
	}()
}

// LogMessage logs a message event
func (c *LogstackClient) LogMessage(channel, model, content string, latencyMs int64) {
	c.Log(LogEntry{
		Level:     "info",
		Type:      "message",
		Channel:   channel,
		Model:     model,
		Content:   content,
		LatencyMs: latencyMs,
	})
}

// LogError logs an error event
func (c *LogstackClient) LogError(channel, model, errMsg string) {
	c.Log(LogEntry{
		Level:   "error",
		Type:    "error",
		Channel: channel,
		Model:   model,
		Error:   errMsg,
	})
}

// LogRouting logs a routing decision
func (c *LogstackClient) LogRouting(channel, agent, model string) {
	c.Log(LogEntry{
		Level:   "info",
		Type:    "routing",
		Channel: channel,
		Agent:   agent,
		Model:   model,
		Content: map[string]string{"action": "route", "agent": agent},
	})
}
