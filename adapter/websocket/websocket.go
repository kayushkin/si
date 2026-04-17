package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// envToken returns the required bearer token for WS clients, or "" if auth is disabled.
// Set SI_WS_TOKEN to enable. If empty, the adapter is dev-open.
func envToken() string { return os.Getenv("SI_WS_TOKEN") }

// wsClient is one connected WebSocket peer.
type wsClient struct {
	id       string // client-supplied (X-Client-Id header or ?client_id=) or generated
	observer bool   // observer mode receives every outbound, not just targeted replies
	conn     *websocket.Conn
	writeMu  sync.Mutex
}

func (c *wsClient) write(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// Adapter runs a WebSocket server for direct clients (e.g. android-bridge phone, dashboards).
//
// Routing:
//   - Inbound: a client sends a message → we stamp ClientID and track inflight[msgID]=clientID
//     so the response can be routed back. Then forward to the feed.
//   - Outbound: router broadcasts an event → if the event's Message.ReplyTo matches a known
//     inflight message, only that originating client receives it. Observer clients always
//     receive every outbound event.
//
// Auth: if SI_WS_TOKEN is set, requires Authorization: Bearer <token> on the upgrade request.
type Adapter struct {
	addr     string
	incoming chan si.Message
	clients  map[*wsClient]struct{}
	byID     map[string]*wsClient // clientID → client (most recent connection for that id)
	inflight map[string]string    // message_id → client_id (pending reply)
	mu       sync.RWMutex
	router   *si.Router
}

// New creates a WebSocket adapter listening on addr (e.g. ":8090").
func New(addr string) *Adapter {
	return &Adapter{
		addr:     addr,
		incoming: make(chan si.Message, 64),
		clients:  make(map[*wsClient]struct{}),
		byID:     make(map[string]*wsClient),
		inflight: make(map[string]string),
	}
}

// SetRouter gives the adapter access to the router for the event bus.
func (a *Adapter) SetRouter(r *si.Router) { a.router = r }

func (a *Adapter) Name() string { return "websocket" }

// Start runs the WebSocket server.
func (a *Adapter) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", a.handleWS)
	mux.HandleFunc("/api/status", a.handleStatus)

	srv := &http.Server{Addr: a.addr, Handler: mux}

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
					a.routeEvent(e)
				}
			}
		}()
	}

	go func() {
		<-ctx.Done()
		srv.Close()
		close(a.incoming)
	}()

	if envToken() == "" {
		log.Printf("[websocket] listening on %s (AUTH DISABLED — set SI_WS_TOKEN to require bearer)", a.addr)
	} else {
		log.Printf("[websocket] listening on %s (bearer auth required)", a.addr)
	}
	return srv.ListenAndServe()
}

// routeEvent delivers an event to the correct client(s) based on ReplyTo.
func (a *Adapter) routeEvent(e si.Event) {
	envelope := map[string]interface{}{
		"type":       "event",
		"event_type": string(e.Type),
		"message":    e.Message,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return
	}

	// Find the target client for this reply, if any.
	target := ""
	if e.Message.ReplyTo != "" {
		a.mu.RLock()
		target = a.inflight[e.Message.ReplyTo]
		a.mu.RUnlock()
	}

	// When the stream closes, free the inflight entry.
	if target != "" && (e.Message.Stream == "done" || e.Message.Stream == "") {
		a.mu.Lock()
		delete(a.inflight, e.Message.ReplyTo)
		a.mu.Unlock()
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	for c := range a.clients {
		if c.observer {
			if err := c.write(data); err != nil {
				log.Printf("[websocket] observer write error: %v", err)
			}
			continue
		}
		if target != "" && c.id == target {
			if err := c.write(data); err != nil {
				log.Printf("[websocket] client write error: %v", err)
			}
			continue
		}
		// Legacy: untargeted events (no ReplyTo) still broadcast to everyone so
		// existing dashboards keep working until they opt into observer mode.
		if target == "" {
			if err := c.write(data); err != nil {
				log.Printf("[websocket] broadcast error: %v", err)
			}
		}
	}
}

func (a *Adapter) handleWS(w http.ResponseWriter, r *http.Request) {
	if !a.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[websocket] upgrade error: %v", err)
		return
	}

	clientID := strings.TrimSpace(r.Header.Get("X-Client-Id"))
	if clientID == "" {
		clientID = strings.TrimSpace(r.URL.Query().Get("client_id"))
	}
	if clientID == "" {
		clientID = uuid.New().String()
	}
	observer := r.URL.Query().Get("mode") == "observer"

	c := &wsClient{id: clientID, observer: observer, conn: conn}

	a.mu.Lock()
	a.clients[c] = struct{}{}
	a.byID[clientID] = c
	a.mu.Unlock()

	log.Printf("[websocket] client connected: id=%s observer=%v remote=%s", clientID, observer, conn.RemoteAddr())

	defer func() {
		a.mu.Lock()
		delete(a.clients, c)
		if a.byID[clientID] == c {
			delete(a.byID, clientID)
		}
		a.mu.Unlock()
		conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[websocket] read error (client=%s): %v", clientID, err)
			return
		}

		var msg si.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			msg = si.Message{Text: string(data)}
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
		msg.ClientID = clientID

		// Observer clients are read-only.
		if observer {
			continue
		}

		a.mu.Lock()
		a.inflight[msg.ID] = clientID
		a.mu.Unlock()

		a.incoming <- msg
	}
}

// authorize checks the bearer token if SI_WS_TOKEN is set.
func (a *Adapter) authorize(r *http.Request) bool {
	want := envToken()
	if want == "" {
		return true
	}
	// Accept Authorization: Bearer <token> or ?token=<token> (for browser-only dashboards).
	if got := r.Header.Get("Authorization"); strings.HasPrefix(got, "Bearer ") {
		return strings.TrimPrefix(got, "Bearer ") == want
	}
	return r.URL.Query().Get("token") == want
}

// handleStatus serves GET /api/status.
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
	status["inflight"] = len(a.inflight)
	a.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(status)
}

// Send is a no-op — event bus broadcast handles WS delivery.
func (a *Adapter) Send(msg si.Message) error { return nil }

// Receive returns the channel of inbound messages.
func (a *Adapter) Receive() <-chan si.Message { return a.incoming }
