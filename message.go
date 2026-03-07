package si

import "time"

// Message is the universal message format routed between adapters and inber feeds.
type Message struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Author    string    `json:"author"`
	Agent        string    `json:"agent"`        // optional: target agent for the message
	Orchestrator string    `json:"orchestrator"` // optional: target orchestrator/backend
	Channel      string    `json:"channel"`      // adapter name or channel identifier
	ReplyTo   string    `json:"reply_to"`  // optional: message ID being replied to
	MediaURL  string    `json:"media_url"` // optional: attachment URL
	MediaData []byte    `json:"-"`         // optional: raw attachment bytes (not serialized)
	Timestamp time.Time `json:"timestamp"`

	// Metadata from inber (omitted when zero)
	Meta *MessageMeta `json:"meta,omitempty"`
}

// MessageMeta holds optional stats from an inber turn.
type MessageMeta struct {
	InputTokens         int     `json:"input_tokens,omitempty"`
	OutputTokens        int     `json:"output_tokens,omitempty"`
	CacheReadTokens     int     `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int     `json:"cache_creation_tokens,omitempty"`
	ToolCalls           int     `json:"tool_calls,omitempty"`
	Cost                float64 `json:"cost,omitempty"`
	DurationMs          int64   `json:"duration_ms,omitempty"`
	Model               string  `json:"model,omitempty"`
	Turn                int     `json:"turn,omitempty"`
}
