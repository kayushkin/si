package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Adapter runs a WebSocket server for direct clients (e.g. Claxon Android).
type Adapter struct {
	addr     string
	incoming chan si.Message
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
}

// New creates a WebSocket adapter listening on addr (e.g. ":8090").
func New(addr string) *Adapter {
	return &Adapter{
		addr:     addr,
		incoming: make(chan si.Message, 64),
		clients:  make(map[*websocket.Conn]bool),
	}
}

func (a *Adapter) Name() string { return "websocket" }

// Start runs the WebSocket server.
func (a *Adapter) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", a.handleWS)

	srv := &http.Server{Addr: a.addr, Handler: mux}

	go func() {
		<-ctx.Done()
		srv.Close()
		close(a.incoming)
	}()

	log.Printf("[websocket] listening on %s", a.addr)
	return srv.ListenAndServe()
}

func (a *Adapter) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[websocket] upgrade error: %v", err)
		return
	}

	a.mu.Lock()
	a.clients[conn] = true
	a.mu.Unlock()

	log.Printf("[websocket] client connected: %s", conn.RemoteAddr())

	defer func() {
		a.mu.Lock()
		delete(a.clients, conn)
		a.mu.Unlock()
		conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[websocket] read error: %v", err)
			return
		}

		var msg si.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			// Treat as plain text
			msg = si.Message{
				Text:    string(data),
				Channel: "websocket",
			}
		}

		if msg.ID == "" {
			msg.ID = uuid.New().String()
		}
		if msg.Channel == "" {
			msg.Channel = "websocket"
		}
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}

		a.incoming <- msg
	}
}

// Send broadcasts a message to all connected WebSocket clients.
func (a *Adapter) Send(msg si.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	for conn := range a.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[websocket] write error: %v", err)
		}
	}
	return nil
}

// Receive returns the channel of inbound messages.
func (a *Adapter) Receive() <-chan si.Message {
	return a.incoming
}
