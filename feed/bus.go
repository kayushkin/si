package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

// BusFeed connects si to the message bus.
// Inbound messages from adapters are published to the bus.
// Outbound responses from agents are received via subscription.
type BusFeed struct {
	busURL    string
	wsURL     string
	token     string
	consumer  string
	inbound   chan si.Message
	outbound  chan si.Message
	ctx       context.Context
	cancel    context.CancelFunc
	http      *http.Client
}

// BusFeedConfig configures the bus feed.
type BusFeedConfig struct {
	BusURL   string // e.g., "https://kayushkin.com/bus" or "http://localhost:8100"
	Token    string // bus auth token
	Consumer string // consumer ID for this si instance
}

// NewBusFeed creates a feed that connects to the message bus.
func NewBusFeed(cfg BusFeedConfig) *BusFeed {
	ctx, cancel := context.WithCancel(context.Background())

	wsURL := strings.Replace(cfg.BusURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	consumer := cfg.Consumer
	if consumer == "" {
		consumer = "si-server"
	}

	return &BusFeed{
		busURL:   cfg.BusURL,
		wsURL:    wsURL,
		token:    cfg.Token,
		consumer: consumer,
		inbound:  make(chan si.Message, 64),
		outbound: make(chan si.Message, 64),
		ctx:      ctx,
		cancel:   cancel,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Start connects to the bus and begins processing.
func (f *BusFeed) Start() error {
	// Subscribe to agent responses via WebSocket.
	go f.subscribeLoop()

	// Forward outbound messages (from adapters) to the bus.
	go f.publishLoop()

	log.Printf("[feed/bus] connected to %s as %s", f.busURL, f.consumer)
	return nil
}

// Write queues a message to be published to the bus.
func (f *BusFeed) Write(msg si.Message) error {
	select {
	case f.outbound <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("publish timeout")
	}
}

// Read returns the channel of responses from agents.
func (f *BusFeed) Read() <-chan si.Message {
	return f.inbound
}

// Close shuts down the feed.
func (f *BusFeed) Close() error {
	f.cancel()
	return nil
}

// publishLoop publishes outbound messages to the bus via HTTP.
func (f *BusFeed) publishLoop() {
	for {
		select {
		case <-f.ctx.Done():
			return
		case msg := <-f.outbound:
			topic := "inbound." + msg.Agent
			if msg.Agent == "" {
				topic = "inbound.default"
			}

			payload, _ := json.Marshal(msg)

			body := map[string]interface{}{
				"topic":   topic,
				"payload": json.RawMessage(payload),
				"source":  "si",
			}
			data, _ := json.Marshal(body)

			url := f.busURL + "/publish?token=" + f.token
			resp, err := f.http.Post(url, "application/json", bytes.NewReader(data))
			if err != nil {
				log.Printf("[feed/bus] publish error: %v", err)
				continue
			}
			resp.Body.Close()

			log.Printf("[feed/bus] published to %s: %s", topic, truncateBus(msg.Text, 50))
		}
	}
}

// subscribeLoop connects to the bus WebSocket and receives agent responses.
func (f *BusFeed) subscribeLoop() {
	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		if err := f.subscribe(); err != nil {
			log.Printf("[feed/bus] subscribe error: %v, reconnecting in 3s...", err)
			select {
			case <-f.ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}
}

func (f *BusFeed) subscribe() error {
	url := fmt.Sprintf("%s/subscribe?consumer=%s&topics=outbound.*,outbound.default&token=%s",
		f.wsURL, f.consumer, f.token)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("[feed/bus] subscribed to outbound topics")

	for {
		select {
		case <-f.ctx.Done():
			return nil
		default:
		}

		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		// Bus message wraps our si.Message in payload.
		var busMsg struct {
			ID      int64           `json:"id"`
			Topic   string          `json:"topic"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(data, &busMsg); err != nil {
			log.Printf("[feed/bus] unmarshal error: %v", err)
			continue
		}

		var msg si.Message
		if err := json.Unmarshal(busMsg.Payload, &msg); err != nil {
			log.Printf("[feed/bus] payload unmarshal error: %v", err)
			continue
		}

		log.Printf("[feed/bus] received from bus: %s", truncateBus(msg.Text, 50))
		f.inbound <- msg

		// Ack the message.
		go f.ack(busMsg.Topic, busMsg.ID)
	}
}

func (f *BusFeed) ack(topic string, id int64) {
	body := map[string]interface{}{
		"consumer":   f.consumer,
		"topic":      topic,
		"message_id": id,
	}
	data, _ := json.Marshal(body)
	url := f.busURL + "/ack?token=" + f.token
	resp, err := f.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("[feed/bus] ack error: %v", err)
		return
	}
	resp.Body.Close()
}

// fetchHistory gets recent messages for catch-up.
func (f *BusFeed) fetchHistory(topic string, limit int) ([]si.Message, error) {
	url := fmt.Sprintf("%s/history?topic=%s&limit=%d&token=%s",
		f.busURL, topic, limit, f.token)

	resp, err := f.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, body)
	}

	var busMessages []struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&busMessages); err != nil {
		return nil, err
	}

	var msgs []si.Message
	for _, bm := range busMessages {
		var msg si.Message
		if err := json.Unmarshal(bm.Payload, &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func truncateBus(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
