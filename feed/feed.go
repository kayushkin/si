package feed

import si "github.com/kayushkin/si"

// Stdio implements Feed by reading/writing to an inber process's stdin/stdout.
// This is the initial implementation — will evolve as inber's session server
// exposes proper feed endpoints (unix socket, HTTP SSE, etc).
type Stdio struct {
	outbound chan si.Message
	inbound  chan si.Message
}

// NewStdio creates a stdio-based feed.
// For now this is a placeholder — the concrete pipe wiring depends on
// how inber exposes its session I/O.
func NewStdio() *Stdio {
	return &Stdio{
		outbound: make(chan si.Message, 64),
		inbound:  make(chan si.Message, 64),
	}
}

func (f *Stdio) Write(msg si.Message) error {
	f.outbound <- msg
	return nil
}

func (f *Stdio) Read() <-chan si.Message {
	return f.inbound
}

// Outbound returns messages heading to inber (for the pipe writer to consume).
func (f *Stdio) Outbound() <-chan si.Message {
	return f.outbound
}

// Deliver pushes a message from inber into the feed (for the pipe reader to call).
func (f *Stdio) Deliver(msg si.Message) {
	f.inbound <- msg
}

func (f *Stdio) Close() error {
	close(f.outbound)
	close(f.inbound)
	return nil
}
