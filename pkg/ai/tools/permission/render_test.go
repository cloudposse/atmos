package permission

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ansi"
)

const longCommand = `atmos list stacks 2>/dev/null | grep -v '^$' | wc -l; atmos list stacks 2>/dev/null | head -50`

// renderTool is a Tool with a configurable name and description.
type renderTool struct {
	name        string
	description string
}

func (r renderTool) Name() string        { return r.name }
func (r renderTool) Description() string { return r.description }
func (r renderTool) IsRestricted() bool  { return false }

// plainLines renders the request and returns its ANSI-stripped lines.
func plainLines(tool Tool, params map[string]interface{}, width int) []string {
	return strings.Split(ansi.Strip(renderRequest(tool, params, width)), "\n")
}

// assertFits fails when any line is wider than width.
func assertFits(t *testing.T, lines []string, width int) {
	t.Helper()
	for _, line := range lines {
		assert.LessOrEqual(t, runewidth.StringWidth(line), width, "line too wide: %q", line)
	}
}

func TestRenderRequest_TitleAndRows(t *testing.T) {
	tool := renderTool{name: "Bash", description: "Count and list Atmos stacks"}
	lines := plainLines(tool, map[string]interface{}{"command": "atmos list stacks"}, 80)

	assert.True(t, strings.HasPrefix(lines[0], requestTitle), "title first: %q", lines[0])
	assert.Contains(t, lines, "Tool  Bash")
	assert.Contains(t, lines, "Why   Count and list Atmos stacks")
	assert.Contains(t, lines, panelPrefix+"atmos list stacks")
	assertFits(t, lines, 80)
}

func TestRenderRequest_DescriptionAppearsOnce(t *testing.T) {
	tool := renderTool{name: "Bash", description: "Count and list Atmos stacks"}
	params := map[string]interface{}{
		"command":     "atmos list stacks",
		"description": "Count and list Atmos stacks",
	}

	out := ansi.Strip(renderRequest(tool, params, 80))

	assert.Equal(t, 1, strings.Count(out, "Count and list Atmos stacks"))
	assert.NotContains(t, out, "description")
}

func TestRenderRequest_DistinctDescriptionParamIsKept(t *testing.T) {
	tool := renderTool{name: "Bash", description: "Run a shell command"}
	params := map[string]interface{}{"command": "ls", "description": "List files"}

	out := ansi.Strip(renderRequest(tool, params, 80))

	assert.Contains(t, out, "Run a shell command")
	assert.Contains(t, out, "description  List files")
}

func TestRenderRequest_LongCommandWraps(t *testing.T) {
	tool := renderTool{name: "Bash", description: "Count and list Atmos stacks"}
	params := map[string]interface{}{"command": longCommand, "description": "Count and list Atmos stacks"}

	for _, width := range []int{40, 60, 80, 120} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			lines := plainLines(tool, params, width)
			assertFits(t, lines, width)

			// Rejoining the panel content recovers the command verbatim.
			var panel []string
			for _, line := range lines {
				if rest, ok := strings.CutPrefix(line, panelPrefix); ok {
					panel = append(panel, strings.TrimSpace(rest))
				}
			}
			assert.Equal(t, longCommand, strings.Join(panel, " "))
			if width < 100 {
				assert.Greater(t, len(panel), 1, "command should wrap at width %d", width)
			}
		})
	}
}

func TestRenderRequest_ContinuationLinesAreIndented(t *testing.T) {
	tool := renderTool{name: "Bash"}
	lines := plainLines(tool, map[string]interface{}{"command": longCommand}, 60)

	var panel []string
	for _, line := range lines {
		if rest, ok := strings.CutPrefix(line, panelPrefix); ok {
			panel = append(panel, rest)
		}
	}
	require.Greater(t, len(panel), 1)
	assert.False(t, strings.HasPrefix(panel[0], " "))
	for _, cont := range panel[1:] {
		assert.True(t, strings.HasPrefix(cont, continuationIndent), "continuation not indented: %q", cont)
	}
}

