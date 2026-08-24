package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// This file covers direct unit tests of style.go's line-ending helpers
// (detectLineEnding, ensureFinalNewline, removeFinalNewline). These are
// distinct from editorconfig_test.go, which exercises line-ending/indent
// behavior indirectly through the public SetFile/FormatFile file API.

func TestDetectLineEnding(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"pure LF", "a: 1\nb: 2\n", lineEndingLF},
		{"pure CRLF", "a: 1\r\nb: 2\r\n", lineEndingCRLF},
		{"no line endings at all", "a: 1", ""},
		{"mixed LF and CRLF", "a: 1\r\nb: 2\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, detectLineEnding([]byte(tt.in)))
		})
	}
}

func TestEnsureFinalNewline(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already LF-terminated", "a: 1\n", "a: 1\n"},
		{"already bare-CR-terminated", "a: 1\r", "a: 1\r"},
		{"no trailing newline", "a: 1", "a: 1\n"},
		{"empty input", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(ensureFinalNewline([]byte(tt.in))))
		})
	}
}

func TestRemoveFinalNewline(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"CRLF-terminated strips both bytes", "a: 1\r\n", "a: 1"},
		{"bare-LF-terminated strips one byte", "a: 1\n", "a: 1"},
		{"bare-CR-terminated strips one byte", "a: 1\r", "a: 1"},
		{"no trailing newline is a no-op", "a: 1", "a: 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(removeFinalNewline([]byte(tt.in))))
		})
	}
}
