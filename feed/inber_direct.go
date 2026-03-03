package feed

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	si "github.com/kayushkin/si"
)

// InberDirect calls inber CLI directly for each incoming message.
// Uses the task-manager (opus46) orchestrator for initial request processing.
type InberDirect struct {
	inberBin    string
	inberDir    string
	agent       string
	model       string

	inbound  chan si.Message // from adapters → inber
	outbound chan si.Message // from inber → adapters

	// session tracking for context continuity
	sessions map[string]string // origin → session ID
	mu       sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// InberDirectConfig configures the inber direct feed.
type InberDirectConfig struct {
	InberBin string // path to inber binary (default: ~/bin/inber)
	InberDir string // working directory (default: ~/life/repos/inber)
	Agent    string // agent to use (default: task-manager)
	Model    string // model override (default: uses agent's model)
}

// NewInberDirect creates a feed that calls inber directly.
func NewInberDirect(cfg InberDirectConfig) *InberDirect {
	if cfg.InberBin == "" {
		cfg.InberBin = os.ExpandEnv("$HOME/bin/inber")
	}
	if cfg.InberDir == "" {
		cfg.InberDir = os.ExpandEnv("$HOME/life/repos/inber")
	}
	if cfg.Agent == "" {
		cfg.Agent = "task-manager" // opus46 orchestrator
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &InberDirect{
		inberBin: cfg.InberBin,
		inberDir: cfg.InberDir,
		agent:    cfg.Agent,
		model:    cfg.Model,
		inbound:  make(chan si.Message, 64),
		outbound: make(chan si.Message, 64),
		sessions: make(map[string]string),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins processing messages. Blocks until context is done.
func (f *InberDirect) Start() error {
	log.Printf("[feed/inber] starting with agent=%s bin=%s", f.agent, f.inberBin)

	go func() {
		for {
			select {
			case <-f.ctx.Done():
				return
			case msg, ok := <-f.inbound:
				if !ok {
					return
				}
				go f.processMessage(msg)
			}
		}
	}()

	return nil
}

// processMessage handles a single message through inber.
func (f *InberDirect) processMessage(msg si.Message) {
	start := time.Now()

	// Build context prefix with channel info
	contextPrefix := ""
	if msg.Channel != "" {
		contextPrefix = fmt.Sprintf("[from %s", msg.Channel)
		if msg.Author != "" {
			contextPrefix += fmt.Sprintf(" by %s", msg.Author)
		}
		contextPrefix += "] "
	}

	// Get or create session for this channel
	sessionID := f.getOrCreateSession(msg.Channel)

	// Build command
	args := []string{"run", "--agent", f.agent}
	if f.model != "" {
		args = append(args, "--model", f.model)
	}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	args = append(args, "--raw") // raw output mode

	cmd := exec.CommandContext(f.ctx, f.inberBin, args...)
	cmd.Dir = f.inberDir

	// Pipe input
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("[feed/inber] stdin pipe error: %v", err)
		return
	}

	// Capture output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[feed/inber] stdout pipe error: %v", err)
		return
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Printf("[feed/inber] start error: %v", err)
		f.sendError(msg, fmt.Sprintf("failed to start inber: %v", err))
		return
	}

	// Send the message with context
	fullInput := contextPrefix + msg.Text
	io.WriteString(stdin, fullInput)
	stdin.Close()

	// Read response
	var response strings.Builder
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip ANSI codes and status lines
		if strings.HasPrefix(line, "\x1b[") || strings.HasPrefix(line, "resuming session") {
			continue
		}
		response.WriteString(line)
		response.WriteString("\n")
	}

	// Check for errors
	errData, _ := io.ReadAll(stderr)
	cmd.Wait()

	responseText := strings.TrimSpace(response.String())
	elapsed := time.Since(start)

	if cmd.ProcessState.ExitCode() != 0 {
		log.Printf("[feed/inber] exit %d: %s", cmd.ProcessState.ExitCode(), string(errData))
		if responseText == "" {
			f.sendError(msg, fmt.Sprintf("inber error: %s", string(errData)))
			return
		}
	}

	if responseText == "" {
		log.Printf("[feed/inber] empty response in %v", elapsed)
		return
	}

	log.Printf("[feed/inber] response in %v: %s", elapsed, truncate(responseText, 80))

	// Send response back through the feed
	f.outbound <- si.Message{
		Text:    responseText,
		Channel: msg.Channel, // route back to origin
		Author:  f.agent,
	}
}

// getOrCreateSession returns a session ID for continuity (currently stub).
// In the future, this could use inber's session resume feature.
func (f *InberDirect) getOrCreateSession(channel string) string {
	if channel == "" {
		return ""
	}
	f.mu.RLock()
	sessionID, ok := f.sessions[channel]
	f.mu.RUnlock()

	if ok {
		return sessionID
	}

	// For now, use a deterministic session based on channel
	// Inber will create/resume this session
	sessionID = "si-" + strings.ReplaceAll(channel, ":", "-")
	f.mu.Lock()
	f.sessions[channel] = sessionID
	f.mu.Unlock()

	return sessionID
}

// sendError sends an error message back.
func (f *InberDirect) sendError(original si.Message, errMsg string) {
	f.outbound <- si.Message{
		Text:    fmt.Sprintf("⚠️ %s", errMsg),
		Channel: original.Channel,
		Author:  "si",
	}
}

// Write sends a message to inber (from adapter).
func (f *InberDirect) Write(msg si.Message) error {
	select {
	case f.inbound <- msg:
		return nil
	default:
		return fmt.Errorf("inber feed buffer full")
	}
}

// Read returns responses from inber.
func (f *InberDirect) Read() <-chan si.Message {
	return f.outbound
}

// Close shuts down the feed.
func (f *InberDirect) Close() error {
	f.cancel()
	close(f.inbound)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
