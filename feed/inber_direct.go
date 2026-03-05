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
// Falls back to glm-5 if anthropic is not responding.
type InberDirect struct {
	inberBin       string
	inberDir       string
	agent          string
	model          string
	fallbackModel  string // model to use if primary fails

	inbound  chan si.Message // from adapters → inber
	outbound chan si.Message // from inber → adapters

	// session tracking for context continuity
	sessions map[string]string // channel → session ID
	mu       sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	
	// logstack client for centralized logging
	logstackClient *LogstackClient
}

// InberDirectConfig configures the inber direct feed.
type InberDirectConfig struct {
	InberBin      string // path to inber binary (default: ~/bin/inber)
	InberDir      string // working directory (default: ~/life/repos/inber)
	Agent         string // agent to use (default: task-manager)
	Model         string // model override (default: uses agent's model)
	FallbackModel string // fallback model (default: glm-5)
}

// NewInberDirect creates a feed that calls inber directly.
func NewInberDirect(cfg InberDirectConfig) *InberDirect {
	if cfg.InberBin == "" {
		if bin := os.Getenv("SI_INBER_BIN"); bin != "" {
			cfg.InberBin = bin
		} else {
			cfg.InberBin = os.ExpandEnv("$HOME/bin/inber")
		}
	}
	if cfg.InberDir == "" {
		if dir := os.Getenv("SI_INBER_DIR"); dir != "" {
			cfg.InberDir = dir
		} else {
			cfg.InberDir = os.ExpandEnv("$HOME/life/repos/inber")
		}
	}
	if cfg.Agent == "" {
		cfg.Agent = "task-manager" // opus46 orchestrator
	}
	if cfg.FallbackModel == "" {
		cfg.FallbackModel = "glm-5" // fallback to GLM-5
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &InberDirect{
		inberBin:       cfg.InberBin,
		inberDir:       cfg.InberDir,
		agent:          cfg.Agent,
		model:          cfg.Model,
		fallbackModel:  cfg.FallbackModel,
		inbound:        make(chan si.Message, 64),
		outbound:       make(chan si.Message, 64),
		sessions:       make(map[string]string),
		ctx:            ctx,
		cancel:         cancel,
		logstackClient: NewLogstackClient(),
	}
}

// Start begins processing messages. Blocks until context is done.
func (f *InberDirect) Start() error {
	log.Printf("[feed/inber] starting with agent=%s bin=%s fallback=%s", f.agent, f.inberBin, f.fallbackModel)

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

// processMessage handles a single message through inber with fallback.
func (f *InberDirect) processMessage(msg si.Message) {
	start := time.Now()

	// Use message-specified agent or default
	agent := f.agent
	if msg.Agent != "" {
		agent = msg.Agent
	}

	// Log incoming message
	f.logstackClient.LogRouting(msg.Channel, agent, f.model)

	// Build context prefix with channel info
	contextPrefix := ""
	if msg.Channel != "" {
		contextPrefix = fmt.Sprintf("[from %s", msg.Channel)
		if msg.Author != "" {
			contextPrefix += fmt.Sprintf(" by %s", msg.Author)
		}
		contextPrefix += "] "
	}

	// Get or create session for this channel+agent
	sessionID := f.getOrCreateSession(msg.Channel, agent)
	fullInput := contextPrefix + msg.Text

	// Try primary model first
	responseText, err := f.runInber(fullInput, sessionID, agent, f.model)
	modelUsed := f.model

	if err != nil || needsFallback(responseText, err) {
		// Log the failure and try fallback
		if err != nil {
			log.Printf("[feed/inber] primary model failed: %v, trying fallback %s", err, f.fallbackModel)
			f.logstackClient.LogError(msg.Channel, f.model, err.Error())
		} else {
			log.Printf("[feed/inber] primary model gave error response, trying fallback %s", f.fallbackModel)
		}

		// Retry with fallback model
		fallbackText, fallbackErr := f.runInber(fullInput, sessionID, agent, f.fallbackModel)
		if fallbackErr != nil {
			log.Printf("[feed/inber] fallback also failed: %v", fallbackErr)
			f.logstackClient.LogError(msg.Channel, f.fallbackModel, fallbackErr.Error())
			f.sendError(msg, fmt.Sprintf("both models failed: %v (fallback: %v)", err, fallbackErr))
			return
		}
		responseText = fallbackText
		modelUsed = f.fallbackModel
	}

	elapsed := time.Since(start)

	if responseText == "" {
		log.Printf("[feed/inber] empty response in %v", elapsed)
		return
	}

	log.Printf("[feed/inber] response in %v: %s", elapsed, truncate(responseText, 80))

	// Log successful response
	f.logstackClient.LogMessage(msg.Channel, modelUsed, responseText, elapsed.Milliseconds())

	// Send response back through the feed
	f.outbound <- si.Message{
		Text:    responseText,
		Channel: msg.Channel, // route back to origin
		Author:  agent,
	}
}

// runInber executes inber with the given input and model.
func (f *InberDirect) runInber(input, sessionID, agent, model string) (string, error) {
	// Build command
	// Note: inber doesn't have --session flag, so we use --detach for isolated runs
	args := []string{"run", "--agent", agent, "--detach"}
	if model != "" {
		args = append(args, "--model", model)
	}
	// sessionID is tracked by si but not passed to inber (for future use)
	args = append(args, "--raw") // raw output mode

	ctx, cancel := context.WithTimeout(f.ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, f.inberBin, args...)
	cmd.Dir = f.inberDir

	// Pipe input
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}

	// Capture output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start: %w", err)
	}

	// Send the message with context
	io.WriteString(stdin, input)
	stdin.Close()

	// Read response
	var response strings.Builder
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip ANSI codes and status lines
		if strings.HasPrefix(line, "\x1b[") ||
			strings.HasPrefix(line, "resuming session") ||
			strings.HasPrefix(line, "model:") ||
			strings.HasPrefix(line, "logging to") {
			continue
		}
		response.WriteString(line)
		response.WriteString("\n")
	}

	// Check for errors
	errData, _ := io.ReadAll(stderr)
	waitErr := cmd.Wait()

	responseText := strings.TrimSpace(response.String())

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("timeout after 120s")
	}

	if waitErr != nil {
		// Check if we got a partial response anyway
		if responseText != "" {
			return responseText, nil
		}
		return "", fmt.Errorf("exit %d: %s", cmd.ProcessState.ExitCode(), string(errData))
	}

	return responseText, nil
}

