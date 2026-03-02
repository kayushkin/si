package feed

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// API exposes a WebSocket endpoint for inber to connect to.
// Inber sends responses (outbound), sí pushes incoming messages (inbound).
//
// Protocol (JSON over WebSocket):
//
// inber → sí (response from agent):
//
//	{"text": "hey what's up"}
//
// sí → inber (message from user):
//
//	{"text": "hello", "origin": "discord:143132977210195968", "author": "DeandreT"}
type API struct {
	addr    string
	inbound chan si.Message // messages from adapters → waiting for inber to consume

	// outbound: inber's responses → route back to origin
	outbound chan si.Message

	// origin tracking: last origin per session (for now, single session)
	lastOrigin string
	mu         sync.RWMutex

	conn *websocket.Conn // current inber connection
	connMu sync.Mutex
}

// NewAPI creates an API feed listening on addr (e.g. ":8091").
func NewAPI(addr string) *API {
	return &API{
		addr:     addr,
		inbound:  make(chan si.Message, 64),
		outbound: make(chan si.Message, 64),
	}
}

// Start runs the API server. Call this in a goroutine before Router.Run.
func (a *API) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", a.handleFeed)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		a.connMu.Lock()
		connected := a.conn != nil
		a.connMu.Unlock()

		status := map[string]interface{}{
			"ok":        true,
			"connected": connected,
		}
		a.mu.RLock()
		status["last_origin"] = a.lastOrigin
		a.mu.RUnlock()

		json.NewEncoder(w).Encode(status)
	})

	srv := &http.Server{Addr: a.addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	log.Printf("[feed/api] listening on %s", a.addr)
	return srv.ListenAndServe()
}

func (a *API) handleFeed(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[feed/api] upgrade error: %v", err)
		return
	}

	a.connMu.Lock()
	old := a.conn
	a.conn = conn
	a.connMu.Unlock()

	if old != nil {
		old.Close() // only one inber connection at a time
	}

	log.Printf("[feed/api] inber connected from %s", conn.RemoteAddr())

	// Push inbound messages (from adapters) to inber
	go func() {
		for msg := range a.inbound {
			data, _ := json.Marshal(msg)
			a.connMu.Lock()
			c := a.conn
			a.connMu.Unlock()
			if c != nil {
				if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
					log.Printf("[feed/api] write to inber error: %v", err)
				}
			}
		}
	}()

	// Read responses from inber
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[feed/api] inber disconnected: %v", err)
			a.connMu.Lock()
			if a.conn == conn {
				a.conn = nil
			}
			a.connMu.Unlock()
			return
		}

		var msg si.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[feed/api] bad message from inber: %v", err)
			continue
		}

		// If inber didn't specify a channel, use last origin
		if msg.Channel == "" {
			a.mu.RLock()
			msg.Channel = a.lastOrigin
			a.mu.RUnlock()
		}

		if msg.Author == "" {
			msg.Author = "inber"
		}

		a.outbound <- msg
	}
}

// Write sends an inbound message to inber (from adapter → inber).
// Also tracks the origin for routing responses back.
func (a *API) Write(msg si.Message) error {
	// Track origin
	origin := msg.Channel
	if origin != "" {
		a.mu.Lock()
		a.lastOrigin = origin
		a.mu.Unlock()
	}

	a.inbound <- msg
	return nil
}

// Read returns outbound messages from inber (inber → adapters).
func (a *API) Read() <-chan si.Message {
	return a.outbound
}

func (a *API) Close() error {
	a.connMu.Lock()
	if a.conn != nil {
		a.conn.Close()
	}
	a.connMu.Unlock()
	close(a.inbound)
	return nil
}
