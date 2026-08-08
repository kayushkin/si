// Package logtext holds the string shortening si does before it writes a
// message body into a log line.
//
// It exists as one package because the same function was written three times —
// router.go's truncate, feed/bus.go's truncateBus and feed/nats.go's
// truncateNats were byte-identical under three names, and all three carried the
// same defect. One copy is one place to fix.
package logtext

import "unicode/utf8"

// TruncateAtRuneBoundaryWithEllipsis returns s shortened to at most maxBytes
// bytes with "..." appended, cutting only between runes. A string already
// within budget is returned unchanged and gains no ellipsis.
//
// The result is therefore up to maxBytes+3 bytes long: the ellipsis sits
// outside the budget, which is what all three copies of this function did
// before they were merged here. That arithmetic is deliberately unchanged —
// whether an ellipsis belongs inside the byte budget is an open question for
// the fleet, and settling it here would hide the rune fix inside a second
// behaviour change.
//
// The defect this fixes: cutting a Go string at a fixed byte offset splits
// whatever rune straddles that offset, and the result is not valid UTF-8.
// Nothing reports it. si carries chat text from Discord, Telegram, Slack and
// Matrix, so a multi-byte rune across the cut is routine rather than exotic,
// and the reader of the log sees a replacement character with no error raised
// anywhere along the path.
//
// The walk-back costs at most three byte comparisons and allocates nothing,
// which is why it is preferred here over converting to []rune.
func TruncateAtRuneBoundaryWithEllipsis(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// s[cut] is the first byte past the prefix. While it is a continuation
	// byte, a rune straddles the cut, so move the cut earlier.
	cut := maxBytes
	if cut < 0 {
		// A budget below zero keeps nothing, the same as a budget of zero. The
		// byte cut this replaced panicked here instead, and no scan for the
		// rune defect can see that: in source, a panic and a split rune are the
		// same expression. No caller in si passes a negative budget — all six
		// pass a positive literal — so this is a guard, not a behaviour change.
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}
