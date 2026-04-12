package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/kayushkin/bus"
	"github.com/kayushkin/bus/messages"
	si "github.com/kayushkin/si"
)

// NatsFeed connects si to NATS as a stateless protocol adapter.
// Publishes inbound messages to chat.inbound.<orchestrator>.
// Subscribes to chat.stream for streaming events and chat.outbound for completed responses.
type NatsFeed struct {
	client   *bus.Client
	inbound  chan si.Message
	outbound chan si.Message
	ctx      context.Context
	cancel   context.CancelFunc
}

type NatsFeedConfig struct {
	NatsURL string // default nats://localhost:4222
}

func NewNatsFeed(cfg NatsFeedConfig) (*NatsFeed, error) {
	ctx, cancel := context.WithCancel(context.Background())

	client, err := bus.Connect(bus.Options{
		URL:  cfg.NatsURL,
		Name: "si",
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	return &NatsFeed{
		client:   client,
		inbound:  make(chan si.Message, 64),
		outbound: make(chan si.Message, 64),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

func (f *NatsFeed) Start() error {
	// Subscribe to chat.stream for streaming events (text, thinking, tool, done, etc.)
	_, err := f.client.Subscribe("chat.stream", func(subject string, data []byte) {
		var delta messages.ChatDelta
		if err := json.Unmarshal(data, &delta); err != nil {
			log.Printf("[feed/nats] stream unmarshal error: %v", err)
			return
		}

		msg := si.Message{
			Agent:        delta.Agent,
			Orchestrator: delta.Orchestrator,
			Timestamp:    time.Now(),
		}

		switch delta.Type {
		case "text":
			msg.Text = delta.Text
			msg.Stream = "delta"
			msg.StreamID = delta.SessionID
		case "thinking":
			msg.Text = delta.Text
			msg.Stream = "thinking"
			msg.StreamID = delta.SessionID
		case "tool":
			msg.Text = delta.Text
			msg.Stream = "tool_call"
			msg.StreamID = delta.SessionID
			if delta.Tool != "" {
				msg.Meta = &si.MessageMeta{
					Tools: []si.ToolEvent{{Tool: delta.Tool, Input: delta.ToolInput}},
				}
			}
		case "tool_result":
			msg.Text = delta.Text
			msg.Stream = "tool_result"
			msg.StreamID = delta.SessionID
			if delta.Tool != "" {
				msg.Meta = &si.MessageMeta{
					Tools: []si.ToolEvent{{Tool: delta.Tool, Output: delta.ToolOutput}},
				}
			}
		case "done":
			msg.Stream = "done"
			msg.StreamID = delta.SessionID
			if delta.Stats != nil {
				msg.Meta = &si.MessageMeta{
					InputTokens:  delta.Stats.InputTokens,
					OutputTokens: delta.Stats.OutputTokens,
					Cost:         delta.Stats.Cost,
					DurationMs:   int64(delta.Stats.DurationMs),
					Model:        delta.Stats.Model,
					Turn:         delta.Stats.Turn,
					ToolCalls:    delta.Stats.ToolCalls,
				}
			}
		default:
			// Forward other event types as-is
			msg.Text = string(data)
			msg.Channel = delta.Type
		}

		log.Printf("[feed/nats] ← stream %s [%s/%s]", delta.Type, delta.Agent, delta.Orchestrator)
		f.inbound <- msg
	})
	if err != nil {
		return fmt.Errorf("subscribe chat.stream: %w", err)
	}

	// Subscribe to chat.outbound via JetStream for completed responses
	err = f.client.JetSubscribe("chat.outbound", "si", func(subject string, data []byte) {
		var out messages.ChatOutbound
		if err := json.Unmarshal(data, &out); err != nil {
			log.Printf("[feed/nats] outbound unmarshal error: %v", err)
			return
		}

		msg := si.Message{
			Text:         out.Text,
			Author:       out.Agent,
			Agent:        out.Agent,
			Orchestrator: out.Orchestrator,
			Timestamp:    out.Timestamp,
		}
		if out.Stats != nil {
			msg.Meta = &si.MessageMeta{
				InputTokens:  out.Stats.InputTokens,
				OutputTokens: out.Stats.OutputTokens,
				Cost:         out.Stats.Cost,
				DurationMs:   int64(out.Stats.DurationMs),
				Model:        out.Stats.Model,
				Turn:         out.Stats.Turn,
				ToolCalls:    out.Stats.ToolCalls,
			}
		}

		log.Printf("[feed/nats] ← outbound [%s] %s", out.Agent, truncateNats(out.Text, 50))
		f.inbound <- msg
	})
	if err != nil {
		return fmt.Errorf("jetsubscribe chat.outbound: %w", err)
	}

	// Subscribe to health.> for healthcheck events
	_, err = f.client.Subscribe("health.>", func(subject string, data []byte) {
		msg := si.Message{
			Text:      string(data),
			Channel:   "events",
			Timestamp: time.Now(),
		}
		log.Printf("[feed/nats] ← %s event", subject)
		f.inbound <- msg
	})
	if err != nil {
		return fmt.Errorf("subscribe health.>: %w", err)
	}

	go f.publishLoop()

	log.Printf("[feed/nats] connected and subscribed")
	return nil
}

func (f *NatsFeed) publishLoop() {
	for {
		select {
		case <-f.ctx.Done():
			return
		case msg := <-f.outbound:
			orchestrator := msg.Orchestrator
			if orchestrator == "" {
				orchestrator = "claude-code"
			}
			subject := "chat.inbound." + orchestrator

			inbound := messages.ChatInbound{
				Text:         msg.Text,
				Author:       msg.Author,
				Agent:        msg.Agent,
				Orchestrator: orchestrator,
				Channel:      msg.Channel,
				Timestamp:    time.Now(),
			}

			if err := f.client.Publish(subject, inbound); err != nil {
				log.Printf("[feed/nats] publish error: %v", err)
				continue
			}

			log.Printf("[feed/nats] → %s [%s] %s: %s",
				subject, msg.Channel, msg.Author, truncateNats(msg.Text, 50))
		}
	}
}

func (f *NatsFeed) Write(msg si.Message) error {
	select {
	case f.outbound <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("publish timeout")
	}
}

func (f *NatsFeed) Read() <-chan si.Message {
	return f.inbound
}

func (f *NatsFeed) Close() error {
	f.cancel()
	f.client.Close()
	return nil
}

func truncateNats(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
