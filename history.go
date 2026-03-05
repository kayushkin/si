package si

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// History stores events in a ring buffer and persists to a JSONL file.
type History struct {
	mu       sync.RWMutex
	messages []Event
	maxSize  int
	head     int // next write position
	count    int
	file     *os.File
}

// NewHistory creates a history store. It loads existing events from path
// and appends new ones. maxSize is the ring buffer capacity.
func NewHistory(path string, maxSize int) *History {
	h := &History{
		messages: make([]Event, maxSize),
		maxSize:  maxSize,
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("[history] mkdir error: %v", err)
		return h
	}

	// Load existing events
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var e Event
			if err := json.Unmarshal(scanner.Bytes(), &e); err == nil {
				h.messages[h.head] = e
				h.head = (h.head + 1) % maxSize
				if h.count < maxSize {
					h.count++
				}
			}
		}
		f.Close()
		log.Printf("[history] loaded %d events from %s", h.count, path)
	}

	// Open for appending
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[history] open error: %v", err)
	} else {
		h.file = f
	}

	return h
}

// Add stores an event in the ring buffer and appends to the JSONL file.
func (h *History) Add(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.messages[h.head] = e
	h.head = (h.head + 1) % h.maxSize
	if h.count < h.maxSize {
		h.count++
	}

	if h.file != nil {
		if data, err := json.Marshal(e); err == nil {
			h.file.Write(data)
			h.file.Write([]byte("\n"))
		}
	}
}

// Recent returns the last n events (or fewer if not enough stored).
func (h *History) Recent(n int) []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if n > h.count {
		n = h.count
	}
	if n == 0 {
		return nil
	}

	result := make([]Event, n)
	start := (h.head - n + h.maxSize) % h.maxSize
	for i := 0; i < n; i++ {
		result[i] = h.messages[(start+i)%h.maxSize]
	}
	return result
}

// Since returns all events with timestamp >= t.
func (h *History) Since(t time.Time) []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []Event
	start := (h.head - h.count + h.maxSize) % h.maxSize
	for i := 0; i < h.count; i++ {
		e := h.messages[(start+i)%h.maxSize]
		if !e.Message.Timestamp.Before(t) {
			result = append(result, e)
		}
	}
	return result
}

// Close closes the history file.
func (h *History) Close() error {
	if h.file != nil {
		return h.file.Close()
	}
	return nil
}
