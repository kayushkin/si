package si

import "context"

// Adapter is the interface all communication adapters implement.
// Each adapter connects one external platform to the router.
type Adapter interface {
	// Name returns the adapter identifier (e.g. "matterbridge", "tui", "websocket").
	Name() string

	// Start begins the adapter's event loop. Blocks until ctx is cancelled.
	Start(ctx context.Context) error

	// Send delivers a message to the external platform.
	Send(msg Message) error

	// Receive returns a channel of inbound messages from the platform.
	Receive() <-chan Message
}
