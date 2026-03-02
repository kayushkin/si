package si

import "time"

// Message is the universal message format routed between adapters and inber feeds.
type Message struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Author    string    `json:"author"`
	Channel   string    `json:"channel"`   // adapter name or channel identifier
	ReplyTo   string    `json:"reply_to"`  // optional: message ID being replied to
	MediaURL  string    `json:"media_url"` // optional: attachment URL
	MediaData []byte    `json:"-"`         // optional: raw attachment bytes (not serialized)
	Timestamp time.Time `json:"timestamp"`
}
