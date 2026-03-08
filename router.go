package si

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
)

// Feed is the interface for reading/writing to the message bus.
type Feed interface {
	Write(msg Message) error
	Read() <-chan Message
	Close() error
}

// Router shuffles messages between adapters and the feed.
// Stateless — no history, no session tracking. Just routing.
type Router struct {
	feed     Feed
	adapters []Adapter
	mu       sync.RWMutex

	// Event bus for live subscribers (e.g., WebSocket broadcast)
	subscribers map[chan Event]bool
	subMu       sync.RWMutex

	// Optional logstack for persistent event logging.
	logstack *LogstackClient
}

// NewRouter creates a router connected to the given feed.
func NewRouter(feed Feed) *Router {
	return &Router{
		feed:        feed,
		subscribers: make(map[chan Event]bool),
		logstack:    NewLogstackClient(),
	}
}

// AddAdapter registers an adapter with the router.
func (r *Router) AddAdapter(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters = append(r.adapters, a)
}

// Subscribe returns a channel that receives all routed events.
func (r *Router) Subscribe() <-chan Event {
	ch := make(chan Event, 128)
	r.subMu.Lock()
	r.subscribers[ch] = true
	r.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (r *Router) Unsubscribe(ch <-chan Event) {
	r.subMu.Lock()
	defer r.subMu.Unlock()
	for sub := range r.subscribers {
		if sub == ch {
			delete(r.subscribers, sub)
			close(sub)
			return
		}
	}
}

// publish sends an event to all live subscribers and logs to logstack.
// Stream deltas are forwarded to subscribers but NOT logged (only "done" messages are logged).
func (r *Router) publish(e Event) {
	// Log to logstack for persistent history — skip stream deltas.
	if r.logstack != nil && e.Message.Stream != "delta" {
		r.logstack.LogEvent(e)
	}

	r.subMu.RLock()
	defer r.subMu.RUnlock()
	for ch := range r.subscribers {
		select {
		case ch <- e:
		default:
			// drop if subscriber is slow
		}
	}
}

// Adapters returns the current list of adapters.
func (r *Router) Adapters() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Adapter, len(r.adapters))
	copy(out, r.adapters)
	return out
}

// Run starts all adapters and begins routing messages.
// Blocks until ctx is cancelled.
func (r *Router) Run(ctx context.Context) error {
	r.mu.RLock()
	adapters := make([]Adapter, len(r.adapters))
	copy(adapters, r.adapters)
	r.mu.RUnlock()

	// Start all adapters.
	for _, a := range adapters {
		a := a
		go func() {
			if err := a.Start(ctx); err != nil {
				log.Printf("[router] adapter %s stopped: %v", a.Name(), err)
			}
		}()
	}

	// Route inbound: adapters → feed (bus)
	for _, a := range adapters {
		a := a
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-a.Receive():
					if !ok {
						return
					}
					log.Printf("[router] %s → bus: %s", a.Name(), truncate(msg.Text, 80))
					r.publish(Event{Type: EventInbound, Message: msg})
					if err := r.feed.Write(msg); err != nil {
						log.Printf("[router] feed write error: %v", err)
					}
				}
			}
		}()
	}

	// Route outbound: feed (bus) → matching adapters
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-r.feed.Read():
				if !ok {
					return
				}
				r.publish(Event{Type: EventOutbound, Message: msg})
				r.mu.RLock()
				for _, a := range r.adapters {
					if matchAdapter(msg.Channel, a.Name()) {
						if err := a.Send(msg); err != nil {
							log.Printf("[router] send to %s failed: %v", a.Name(), err)
						}
					}
				}
				r.mu.RUnlock()
			}
		}
	}()

	<-ctx.Done()
	return fmt.Errorf("router stopped: %w", ctx.Err())
}

// matchAdapter checks if a message's channel targets this adapter.
// Empty channel = broadcast to all.
// "discord:12345" matches adapter "discord".
func matchAdapter(channel, adapterName string) bool {
	if channel == "" {
		return true
	}
	return channel == adapterName || strings.HasPrefix(channel, adapterName+":")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
