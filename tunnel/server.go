package tunnel

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server runs on Linode, accepts connections from WSL tunnel clients.
type Server struct {
	addr      string
	authToken string
	clients   map[string]*clientConn
	feeds     map[*websocket.Conn]bool
	incoming  chan si.Message
	mu        sync.RWMutex
}

type clientConn struct {
	conn      *websocket.Conn
	connected time.Time
}

// NewServer creates a tunnel server.
func NewServer(addr, authToken string) *Server {
	return &Server{
		addr:      addr,
		authToken: authToken,
		clients:   make(map[string]*clientConn),
		feeds:     make(map[*websocket.Conn]bool),
		incoming:  make(chan si.Message, 64),
	}
}

// Start runs the tunnel server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", s.handleTunnel)   // WSL clients connect here
	mux.HandleFunc("/feed", s.handleFeed)       // si feed connects here

	srv := &http.Server{Addr: s.addr, Handler: mux}

	go func() {
		<-ctx.Done()
		srv.Close()
		close(s.incoming)
	}()

	// Forward WSL responses to all feed connections
	go s.broadcastIncoming()

	log.Printf("[tunnel-server] listening on %s", s.addr)
	return srv.ListenAndServe()
}

// broadcastIncoming forwards messages from WSL clients to all feed connections.
func (s *Server) broadcastIncoming() {
	for msg := range s.incoming {
		// Skip heartbeat acks
		if msg.Text == "__heartbeat_ack__" {
			continue
		}

		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[tunnel-server] marshal error: %v", err)
			continue
		}

		s.mu.RLock()
		for conn := range s.feeds {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("[tunnel-server] write to feed failed: %v", err)
			}
		}
		s.mu.RUnlock()
	}
}

// handleTunnel handles connections from WSL tunnel clients.
func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request) {
	// Auth check
	token := r.URL.Query().Get("token")
	if token != s.authToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[tunnel-server] upgrade error: %v", err)
		return
	}

	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = "default"
	}

	s.mu.Lock()
	s.clients[clientID] = &clientConn{conn: conn, connected: time.Now()}
	s.mu.Unlock()

	log.Printf("[tunnel-server] client '%s' connected from %s", clientID, conn.RemoteAddr())

	defer func() {
		s.mu.Lock()
		delete(s.clients, clientID)
		s.mu.Unlock()
		conn.Close()
		log.Printf("[tunnel-server] client '%s' disconnected", clientID)
	}()

	// Read responses from tunnel client (these came from inber on WSL)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[tunnel-server] client read error: %v", err)
			return
		}

		var msg si.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[tunnel-server] unmarshal error: %v", err)
			continue
		}

		// Handle heartbeat - echo back as ack
		if msg.Text == "__heartbeat__" {
			ack := si.Message{Text: "__heartbeat_ack__"}
			ackData, _ := json.Marshal(ack)
			conn.WriteMessage(websocket.TextMessage, ackData)
			continue
		}

		log.Printf("[tunnel-server] received from WSL: %s", truncateStr(msg.Text, 50))
		s.incoming <- msg
	}
}

// handleFeed handles connections from si (feed adapter).
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	// Auth check
	token := r.URL.Query().Get("token")
	if token != s.authToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[tunnel-server] feed upgrade error: %v", err)
		return
	}

	s.mu.Lock()
	s.feeds[conn] = true
	s.mu.Unlock()

	log.Printf("[tunnel-server] feed connected from %s", conn.RemoteAddr())

	defer func() {
		s.mu.Lock()
		delete(s.feeds, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	// Read messages from si feed, forward to WSL clients
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[tunnel-server] feed read error: %v", err)
			return
		}

		var msg si.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[tunnel-server] feed unmarshal error: %v", err)
			continue
		}

		log.Printf("[tunnel-server] from si → WSL: %s", truncateStr(msg.Text, 50))
		s.forwardToClients(msg)
	}
}

// forwardToClients sends a message to all connected tunnel clients.
func (s *Server) forwardToClients(msg si.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[tunnel-server] marshal error: %v", err)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for id, client := range s.clients {
		if err := client.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[tunnel-server] write to client '%s' failed: %v", id, err)
		}
	}
}

// Incoming returns messages from WSL clients (to be read by si feed).
func (s *Server) Incoming() <-chan si.Message {
	return s.incoming
}

// Broadcast sends a message to all si feeds.
func (s *Server) Broadcast(msg si.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.feeds {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

func truncateStr(str string, n int) string {
	if len(str) <= n {
		return str
	}
	return str[:n] + "..."
}
