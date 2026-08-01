package si

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	logstackclient "github.com/kayushkin/logstack/client"
	logstackmodels "github.com/kayushkin/logstack/models"
)

// LogstackClient sends message events to logstack for persistent history.
type LogstackClient struct {
	baseURL string
	enabled bool
	client  *logstackclient.Client
}

// NewLogstackClient creates a client. Uses LOGSTACK_URL env (default http://localhost:8088).
// Does a quick health check; disables itself if logstack is unreachable.
func NewLogstackClient() *LogstackClient {
	url := os.Getenv("LOGSTACK_URL")
	if url == "" {
		url = "http://localhost:8088"
	}

	enabled := true
	probe := &http.Client{Timeout: 2 * time.Second}
	resp, err := probe.Get(url + "/api/v1/health")
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
		client:  logstackclient.New(url),
	}
}

// entryType translates a routed event into the log type logstack files it under.
//
// These are two different vocabularies that happen to share a word. si's
// EventType is a *routing direction* — EventOutbound means the message
// travelled feed → adapters, whatever the message was. logstack's
// models.TypeOutbound means *a completed agent turn*, and it is the only type
// its usage readers select: Usage and MaxUsage query TypeOutbound and nothing
// else, so everything filed under it is read as billable conversation.
//
// Passing si's own string straight through made every plumbing message a turn.
// The feed republishes healthcheck status changes (feed/nats.go's health.>
// subscription and feed/bus.go's "events" topic) as ordinary messages on the
// "events" channel with no agent, no author and no orchestrator; the router
// then routes them feed → adapters, which is EventOutbound. Replaying the live
// store through this function: of the 145,813 entries si has filed as outbound,
// 138,441 are that shape — 94.9% of the bucket — and they still arrive at
// roughly one a minute. The other 7,372 are real turns and keep their bucket. They carry no tokens, so they add no
// dollars, but every one of them is a row the usage readers scan and a row any
// reader that counts entries rather than tokens will count. MaxUsage derives
// api_calls exactly that way.
//
// So a routed message earns an inbound/outbound bucket only when it names who
// spoke. That is the same rule logstack already enforces on its own NATS
// ingest path, where the chat.outbound subscriber drops any message with an
// empty agent (cmd/logstack/main.go). Everything else is plumbing and belongs
// in TypeRouting.
func entryType(e Event) string {
	if e.Type == EventGateway {
		return logstackmodels.TypeLifecycle
	}
	if speakerOf(e.Message) == "" {
		return logstackmodels.TypeRouting
	}
	if e.Type == EventInbound {
		return logstackmodels.TypeInbound
	}
	return logstackmodels.TypeOutbound
}

// speakerOf names whoever the message is attributed to, preferring the target
// agent over the human author. Empty means nothing identifies a speaker, which
// is what tells a routed chat turn apart from republished plumbing.
func speakerOf(m Message) string {
	if m.Agent != "" {
		return m.Agent
	}
	return m.Author
}

// LogEvent logs a routed event (inbound or outbound message) asynchronously.
func (c *LogstackClient) LogEvent(e Event) {
	if !c.enabled {
		return
	}

	entry := buildEntry(e)

	go func() {
		if err := c.client.Log(entry); err != nil {
			log.Printf("[logstack] failed to log %s entry: %v", entry.Type, err)
		}
	}()
}

// buildEntry renders a routed event as the log entry logstack will store.
func buildEntry(e Event) logstackmodels.LogEntry {
	content := map[string]interface{}{
		"text":         e.Message.Text,
		"author":       e.Message.Author,
		"agent":        e.Message.Agent,
		"orchestrator": e.Message.Orchestrator,
		"channel":      e.Message.Channel,
		"message_id":   e.Message.ID,
	}
	if e.Message.Meta != nil {
		content["meta"] = e.Message.Meta
	}

	entry := logstackmodels.LogEntry{
		ID:        uuid.New().String(),
		Timestamp: e.Message.Timestamp,
		// logstack files an entry under its orchestrator and both usage readers
		// group by it. si has always known it and had nowhere to put it: this
		// struct used to be a hand-written copy of logstack's format carrying a
		// Source field logstack has no place for and omitting Orchestrator
		// entirely. It knew the orchestrator on 7,371 of its 7,388 attributed
		// entries and sent it only inside Content, so every entry si ever wrote
		// landed in unknown.jsonl and by_orchestrator came back empty for the
		// whole box.
		Orchestrator: e.Message.Orchestrator,
		Agent:        speakerOf(e.Message),
		Channel:      e.Message.Channel,
		Level:        logstackmodels.LevelInfo,
		Type:         entryType(e),
		Content:      content,
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	return entry
}
