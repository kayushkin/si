package logtext

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The defect these tests pin: cutting a Go string at a fixed byte offset splits
// whatever rune straddles that offset, and the result is not valid UTF-8.
// Nothing along the path reports it — the log line simply shows a replacement
// character.
//
// A single hand-picked input is not enough to catch it. Whether a cut splits a
// rune depends on where that rune happens to sit relative to the budget, so one
// string lands off the boundary and passes against the unfixed code. Every test
// here slides the cut across a range instead, and counts how much of that range
// actually exercised the case.

// musicalNote is four bytes (U+1D11E). A two-byte rune is a weaker probe: only
// one of its two interior offsets is wrong, so a test that picks the other one
// is green against the bug.
const musicalNote = "\U0001D11E"

// TestSlidesTheCutAcrossEveryOffset walks maxBytes across the whole length of an
// all-four-byte string. Three offsets in every four straddle a rune; the fourth
// is rune-aligned and is the known-negative control, which must come back
// identical to a plain byte cut against fixed and unfixed code alike. Without
// that control a green run cannot tell "detects the straddle" from "detects
// non-ASCII input".
func TestSlidesTheCutAcrossEveryOffset(t *testing.T) {
	s := strings.Repeat(musicalNote, 40) // 160 bytes, past every budget si uses
	straddled, aligned := 0, 0

	for maxBytes := 1; maxBytes < len(s); maxBytes++ {
		got := TruncateAtRuneBoundaryWithEllipsis(s, maxBytes)

		body := strings.TrimSuffix(got, "...")
		if body == got {
			t.Fatalf("maxBytes=%d: input is over budget, so the result must carry the ellipsis; got %q", maxBytes, got)
		}
		if !utf8.ValidString(body) {
			t.Fatalf("maxBytes=%d: result is not valid UTF-8: %q", maxBytes, body)
		}
		if len(body) > maxBytes {
			t.Fatalf("maxBytes=%d: kept %d bytes, over budget", maxBytes, len(body))
		}
		// Validity alone is not a falsifiable assertion — a helper that returns
		// "" for everything is always valid and always within budget. Pin
		// maximality too: whatever was dropped must genuinely not have fitted.
		_, width := utf8.DecodeRuneInString(s[len(body):])
		if len(body)+width <= maxBytes {
			t.Fatalf("maxBytes=%d: stopped at %d bytes but the next rune (%d bytes) still fits",
				maxBytes, len(body), width)
		}

		if maxBytes%4 == 0 {
			aligned++
			if body != s[:maxBytes] {
				t.Fatalf("maxBytes=%d is rune-aligned, so the walk-back must be a no-op; kept %d bytes",
					maxBytes, len(body))
			}
			continue
		}
		straddled++
		if body == s[:maxBytes] {
			t.Fatalf("maxBytes=%d straddles a rune, so the cut must move; it did not", maxBytes)
		}
	}

	// The reach-guard: if the fixture ever stops covering both cases, the loop
	// above proves nothing and would still pass.
	if straddled == 0 || aligned == 0 {
		t.Fatalf("fixture covered %d straddling and %d aligned offsets; both must be non-zero", straddled, aligned)
	}
}

// TestSlidesAMultiByteRuneAcrossOneFixedBudget is the caller's shape rather than
// the helper's: si truncates at fixed budgets of 50 and 80 bytes and the text is
// mostly ASCII with a rune somewhere in it. Sliding that rune across the budget
// is how the defect actually arrives.
func TestSlidesAMultiByteRuneAcrossOneFixedBudget(t *testing.T) {
	for _, maxBytes := range []int{50, 80} {
		straddled := 0
		for offset := maxBytes - 3; offset <= maxBytes; offset++ {
			s := strings.Repeat("a", offset) + musicalNote + strings.Repeat("b", 40)
			got := TruncateAtRuneBoundaryWithEllipsis(s, maxBytes)
			body := strings.TrimSuffix(got, "...")

			if !utf8.ValidString(body) {
				t.Fatalf("maxBytes=%d, rune at %d: result is not valid UTF-8: %q", maxBytes, offset, body)
			}
			if offset < maxBytes {
				straddled++
				if len(body) != offset {
					t.Fatalf("maxBytes=%d, rune at %d: expected the cut to back up to %d bytes, kept %d",
						maxBytes, offset, offset, len(body))
				}
				continue
			}
			// offset == maxBytes: the rune starts exactly at the budget, so
			// nothing straddles and the cut is untouched. The control.
			if len(body) != maxBytes {
				t.Fatalf("maxBytes=%d, rune at %d: nothing straddles the cut, so it must not move; kept %d bytes",
					maxBytes, offset, len(body))
			}
		}
		if straddled != 3 {
			t.Fatalf("maxBytes=%d: expected 3 straddling placements of a four-byte rune, exercised %d", maxBytes, straddled)
		}
	}
}

// TestLeavesTextWithinBudgetAlone pins the branch that must not gain an
// ellipsis.
func TestLeavesTextWithinBudgetAlone(t *testing.T) {
	s := strings.Repeat(musicalNote, 5) // 20 bytes
	for _, maxBytes := range []int{20, 21, 80} {
		if got := TruncateAtRuneBoundaryWithEllipsis(s, maxBytes); got != s {
			t.Fatalf("maxBytes=%d: text is within budget and must come back unchanged; got %q", maxBytes, got)
		}
	}
}

// TestBudgetOfZeroOrLessKeepsNoText holds the two edges apart. A budget of zero
// returned "..." from the byte cut this replaced and still does — that is the
// ellipsis arithmetic, left alone on purpose. A negative budget panicked, which
// is the same expression as the split rune and invisible to every scan for it;
// it now answers as zero does.
func TestBudgetOfZeroOrLessKeepsNoText(t *testing.T) {
	for _, maxBytes := range []int{0, -1} {
		if got := TruncateAtRuneBoundaryWithEllipsis(musicalNote, maxBytes); got != "..." {
			t.Fatalf("maxBytes=%d: no text fits, so only the ellipsis should come back; got %q", maxBytes, got)
		}
	}
}