// needsFallback checks if the response indicates an API error that warrants fallback.
func needsFallback(response string, err error) bool {
	if err != nil {
		// Check for specific error patterns that indicate API issues
		errStr := err.Error()
		if strings.Contains(errStr, "529") || // overloaded
			strings.Contains(errStr, "503") || // service unavailable
			strings.Contains(errStr, "429") || // rate limited
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "context deadline exceeded") {
			return true
		}
		return false
	}

	// Check response for error patterns
	responseLower := strings.ToLower(response)
	if strings.Contains(responseLower, "api error") ||
		strings.Contains(responseLower, "rate limit") ||
		strings.Contains(responseLower, "overloaded") ||
		strings.Contains(responseLower, "service unavailable") ||
		strings.Contains(responseLower, "529") ||
		strings.Contains(responseLower, "503") {
		return true
	}

	return false
}

// getOrCreateSession returns a session ID for continuity.
func (f *InberDirect) getOrCreateSession(channel, agent string) string {
	if channel == "" {
		return ""
	}
	// Key by channel+agent for per-agent sessions
	key := channel + ":" + agent
	f.mu.RLock()
	sessionID, ok := f.sessions[key]
	f.mu.RUnlock()

	if ok {
		return sessionID
	}

	// Use a deterministic session based on channel+agent
	sessionID = "si-" + strings.ReplaceAll(key, ":", "-")
	f.mu.Lock()
	f.sessions[key] = sessionID
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
func (f *InberDirect) Write(msg si.Message) (err error) {
	// Recover from panic if channel is closed
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("feed closed")
		}
	}()

	select {
	case f.inbound <- msg:
		return nil
	case <-f.ctx.Done():
		return fmt.Errorf("feed closed")
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
