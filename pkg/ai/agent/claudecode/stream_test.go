package claudecode

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/approval"
)

func TestSummarize(t *testing.T) {
	long := strings.Repeat("a", 100)
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  string
	}{
		{"bash command", "Bash", map[string]any{"command": "atmos list stacks"}, "atmos list stacks"},
		{"bash first line only", "Bash", map[string]any{"command": "echo one\necho two"}, "echo one"},
		{"bash trims whitespace", "Bash", map[string]any{"command": "  \n ls -la\n"}, "ls -la"},
		{"bash truncated", "Bash", map[string]any{"command": long}, strings.Repeat("a", summaryMaxLen-1) + "…"},
		{"bash exactly at limit", "Bash", map[string]any{"command": strings.Repeat("b", summaryMaxLen)}, strings.Repeat("b", summaryMaxLen)},
		{"bash multibyte truncated by runes", "Bash", map[string]any{"command": strings.Repeat("é", 80)}, strings.Repeat("é", summaryMaxLen-1) + "…"},
		{"bash empty command", "Bash", map[string]any{"command": ""}, "Bash"},
		{"bash missing command", "Bash", map[string]any{}, "Bash"},
		{"bash non-string command", "Bash", map[string]any{"command": 42}, "Bash"},
		{"bash nil input", "Bash", nil, "Bash"},
		{"read", "Read", map[string]any{"file_path": "/tmp/a.txt"}, "/tmp/a.txt"},
		{"edit", "Edit", map[string]any{"file_path": "/tmp/b.go", "old_string": "x"}, "/tmp/b.go"},
		{"write", "Write", map[string]any{"file_path": "/tmp/c.md"}, "/tmp/c.md"},
		{"grep", "Grep", map[string]any{"pattern": "TODO.*"}, "TODO.*"},
		{"glob", "Glob", map[string]any{"pattern": "**/*.go"}, "**/*.go"},
		{"webfetch", "WebFetch", map[string]any{"url": "https://atmos.tools"}, "https://atmos.tools"},
		{"unknown tool", "Task", map[string]any{"description": "x"}, "Task"},
		{"mcp tool", "mcp__aws__list", nil, "mcp__aws__list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarize(tt.tool, tt.input)
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len([]rune(got)), summaryMaxLen)
		})
	}
}

func TestLineReader(t *testing.T) {
	big := strings.Repeat("z", 300*1024)
	input := "first\n\n  \n" + big + "\r\nlast-without-newline"
	r := newLineReader(strings.NewReader(input))

	line, err := r.next()
	require.NoError(t, err)
	assert.Equal(t, "first", string(line))

	line, err = r.next()
	require.NoError(t, err)
	require.Len(t, line, len(big))
	assert.Equal(t, big, string(line))

	line, err = r.next()
	assert.ErrorIs(t, err, io.EOF)
	assert.Equal(t, "last-without-newline", string(line))

	line, err = r.next()
	assert.ErrorIs(t, err, io.EOF)
	assert.Empty(t, line)
}

func TestParseStreamLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantOK   bool
		wantType string
	}{
		{"valid", `{"type":"system","subtype":"init"}`, true, "system"},
		{"not json", `hello`, false, ""},
		{"truncated json", `{"type":`, false, ""},
		{"json array", `[1,2]`, false, ""},
		{"missing type", `{"subtype":"init"}`, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := parseStreamLine([]byte(tt.line))
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantType, msg.Type)
			}
		})
	}
}

func parseLine(t *testing.T, line string) *streamMessage {
	t.Helper()
	msg, ok := parseStreamLine([]byte(line))
	require.True(t, ok)
	return msg
}

func TestProgressTracker(t *testing.T) {
	var events []approval.Event
	tracker := newProgressTracker(func(e approval.Event) { events = append(events, e) })

	// Text-only assistant message and a string-content user message produce no events.
	tracker.observe(parseLine(t, `{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`))
	tracker.observe(parseLine(t, `{"type":"user","message":{"content":"plain string"}}`))
	assert.Empty(t, events)

	// Two parallel tool calls, results arrive out of order.
	tracker.observe(parseLine(t, `{"type":"assistant","message":{"content":[
		{"type":"tool_use","id":"a","name":"Bash","input":{"command":"ls"}},
		{"type":"tool_use","id":"b","name":"Grep","input":{"pattern":"foo"}}]}}`))
	tracker.observe(parseLine(t, `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"b","content":"x"}]}}`))
	tracker.observe(parseLine(t, `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"a","content":"y","is_error":true}]}}`))
	// A result for an unknown call still emits ToolDone, without a name.
	tracker.observe(parseLine(t, `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"zzz"}]}}`))

	assert.Equal(t, []approval.Event{
		{Kind: approval.ToolStart, Tool: "Bash", Summary: "ls"},
		{Kind: approval.ToolStart, Tool: "Grep", Summary: "foo"},
		{Kind: approval.ToolDone, Tool: "Grep", Summary: "foo"},
		{Kind: approval.ToolDone, Tool: "Bash", Summary: "ls"},
		{Kind: approval.ToolDone},
	}, events)
}

func TestProgressTracker_NilHandler(t *testing.T) {
	tracker := newProgressTracker(nil)
	assert.NotPanics(t, func() {
		tracker.observe(parseLine(t, `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"a","name":"Bash","input":{}}]}}`))
	})
}