func TestRenderRequest_ShortCommandStaysOnOneLine(t *testing.T) {
	lines := plainLines(renderTool{name: "Bash"}, map[string]interface{}{"command": "ls -la"}, 80)

	count := 0
	for _, line := range lines {
		if strings.Contains(line, "ls -la") {
			count++
		}
	}
	assert.Equal(t, 1, count)
	assert.Contains(t, lines, panelPrefix+"ls -la")
}

func TestRenderRequest_MultilineCommandKeepsLines(t *testing.T) {
	lines := plainLines(renderTool{name: "Bash"}, map[string]interface{}{"command": "echo one\n  echo two"}, 80)

	assert.Contains(t, lines, panelPrefix+"echo one")
	assert.Contains(t, lines, panelPrefix+"  echo two")
}

func TestRenderRequest_OverlongWordIsSplit(t *testing.T) {
	word := strings.Repeat("x", 200)
	for _, width := range []int{40, 80} {
		lines := plainLines(renderTool{name: "Bash"}, map[string]interface{}{"command": "echo " + word}, width)
		assertFits(t, lines, width)
		// The space before the word is dropped at the break; every other character must survive.
		assert.Equal(t, "echo"+word, strings.Join(panelContent(lines), ""), "no characters may be lost")
	}
}

// panelContent returns the text of the command panel lines with whitespace trimmed.
func panelContent(lines []string) []string {
	var panel []string
	for _, line := range lines {
		if rest, ok := strings.CutPrefix(line, panelPrefix); ok {
			panel = append(panel, strings.TrimSpace(rest))
		}
	}
	return panel
}

func TestRenderRequest_ExtraParamsSortedWithHangingIndent(t *testing.T) {
	long := strings.Repeat("word ", 30)
	params := map[string]interface{}{
		"zeta":  "last",
		"alpha": "first",
		"mid":   strings.TrimSpace(long),
	}

	lines := plainLines(renderTool{name: "t"}, params, 60)
	assertFits(t, lines, 60)

	alpha, mid, zeta := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "alpha"):
			alpha = i
		case strings.HasPrefix(line, "mid"):
			mid = i
		case strings.HasPrefix(line, "zeta"):
			zeta = i
		}
	}
	require.NotEqual(t, -1, alpha)
	require.NotEqual(t, -1, mid)
	require.NotEqual(t, -1, zeta)
	assert.Less(t, alpha, mid)
	assert.Less(t, mid, zeta)

	// Keys are aligned to the widest key ("Tool"/"alpha"/"zeta"/"mid" -> 5) and
	// wrapped values hang under the value column.
	hanging := strings.Repeat(" ", rowIndent+len("alpha")+keyGap)
	assert.True(t, strings.HasPrefix(lines[mid], "mid    word"), "got %q", lines[mid])
	assert.True(t, strings.HasPrefix(lines[mid+1], hanging+"word"), "got %q", lines[mid+1])
}

func TestRenderRequest_LongValueIsCapped(t *testing.T) {
	value := strings.TrimSpace(strings.Repeat("line\n", 40))
	out := ansi.Strip(renderRequest(renderTool{name: "t"}, map[string]interface{}{"content": value}, 80))

	assert.Contains(t, out, "more lines")
	assert.LessOrEqual(t, strings.Count(out, "line"), maxValueLines)
}

func TestRenderRequest_EmptyParams(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{name: "nil", params: nil},
		{name: "empty", params: map[string]interface{}{}},
		{name: "blank values", params: map[string]interface{}{"a": "", "b": "  "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := plainLines(renderTool{name: "Bash", description: "Does things"}, tt.params, 80)

			assert.True(t, strings.HasPrefix(lines[0], requestTitle))
			assert.Equal(t, []string{"Tool  Bash", "Why   Does things"}, lines[1:])
		})
	}
}

func TestRenderRequest_NoDescriptionOmitsWhy(t *testing.T) {
	out := ansi.Strip(renderRequest(renderTool{name: "Bash"}, nil, 80))

	assert.NotContains(t, out, "Why")
}

