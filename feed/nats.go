package feed

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/kayushkin/bus"
	si "github.com/kayushkin/si"
)

// NATSFeed connects si to the NATS message bus.
// All inbound messages publish to "inbound" topic.
// Subscribes to "outbound" topic for agent responses.
type NATSFeed struct {
	client   *bus.Client
	inbound  chan si.Message
	ctx      context.Context
	cancel   context.CancelFunc
	consumer string
}

// NATSFeedConfig configures the NATS feed.
type NATSFeedConfig struct {
	NATSURL  string // e.g., "nats://localhost:4222"
	Consumer string // consumer ID for this si instance
}

// NewNATSFeed creates a feed that connects to the NATS message bus.
func NewNATSFeed(cfg NATSFeedConfig) *NATSFeed {
	ctx, cancel := context.WithCancel(context.Background())

	consumer := cfg.Consumer
	if consumer == "" {
		consumer = "si"
	}

	return &NATSFeed{
		inbound:  make(chan si.Message, 64),
		ctx:      ctx,
		cancel:   cancel,
		consumer: consumer,
	}
}

// Start connects to NATS and begins processing.
func (f *NATSFeed) Start(cfg NATSFeedConfig) error {
	natsURL := cfg.NATSURL
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	client, err := bus.Connect(bus.Options{
		URL:  natsURL,
		Name: f.consumer,
	})
	if err != nil {
		return err
	}
	f.client = client

	// Subscribe to outbound messages from agents
	subjects := []string{"outbound", "events", "gateway"}
	for _, subject := range subjects {
		_, err := f.client.Subscribe(subject, f.handleMessage)
		if err != nil {
			return err
		}
	}

	log.Printf("[feed/nats] connected to %s as %s", natsURL, f.consumer)
	return nil
}

// Write publishes a message to the "inbound" topic.
func (f *NATSFeed) Write(msg si.Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if err := f.client.Publish("inbound", payload); err != nil {
		return err
	}

	log.Printf("[feed/nats] → inbound [%s] %s: %s",
		msg.Channel, msg.Author, truncateNATS(msg.Text, 50))
	return nil
}

// Read returns the channel of responses from agents.
func (f *NATSFeed) Read() <-chan si.Message {
	return f.inbound
}

// Close shuts down the feed.
func (f *NATSFeed) Close() error {
	f.cancel()
	if f.client != nil {
		f.client.Close()
	}
	return nil
}

// handleMessage processes incoming messages from NATS.
func (f *NATSFeed) handleMessage(subject string, payload []byte) {
	// Gateway and events topics get forwarded as-is for dashboard consumption.
	if subject == "gateway" || subject == "events" {
		log.Printf("[feed/nats] ← %s event", subject)
		evMsg := si.Message{
			Text:      string(payload),
			Channel:   subject, // "gateway" or "events"
			Timestamp: time.Now(),
		}
		select {
		case f.inbound <- evMsg:
		default:
			// drop if channel is full
		}
		return
	}

	var msg si.Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		log.Printf("[feed/nats] payload unmarshal error: %v", err)
		return
	}

	log.Printf("[feed/nats] ← outbound [%s] %s", msg.Channel, truncateNATS(msg.Text, 50))
	
	select {
	case f.inbound <- msg:
	case <-f.ctx.Done():
		return
	default:
		// drop if channel is full
	}
}

func truncateNATS(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}