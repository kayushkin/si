package si

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Feed is the interface for reading/writing to inber's I/O.
type Feed interface {
	Write(msg Message) error
	Read() <-chan Message
	Close() error
}

// Router shuffles messages between adapters and the inber feed.
type Router struct {
	feed     Feed
	adapters []Adapter
	mu       sync.RWMutex

	// Event bus
	subscribers map[chan Event]bool
	subMu       sync.RWMutex

	// History
	History *History
}

// NewRouter creates a router connected to the given inber feed.
func NewRouter(feed Feed) *Router {
	home, _ := os.UserHomeDir()
	histPath := filepath.Join(home, ".inber", "si-history.jsonl")

	return &Router{
		feed:        feed,
		subscribers: make(map[chan Event]bool),
		History:     NewHistory(histPath, 500),
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
	// Find the matching chan by iterating
	for sub := range r.subscribers {
		if sub == ch {
			delete(r.subscribers, sub)
			close(sub)
			return
		}
	}
}

// publish sends an event to all subscribers and records it in history.
func (r *Router) publish(e Event) {
	r.History.Add(e)

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

	// Start all adapters
	for _, a := range adapters {
		a := a
		go func() {
			if err := a.Start(ctx); err != nil {
				log.Printf("[router] adapter %s stopped: %v", a.Name(), err)
			}
		}()
	}

	// Route inbound: adapters → feed
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
					log.Printf("[router] %s → inber: %s", a.Name(), truncate(msg.Text, 80))
					r.publish(Event{Type: EventInbound, Message: msg})
					if err := r.feed.Write(msg); err != nil {
						log.Printf("[router] feed write error: %v", err)
					}
				}
			}
		}()
	}

	// Route outbound: feed → all adapters
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
					if msg.Channel == "" || msg.Channel == a.Name() || strings.HasPrefix(msg.Channel, a.Name()+":") {
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
	r.History.Close()
	return fmt.Errorf("router stopped: %w", ctx.Err())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
