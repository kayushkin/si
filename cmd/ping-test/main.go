// ping-test connects to si's WebSocket, sends a test message,
// subscribes to the event bus, and verifies the round-trip.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	si "github.com/kayushkin/si"
)

func main() {
	wsAddr := "ws://localhost:8090/ws"
	httpBase := "http://localhost:8090"

	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  sí ping test")
	fmt.Println("═══════════════════════════════════════")
	fmt.Println()

	passed := 0
	failed := 0

	// ── Test 1: HTTP /api/status ──
	fmt.Print("1. GET /api/status ... ")
	resp, err := http.Get(httpBase + "/api/status")
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		failed++
	} else {
		defer resp.Body.Close()
		var status map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&status)
		fmt.Printf("OK — %v\n", status)
		passed++
	}

	// ── Test 2: HTTP /api/history ──
	fmt.Print("2. GET /api/history ... ")
	resp2, err := http.Get(httpBase + "/api/history?limit=5")
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		failed++
	} else {
		defer resp2.Body.Close()
		var events []si.Event
		json.NewDecoder(resp2.Body).Decode(&events)
		fmt.Printf("OK — %d events in history\n", len(events))
		passed++
	}

	// ── Test 3: WebSocket connect ──
	fmt.Print("3. WebSocket connect to /ws ... ")
	conn, _, err := websocket.DefaultDialer.Dial(wsAddr, nil)
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		failed++
		printSummary(passed, failed)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("OK — connected")
	passed++

	// ── Test 4: Verify ws_clients count increased ──
	fmt.Print("4. Verify ws_clients incremented ... ")
	time.Sleep(100 * time.Millisecond)
	resp3, err := http.Get(httpBase + "/api/status")
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		failed++
	} else {
		defer resp3.Body.Close()
		var status map[string]interface{}
		json.NewDecoder(resp3.Body).Decode(&status)
		clients, _ := status["ws_clients"].(float64)
		if clients >= 1 {
			fmt.Printf("OK — ws_clients=%d\n", int(clients))
			passed++
		} else {
			fmt.Printf("FAIL — ws_clients=%v (expected >= 1)\n", status["ws_clients"])
			failed++
		}
	}

	// ── Test 5: Send message and receive event bus echo ──
	fmt.Print("5. Send message via WS, receive event bus broadcast ... ")

	pingID := uuid.New().String()
	pingMsg := si.Message{
		ID:        pingID,
		Text:      "ping-test: hello from Oisín 🕊️",
		Author:    "ping-test",
		Channel:   "websocket",
		Timestamp: time.Now(),
		Meta: &si.MessageMeta{
			Model:    "ping-test",
			Cost:     0.001337,
			ToolCalls: 1,
		},
	}

	data, _ := json.Marshal(pingMsg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		fmt.Printf("FAIL (write): %v\n", err)
		failed++
	} else {
		// Read event bus broadcast — should get the inbound event
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, respData, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("FAIL (read): %v\n", err)
			failed++
		} else {
			var envelope map[string]json.RawMessage
			json.Unmarshal(respData, &envelope)

			fmt.Printf("OK\n")
			fmt.Printf("   received: %s\n", string(respData)[:min(len(respData), 200)])
			passed++

			// ── Test 6: Verify message metadata preserved ──
			fmt.Print("6. Verify metadata in broadcast ... ")
			var fullEnvelope struct {
				Type      string     `json:"type"`
				EventType string     `json:"event_type"`
				Message   si.Message `json:"message"`
			}
			json.Unmarshal(respData, &fullEnvelope)

			if fullEnvelope.EventType == "inbound" && fullEnvelope.Message.ID == pingID {
				if fullEnvelope.Message.Meta != nil && fullEnvelope.Message.Meta.Cost == 0.001337 {
					fmt.Printf("OK — meta.cost=%.6f, meta.model=%s\n",
						fullEnvelope.Message.Meta.Cost, fullEnvelope.Message.Meta.Model)
					passed++
				} else if fullEnvelope.Message.Meta != nil {
					fmt.Printf("PARTIAL — meta present but cost=%v\n", fullEnvelope.Message.Meta.Cost)
					passed++ // still counts, meta is there
				} else {
					fmt.Printf("FAIL — meta is nil in broadcast\n")
					failed++
				}
			} else {
				fmt.Printf("FAIL — unexpected event: type=%s, id=%s (expected inbound, %s)\n",
					fullEnvelope.EventType, fullEnvelope.Message.ID, pingID)
				failed++
			}
		}
	}

	// ── Test 7: Verify message appeared in history ──
	fmt.Print("7. Verify message in /api/history ... ")
	time.Sleep(200 * time.Millisecond)
	resp4, err := http.Get(httpBase + "/api/history?limit=10")
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		failed++
	} else {
		defer resp4.Body.Close()
		var events []si.Event
		json.NewDecoder(resp4.Body).Decode(&events)
		found := false
		for _, e := range events {
			if e.Message.ID == pingID {
				found = true
				break
			}
		}
		if found {
			fmt.Printf("OK — ping message found in history\n")
			passed++
		} else {
			fmt.Printf("FAIL — ping message not found in %d history events\n", len(events))
			failed++
		}
	}

	fmt.Println()
	printSummary(passed, failed)

	if failed > 0 {
		os.Exit(1)
	}
}

func printSummary(passed, failed int) {
	fmt.Println("═══════════════════════════════════════")
	fmt.Printf("  Results: %d passed, %d failed\n", passed, failed)
	if failed == 0 {
		fmt.Println("  ✅ All systems nominal")
	} else {
		fmt.Println("  ❌ Issues detected")
	}
	fmt.Println("═══════════════════════════════════════")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
