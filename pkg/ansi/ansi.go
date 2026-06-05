// Package ansi provides utilities for working with ANSI escape sequences in strings.
// It offers functions to strip ANSI codes, calculate visible string length,
// and perform ANSI-aware trimming operations.
package ansi

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// newline is the line separator used for multi-line operations.
const newline = "\n"

// Strip removes all ANSI escape codes from a string, returning only visible content.
// This uses a state machine approach for robust handling of all ANSI sequences.
func Strip(s string) string {
	defer perf.Track(nil, "ansi.Strip")()

	var result strings.Builder
	result.Grow(len(s))

	for i := 0; i < len(s); {
		if isStart(s, i) {
			i = skip(s, i)
		} else {
			result.WriteByte(s[i])
			i++
		}
	}

	return result.String()
}

// Length returns the visible character count of a string, excluding ANSI codes.
func Length(s string) int {
	defer perf.Track(nil, "ansi.Length")()

	count := 0
	for i := 0; i < len(s); {
		if isStart(s, i) {
			i = skip(s, i)
		} else {
			count++
			i++
		}
	}
	return count
}

// isStart checks if position i marks the start of an ANSI escape sequence.
func isStart(s string, i int) bool {
	return s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '['
}

// skip advances past an ANSI escape sequence starting at position i.
// Returns the index after the sequence terminator.
func skip(s string, i int) int {
	i += 2 // Skip ESC and [.
	for i < len(s) && !isTerminator(s[i]) {
		i++
	}
	if i < len(s) {
		i++ // Skip terminator.
	}
	return i
}

// isTerminator checks if byte b is an ANSI sequence terminator (A-Z or a-z).
func isTerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// findLastEnd returns the position after the last ANSI escape sequence in s.
// Returns -1 if no ANSI sequences are found.
func findLastEnd(s string) int {
	lastEnd := -1
	for i := 0; i < len(s); {
		if isStart(s, i) {
			i = skip(s, i)
			lastEnd = i
		} else {
			i++
		}
	}
	return lastEnd
}

// copyContentAndANSI copies characters and ANSI codes from s until plainIdx reaches targetLen.
// Returns the result builder pointer and the final position in s.
func copyContentAndANSI(s string, targetLen int) (*strings.Builder, int) {
	result := &strings.Builder{}
	plainIdx := 0
	i := 0

	for i < len(s) && plainIdx < targetLen {
		if isStart(s, i) {
			start := i
			i = skip(s, i)
			result.WriteString(s[start:i])
		} else {
			result.WriteByte(s[i])
			plainIdx++
			i++
		}
	}

	return result, i
}
