package step

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestViewportLinesPreserveStylesWithoutTerminalControls(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		color      bool
		want       []string
	}{
		{
			name: "color carries across rows",
			text: "\x1b[31mfirst\nsecond\x1b[0m plain\n", color: true,
			want: []string{"\x1b[31mfirst" + ansi.ResetStyle, "\x1b[31msecond" + ansi.ResetStyle + " plain"},
		},
		{
			name: "color disabled",
			text: "\x1b[31mfirst\nsecond\x1b[0m plain\n",
			want: []string{"first", "second plain"},
		},
		{
			name: "cursor and clipboard controls removed",
			text: "\x1b[2J\x1b[H\x1b]52;c;secret\a\x1b[32mready\x1b[0m\x1b[?25l", color: true,
			want: []string{"\x1b[32mready" + ansi.ResetStyle},
		},
		{
			name: "carriage return replaces row without losing style",
			text: "\x1b[31mold\rnew\r\nnext\tcolumn", color: true,
			want: []string{"\x1b[31mnew" + ansi.ResetStyle, "\x1b[31mnext    column" + ansi.ResetStyle},
		},
		{
			name: "incomplete escape does not leak",
			text: "ready\x1b[31", color: true,
			want: []string{"ready"},
		},
		{
			name: "blank rows preserved",
			text: "first\n\nlast\n", color: true,
			want: []string{"first", "", "last"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, viewportLines(tc.text, tc.color))
		})
	}
}

func TestOutputViewportPreservesColorThroughScrollingAndClipping(t *testing.T) {
	tail := &viewportTail{}
	_, _ = io.WriteString(tail, "\x1b[31mhidden\nvisible\n"+strings.Repeat("界", 20)+"\n")
	m := &outputViewportModel{
		tail: tail, title: "Build", width: 12, height: 24, maxHeight: 3,
		spinner: spinner.New(), color: true,
	}
	view := m.View()
	assert.NotContains(t, view, "hidden")
	lines := strings.Split(view, "\n")
	assert.Len(t, lines, 3)
	for _, line := range lines[:2] {
		assert.Contains(t, line, "\x1b[31m")
		assert.Contains(t, line, ansi.ResetStyle)
		assert.LessOrEqual(t, ansi.StringWidth(line), 11)
	}
	assert.NotContains(t, lines[2], "\x1b[31m", "child style must not leak into status")
}