func TestRenderRequest_TitleRuleIsWidthAware(t *testing.T) {
	for _, width := range []int{30, 60, 100} {
		title := ansi.Strip(strings.SplitN(renderRequest(renderTool{name: "t"}, nil, width), "\n", 2)[0])
		assert.Equal(t, width, runewidth.StringWidth(title))
	}
}

func TestPrettyToolName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "Bash", want: "Bash"},
		{name: "atmos tool", in: "atmos_list_stacks", want: "atmos_list_stacks"},
		{name: "mcp tool", in: "mcp__atmos__describe_component", want: "atmos → describe_component"},
		{name: "mcp tool with dashes", in: "mcp__claude-ai__list__things", want: "claude-ai → list__things"},
		{name: "mcp without tool", in: "mcp__atmos", want: "mcp__atmos"},
		{name: "mcp empty server", in: "mcp____tool", want: "mcp____tool"},
		{name: "single underscore is not mcp", in: "mcp_server_tool", want: "mcp_server_tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, prettyToolName(tt.in))
		})
	}
}

func TestRenderRequest_PrettifiesMCPName(t *testing.T) {
	out := ansi.Strip(renderRequest(renderTool{name: "mcp__atmos__list_stacks"}, nil, 80))

	assert.Contains(t, out, "Tool  atmos → list_stacks")
}

func TestWidthSelection(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "unknown falls back to 80", input: 0, want: 80},
		{name: "negative falls back to 80", input: -5, want: 80},
		{name: "terminal width leaves room for the form gutter", input: 90, want: 90 - formGutter},
		{name: "wide terminal is capped", input: 300, want: maxRenderWidth},
		{name: "narrow terminal has a floor", input: 20, want: minRenderWidth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, requestWidthFrom(tt.input))
		})
	}
}

func TestClampWidth(t *testing.T) {
	assert.Equal(t, defaultRenderWidth, clampWidth(0))
	assert.Equal(t, minRenderWidth, clampWidth(5))
	assert.Equal(t, 70, clampWidth(70))
	assert.Equal(t, maxRenderWidth, clampWidth(1000))
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		width  int
		indent string
		want   []string
	}{
		{name: "fits", text: "hello world", width: 20, want: []string{"hello world"}},
		{name: "wraps at spaces", text: "aaa bbb ccc", width: 7, want: []string{"aaa bbb", "ccc"}},
		{name: "continuation indent", text: "aaa bbb ccc", width: 7, indent: "  ", want: []string{"aaa bbb", "  ccc"}},
		{name: "keeps newlines", text: "a\nb", width: 10, want: []string{"a", "b"}},
		{name: "keeps leading indent", text: "  a b", width: 10, want: []string{"  a b"}},
		{name: "splits long word", text: "abcdefghij", width: 4, want: []string{"abcd", "efgh", "ij"}},
		{name: "expands tabs", text: "\ta", width: 10, want: []string{"    a"}},
		{name: "empty", text: "", width: 10, want: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, wrapText(tt.text, tt.width, tt.indent))
		})
	}
}

func TestSummarizeRequest(t *testing.T) {
	tool := renderTool{name: "Bash"}

	tests := []struct {
		name   string
		tool   Tool
		params map[string]interface{}
		max    int
		want   string
	}{
		{name: "command", tool: tool, params: map[string]interface{}{"command": "ls -la"}, max: 60, want: "Bash: ls -la"},
		{name: "multiline command collapses to one line", tool: tool, params: map[string]interface{}{"command": "echo a\n  echo b"}, max: 60, want: "Bash: echo a echo b"},
		{name: "long command is truncated", tool: tool, params: map[string]interface{}{"command": longCommand}, max: 40, want: "Bash: atmos list stacks 2>/dev/null | g…"},
		{name: "no params", tool: tool, params: nil, max: 60, want: "Bash"},
		{name: "path param", tool: renderTool{name: "Read"}, params: map[string]interface{}{"file_path": "/tmp/x"}, max: 60, want: "Read: /tmp/x"},
		{name: "mcp name", tool: renderTool{name: "mcp__atmos__list_stacks"}, params: nil, max: 60, want: "atmos → list_stacks"},
		{name: "tiny budget drops subject", tool: tool, params: map[string]interface{}{"command": "ls"}, max: 8, want: "Bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeRequest(tt.tool, tt.params, tt.max)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "\n")
		})
	}
}

