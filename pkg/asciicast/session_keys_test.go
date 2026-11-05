package asciicast

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeySequenceExtendedKeys(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "delete", want: "\x1b[3~"},
		{input: "Delete", want: "\x1b[3~"},
		{input: "insert", want: "\x1b[2~"},
		{input: "home", want: "\x1b[H"},
		{input: "end", want: "\x1b[F"},
		{input: "shift+tab", want: "\x1b[Z"},
		{input: "Shift+Tab", want: "\x1b[Z"},
		{input: "shift+a", want: "A"},
		{input: "shift+z", want: "Z"},
		{input: "alt+b", want: "\x1bb"},
		{input: "alt+B", want: "\x1bB"},
		{input: "alt+.", want: "\x1b."},
		{input: "alt+é", want: "\x1bé"},
		{input: "ctrl+alt+c", want: "\x1b\x03"},
		{input: "Ctrl+Alt+A", want: "\x1b\x01"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := keySequence(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestKeySequenceRejectsMalformedModifierKeys(t *testing.T) {
	for _, input := range []string{"shift+1", "shift+enter", "shift+ab", "alt+", "alt+ab", "ctrl+alt+1", "ctrl+alt+", "ctrl+alt+ab", "ctrl+shift+a", "meta+a"} {
		t.Run(input, func(t *testing.T) {
			_, err := keySequence(input)
			require.ErrorIs(t, err, ErrUnsupportedCastKey)
		})
	}
}
