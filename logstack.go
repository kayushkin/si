package si

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

// LogstackClient sends message events to logstack for persistent history.
type LogstackClient struct {
	baseURL string
	enabled bool
	client  *http.Client
}

// LogEntry matches logstack's expected format.
type LogEntry struct {
	ID        string      `json:"id"`
	Timestamp time.Time   `json:"timestamp"`
	Source    string      `json:"source"`
	Agent     string      `json:"agent,omitempty"`
	Channel   string      `json:"channel,omitempty"`
	Level     string      `json:"level"`
	Type      string      `json:"type"`
	Content   interface{} `json:"content"`
}

// NewLogstackClient creates a client. Uses LOGSTACK_URL env (default http://localhost:8088).
// Does a quick health check; disables itself if logstack is unreachable.
func NewLogstackClient() *LogstackClient {
	url := os.Getenv("LOGSTACK_URL")
	if url == "" {
		url = "http://localhost:8088"
	}

	client := &http.Client{Timeout: 2 * time.Second}
	enabled := true

	resp, err := client.Get(url + "/api/v1/health")
	if err != nil {
		log.Printf("[logstack] not available at %s, logging disabled", url)
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

// LogEvent logs a routed event (inbound or outbound message) asynchronously.
func (c *LogstackClient) LogEvent(e Event) {
	if !c.enabled {
		return
	}

	content := map[string]interface{}{
		"text":       e.Message.Text,
		"author":     e.Message.Author,
		"channel":    e.Message.Channel,
		"message_id": e.Message.ID,
	}
	if e.Message.Meta != nil {
		content["meta"] = e.Message.Meta
	}

	entry := LogEntry{
		ID:        uuid.New().String(),
		Timestamp: e.Message.Timestamp,
		Source:    "si",
		Agent:     e.Message.Author,
		Channel:   e.Message.Channel,
		Level:     "info",
		Type:      string(e.Type),
		Content:   content,
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

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
