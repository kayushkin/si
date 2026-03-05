package feed

import (
	"testing"
)

func TestParseStderrMeta(t *testing.T) {
	stderr := `
┌─ Tokens ──────────────────────
│ in=1234  out=567  total=1801  tools=3
│ cache: 800 read, 200 created
│ cost=$0.0142
└───────────────────────────────
`
	meta := parseStderrMeta(stderr)
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if meta.InputTokens != 1234 {
		t.Errorf("InputTokens: got %d, want 1234", meta.InputTokens)
	}
	if meta.OutputTokens != 567 {
		t.Errorf("OutputTokens: got %d, want 567", meta.OutputTokens)
	}
	if meta.ToolCalls != 3 {
		t.Errorf("ToolCalls: got %d, want 3", meta.ToolCalls)
	}
	if meta.CacheReadTokens != 800 {
		t.Errorf("CacheReadTokens: got %d, want 800", meta.CacheReadTokens)
	}
	if meta.CacheCreationTokens != 200 {
		t.Errorf("CacheCreationTokens: got %d, want 200", meta.CacheCreationTokens)
	}
	if meta.Cost < 0.014 || meta.Cost > 0.015 {
		t.Errorf("Cost: got %f, want ~0.0142", meta.Cost)
	}
}

func TestParseStderrMeta_NoCache(t *testing.T) {
	stderr := `
┌─ Tokens ──────────────────────
│ in=500  out=100  total=600  tools=0
│ cost=$0.0010
└───────────────────────────────
`
	meta := parseStderrMeta(stderr)
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if meta.InputTokens != 500 {
		t.Errorf("InputTokens: got %d, want 500", meta.InputTokens)
	}
	if meta.CacheReadTokens != 0 {
		t.Errorf("CacheReadTokens: got %d, want 0", meta.CacheReadTokens)
	}
}

func TestParseStderrMeta_Empty(t *testing.T) {
	meta := parseStderrMeta("")
	if meta != nil {
		t.Error("expected nil for empty stderr")
	}
}

func TestParseStderrMeta_NoStats(t *testing.T) {
	meta := parseStderrMeta("some random error output")
	if meta != nil {
		t.Error("expected nil for non-stats stderr")
	}
}