func TestEscapeNoteMarkup(t *testing.T) {
	assert.Equal(t, `a\_b \*c\* \`+"`"+`d\`+"`"+` \\n`, escapeNoteMarkup("a_b *c* `d` \\n"))
	assert.Equal(t, "plain text", escapeNoteMarkup("plain text"))
}

func TestHandleCachedResponse_ReportsSaved(t *testing.T) {
	tests := []struct {
		response string
		allowed  bool
		saved    bool
	}{
		{response: choiceAllowOnce, allowed: true, saved: false},
		{response: choiceAlwaysAllow, allowed: true, saved: true},
		{response: choiceDenyOnce, allowed: false, saved: false},
		{response: choiceAlwaysDeny, allowed: false, saved: true},
		{response: "", allowed: false, saved: false},
	}

	for _, tt := range tests {
		t.Run("response_"+tt.response, func(t *testing.T) {
			p := NewCLIPrompterWithCache(newTestCache(t))

			allowed, saved := p.handleCachedResponse(tt.response, "Bash(ls)")

			assert.Equal(t, tt.allowed, allowed)
			assert.Equal(t, tt.saved, saved)
		})
	}
}

func TestAlwaysLabels(t *testing.T) {
	for _, tool := range []scopedFakeTool{
		{name: "Bash", key: "Bash(ls)"},
		{name: "Read", key: "Read(/repo/go.mod)"},
		{name: "Glob", key: "Glob(**/*.go)"},
		{name: "WebFetch", key: "WebFetch(https://example.com)"},
		{name: "mcp__field_test__write_marker", key: "mcp__field_test__write_marker"},
	} {
		t.Run(tool.name, func(t *testing.T) {
			assert.Equal(t, "Always allow "+prettyToolName(tool.key), alwaysAllowLabel(tool))
			assert.Equal(t, "Always deny "+prettyToolName(tool.key), alwaysDenyLabel(tool))
		})
	}
	assert.Equal(t, "Always allow atmos_list_stacks", alwaysAllowLabel(plainFakeTool{name: "atmos_list_stacks"}))
	assert.Equal(t, "Always deny atmos → list", alwaysDenyLabel(plainFakeTool{name: "mcp__atmos__list"}))
}

func TestPrintReceipt(t *testing.T) {
	tool := renderTool{name: "Bash"}
	params := map[string]interface{}{"command": "atmos list stacks\n| head"}

	tests := []struct {
		name     string
		allowed  bool
		saved    bool
		contains []string
	}{
		{name: "allowed once", allowed: true, contains: []string{"Allowed Bash: atmos list stacks | head"}},
		{name: "allowed always", allowed: true, saved: true, contains: []string{"Allowed Bash:", "saved to " + settingsPathHint}},
		{name: "denied", allowed: false, contains: []string{"Denied Bash: atmos list stacks | head"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := ansi.Strip(captureStderr(t, func() { printReceipt(tool, params, tt.allowed, tt.saved) }))

			for _, want := range tt.contains {
				assert.Contains(t, output, want)
			}
			// Exactly one receipt line, then a blank line.
			assert.True(t, strings.HasSuffix(output, "\n\n"), "receipt must be followed by a blank line: %q", output)
			assert.Len(t, strings.Split(strings.TrimSuffix(output, "\n\n"), "\n"), 1)
			assert.Equal(t, tt.saved, strings.Contains(output, "saved to"))
		})
	}
}

// captureStderr runs fn and returns what it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	oldStderr := os.Stderr
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	r.Close()

	return buf.String()
}
