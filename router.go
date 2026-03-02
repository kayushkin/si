package si

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Feed is the interface for reading/writing to inber's I/O.
// The concrete implementation depends on how inber exposes its sessions
// (stdin/stdout pipes, unix socket, HTTP, etc).
type Feed interface {
	// Write sends a message to inber.
	Write(msg Message) error

	// Read returns a channel of messages from inber.
	Read() <-chan Message

	// Close shuts down the feed connection.
	Close() error
}

// Router shuffles messages between adapters and the inber feed.
type Router struct {
	feed     Feed
	adapters []Adapter
	mu       sync.RWMutex
}

// NewRouter creates a router connected to the given inber feed.
func NewRouter(feed Feed) *Router {
	return &Router{
		feed: feed,
	}
}

// AddAdapter registers an adapter with the router.
func (r *Router) AddAdapter(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters = append(r.adapters, a)
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
				r.mu.RLock()
				for _, a := range r.adapters {
					// Route to the originating adapter, or broadcast if no channel specified
					if msg.Channel == "" || msg.Channel == a.Name() {
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
