package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/terminal"
)

func TestToastMarkdownListLayout(t *testing.T) {
	for _, profile := range []terminal.ColorProfile{terminal.ColorNone, terminal.ColorTrue} {
		f := NewFormatter(createTestIOContext(), createMockTerminal(profile)).(*formatter)
		for _, tt := range []struct{ input, want string }{
			{"1. numbered with dot", "▶ 1. numbered with dot"},
			{"  1. indented numbered", "▶ 1. indented numbered"},
			{"2) paren style", "▶ 2. paren style"},
			{"- dash bullet", "▶ • dash bullet"},
			{"* star bullet", "▶ • star bullet"},
			{"1. **Create** a `token`", "▶ 1. Create a token"},
			{"2. First\n3. Second", "▶ 2. First\n  3. Second"},
			{"- First\n- Second", "▶ • First\n  • Second"},
			{`1\. literal`, "▶ 1. literal"},
			{`\* literal`, "▶ * literal"},
		} {
			t.Run(strconv.Itoa(int(profile))+tt.input, func(t *testing.T) {
				rendered := f.Info(tt.input)
				assert.Equal(t, tt.want, ansi.Strip(rendered))
				if profile == terminal.ColorNone {
					assert.NotContains(t, rendered, "\x1b")
				} else {
					assert.Contains(t, rendered, "\x1b[", "themed Markdown must remain styled")
				}
			})
		}
	}
}

func TestToastMarkdownNestedWrappedLists(t *testing.T) {
	for _, profile := range []terminal.ColorProfile{terminal.ColorNone, terminal.ColorTrue} {
		t.Run(strconv.Itoa(int(profile)), func(t *testing.T) {
			ioCtx := createTestIOContext()
			ioCtx.Config().AtmosConfig.Settings.Terminal.MaxWidth = 32
			f := NewFormatter(ioCtx, createMockTerminal(profile)).(*formatter)
			rendered := ansi.Strip(f.Info("- **outer**\n  - inner item has enough words to wrap onto another line\n- sibling"))
			var lines []string
			for _, line := range strings.Split(rendered, "\n") {
				if strings.TrimSpace(line) != "" {
					lines = append(lines, line)
				}
			}
			require.GreaterOrEqual(t, len(lines), 4, "nested item should wrap: %q", rendered)
			assert.Equal(t, "▶ • outer", lines[0])
			assert.True(t, strings.HasPrefix(strings.TrimSpace(lines[1]), "• inner item"), "%q", rendered)
			assert.Equal(t, "  • sibling", lines[len(lines)-1])
			indent := func(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }
			assert.Greater(t, indent(lines[1]), 2, "nested list must remain indented")
			assert.Equal(t, indent(lines[1])+2, indent(lines[2]), "wrapped text must align after the nested bullet")
		})
	}
}
