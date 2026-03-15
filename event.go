package si

// EventType identifies the direction of a routed message.
type EventType string

const (
	EventInbound  EventType = "inbound"  // adapter → feed
	EventOutbound EventType = "outbound" // feed → adapters
	EventGateway  EventType = "gateway"  // gateway lifecycle events (spawn, session status)
)

// Event wraps a message with its routing direction.
type Event struct {
	Type    EventType `json:"type"`
	Message Message   `json:"message"`
}
