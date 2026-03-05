package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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

	router *si.Router
}

// New creates a WebSocket adapter listening on addr (e.g. ":8090").
func New(addr string) *Adapter {
	return &Adapter{
		addr:     addr,
		incoming: make(chan si.Message, 64),
		clients:  make(map[*websocket.Conn]bool),
	}
}

// SetRouter gives the adapter access to the router for history and event bus.
func (a *Adapter) SetRouter(r *si.Router) {
	a.router = r
}

func (a *Adapter) Name() string { return "websocket" }

// Start runs the WebSocket server.
func (a *Adapter) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", a.handleWS)
	mux.HandleFunc("/api/history", a.handleHistory)
	mux.HandleFunc("/api/status", a.handleStatus)

	srv := &http.Server{Addr: a.addr, Handler: mux}

	// Subscribe to event bus and broadcast to all WS clients
	if a.router != nil {
		events := a.router.Subscribe()
		go func() {
			for {
				select {
				case <-ctx.Done():
					a.router.Unsubscribe(events)
					return
				case e, ok := <-events:
					if !ok {
						return
					}
					a.broadcastEvent(e)
				}
			}
		}()
	}

	go func() {
		<-ctx.Done()
		srv.Close()
		close(a.incoming)
	}()

	log.Printf("[websocket] listening on %s", a.addr)
	return srv.ListenAndServe()
}

// broadcastEvent sends an event to all connected WebSocket clients.
func (a *Adapter) broadcastEvent(e si.Event) {
	envelope := map[string]interface{}{
		"type":       "event",
		"event_type": string(e.Type),
		"message":    e.Message,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	for conn := range a.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[websocket] broadcast error: %v", err)
		}
	}
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

// handleHistory serves GET /api/history?limit=50&agent=claxon
func (a *Adapter) handleHistory(w http.ResponseWriter, r *http.Request) {
	if a.router == nil || a.router.History == nil {
		http.Error(w, "history not available", http.StatusServiceUnavailable)
		return
	}

	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	agent := r.URL.Query().Get("agent")

	events := a.router.History.Recent(limit)

	// Filter by agent if specified
	if agent != "" {
		filtered := make([]si.Event, 0, len(events))
		for _, e := range events {
			if e.Message.Agent == agent || e.Message.Author == agent {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(events)
}

// handleStatus serves GET /api/status
func (a *Adapter) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"adapters": []string{},
		"feed":     "connected",
	}

	if a.router != nil {
		adapters := a.router.Adapters()
		names := make([]string, len(adapters))
		for i, ad := range adapters {
			names[i] = ad.Name()
		}
		status["adapters"] = names
	}

	a.mu.RLock()
	status["ws_clients"] = len(a.clients)
	a.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(status)
}

// Send is called by the router for direct message routing.
// Since the event bus broadcast already handles delivery to WS clients,
// this is a no-op to avoid duplicate messages.
func (a *Adapter) Send(msg si.Message) error {
	// Event bus handles all delivery now — no-op to prevent duplicates.
	return nil
}

func (a *Adapter) sendLegacy_unused(msg si.Message) error {
	envelope := map[string]interface{}{
		"type":    "response",
		"message": msg,
	}
	data, err := json.Marshal(envelope)
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
