package feed

import (
	"context"
	"os"
	"testing"
	"time"

	si "github.com/kayushkin/si"
)

// TestInberDirect_BasicFlow tests the basic message flow
func TestInberDirect_BasicFlow(t *testing.T) {
	cfg := InberDirectConfig{
		Agent: "task-manager",
	}

	feed := NewInberDirect(cfg)
	defer feed.Close()

	// Start the feed
	if err := feed.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify channels are ready
	if feed.inbound == nil {
		t.Error("inbound channel is nil")
	}
	if feed.outbound == nil {
		t.Error("outbound channel is nil")
	}

	// Verify fallback model is set
	if feed.fallbackModel != "glm-5" {
		t.Errorf("expected fallback model 'glm-5', got %q", feed.fallbackModel)
	}

	t.Log("✓ Feed initialized correctly with fallback")
}

// TestInberDirect_SessionTracking tests per-channel session tracking
func TestInberDirect_SessionTracking(t *testing.T) {
	feed := NewInberDirect(InberDirectConfig{Agent: "worker"})
	defer feed.Close()

	tests := []struct {
		channel  string
		expected string
	}{
		{"discord:123456", "si-discord-123456-worker"},
		{"tui", "si-tui-worker"},
		{"websocket", "si-websocket-worker"},
		{"openclaw", "si-openclaw-worker"},
	}

	for _, tc := range tests {
		sessionID := feed.getOrCreateSession(tc.channel, "worker")
		if sessionID != tc.expected {
			t.Errorf("channel %q: expected session %q, got %q", tc.channel, tc.expected, sessionID)
		}

		// Second call should return same session
		sessionID2 := feed.getOrCreateSession(tc.channel, "worker")
		if sessionID2 != sessionID {
			t.Errorf("channel %q: session not consistent", tc.channel)
		}
	}

	t.Log("✓ Session tracking works correctly")
}

// TestInberDirect_WriteBufferFull tests buffer overflow handling
func TestInberDirect_WriteBufferFull(t *testing.T) {
	feed := NewInberDirect(InberDirectConfig{Agent: "worker"})
	defer feed.Close()

	// Fill the buffer
	for i := 0; i < 64; i++ {
		if err := feed.Write(si.Message{Text: "test"}); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	// Next write should fail (buffer full)
	err := feed.Write(si.Message{Text: "overflow"})
	if err == nil {
		t.Error("expected error when buffer full")
	}

	t.Logf("✓ Buffer overflow handled: %v", err)
}

// TestInberDirect_Close tests clean shutdown
func TestInberDirect_Close(t *testing.T) {
	feed := NewInberDirect(InberDirectConfig{Agent: "worker"})

	if err := feed.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Close should not block or error
	done := make(chan bool)
	go func() {
		feed.Close()
		done <- true
	}()

	select {
	case <-done:
		t.Log("✓ Close completed cleanly")
	case <-time.After(2 * time.Second):
		t.Error("Close blocked for too long")
	}
}

// TestInberDirect_WriteAfterClose tests writing to closed feed
func TestInberDirect_WriteAfterClose(t *testing.T) {
	feed := NewInberDirect(InberDirectConfig{Agent: "worker"})
	feed.Start()
	feed.Close()

	// Give it a moment to close
	time.Sleep(100 * time.Millisecond)

	err := feed.Write(si.Message{Text: "test"})
	if err == nil {
		t.Error("expected error when writing to closed feed")
	}

	t.Logf("✓ Write after close returns error: %v", err)
}

// TestInberDirect_MultipleChannels tests handling multiple concurrent channels
func TestInberDirect_MultipleChannels(t *testing.T) {
	feed := NewInberDirect(InberDirectConfig{Agent: "worker"})
	defer feed.Close()
	feed.Start()

	channels := []string{"discord:1", "discord:2", "tui", "websocket"}

	for _, ch := range channels {
		sessionID := feed.getOrCreateSession(ch, "worker")
		if sessionID == "" {
			t.Errorf("channel %q: empty session ID", ch)
		}
	}

	// Verify all sessions are tracked
	feed.mu.RLock()
	if len(feed.sessions) != len(channels) {
		t.Errorf("expected %d sessions, got %d", len(channels), len(feed.sessions))
	}
	feed.mu.RUnlock()

	t.Log("✓ Multiple channels handled correctly")
}

// TestInberDirect_EmptyChannel tests handling messages without channel
func TestInberDirect_EmptyChannel(t *testing.T) {
	feed := NewInberDirect(InberDirectConfig{Agent: "worker"})
	defer feed.Close()

	// Empty channel should return empty session
	sessionID := feed.getOrCreateSession("", "worker")
	if sessionID != "" {
		t.Errorf("expected empty session for empty channel, got %q", sessionID)
	}

	t.Log("✓ Empty channel handled correctly")
}

// TestInberDirect_CustomFallback tests custom fallback model config
func TestInberDirect_CustomFallback(t *testing.T) {
	feed := NewInberDirect(InberDirectConfig{
		Agent:         "worker",
		FallbackModel: "custom-model",
	})

	if feed.fallbackModel != "custom-model" {
		t.Errorf("expected fallback 'custom-model', got %q", feed.fallbackModel)
	}

	feed.Close()
	t.Log("✓ Custom fallback model configured")
}

// TestNeedsFallback tests the fallback detection logic
func TestNeedsFallback(t *testing.T) {
	tests := []struct {
		name     string
		response string
		err      error
		want     bool
	}{
		{"no error", "Hello world", nil, false},
		{"529 error", "", context.DeadlineExceeded, true},
		{"response with 529", "API error: 529 overloaded", nil, true},
		{"response with 503", "Service unavailable: 503", nil, true},
		{"response with rate limit", "Rate limit exceeded", nil, true},
		{"normal response", "I can help you with that", nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := needsFallback(tc.response, tc.err)
			if got != tc.want {
				t.Errorf("needsFallback(%q, %v) = %v, want %v", tc.response, tc.err, got, tc.want)
			}
		})
	}

	t.Log("✓ Fallback detection logic works")
}

// TestInberDirect_Integration tests a real message through the feed
// This test is skipped by default - set SI_INTEGRATION=1 to run
func TestInberDirect_Integration(t *testing.T) {
	if os.Getenv("SI_INTEGRATION") != "1" {
		t.Skip("Skipping integration test - set SI_INTEGRATION=1 to run")
	}

	feed := NewInberDirect(InberDirectConfig{
		Agent: "worker", // Use worker (glm-5) for faster response
	})
	defer feed.Close()

	if err := feed.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send a test message
	msg := si.Message{
		Text:    "Say 'integration test passed' and nothing else",
		Channel: "test",
		Author:  "tester",
	}

	if err := feed.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Wait for response with timeout
	select {
	case resp := <-feed.Read():
		if resp.Text == "" {
			t.Error("empty response")
		}
		t.Logf("✓ Integration test passed: %s", truncate(resp.Text, 100))
	case <-time.After(60 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}
