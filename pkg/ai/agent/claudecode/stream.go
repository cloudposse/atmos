package claudecode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/approval"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// Message types emitted by `claude --output-format stream-json`.
	msgTypeAssistant     = "assistant"
	msgTypeUser          = "user"
	msgTypeResult        = "result"
	msgTypeControlReq    = "control_request"
	msgTypeControlCancel = "control_cancel_request"

	// Content block types inside assistant and user messages.
	blockTypeToolUse    = "tool_use"
	blockTypeToolResult = "tool_result"

	// The readerBufferSize constant is the initial buffer of the NDJSON reader. Lines longer than
	// this are still read in full (bufio.Reader.ReadBytes grows as needed).
	readerBufferSize = 64 * 1024

	// The summaryMaxLen constant is the maximum number of runes in a tool summary.
	summaryMaxLen   = 60
	summaryEllipsis = "…"
)

// streamMessage is the envelope of one NDJSON line from Claude Code.
type streamMessage struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	RequestID string          `json:"request_id"`
	Request   *controlRequest `json:"request"`
	Message   *messageBody    `json:"message"`
}

// messageBody is the "message" object of assistant and user lines.
type messageBody struct {
	// Content is usually a list of blocks but may be a plain string for user lines.
	Content json.RawMessage `json:"content"`
}

// contentBlock is one block of an assistant or user message.
type contentBlock struct {
	Type      string         `json:"type"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
	ToolUseID string         `json:"tool_use_id"`
	IsError   bool           `json:"is_error"`
}

// controlRequest is the payload of a "control_request" line.
type controlRequest struct {
	Subtype   string         `json:"subtype"`
	ToolName  string         `json:"tool_name"`
	ToolUseID string         `json:"tool_use_id"`
	Input     map[string]any `json:"input"`
}

// blocks returns the content blocks of a message, or nil when the content is not a block list.
func (m *messageBody) blocks() []contentBlock {
	if m == nil || len(m.Content) == 0 {
		return nil
	}
	var blocks []contentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil
	}
	return blocks
}

// lineReader reads newline-delimited JSON of arbitrary line length.
type lineReader struct {
	r *bufio.Reader
}

// newLineReader wraps r in an NDJSON line reader.
func newLineReader(r io.Reader) *lineReader {
	return &lineReader{r: bufio.NewReaderSize(r, readerBufferSize)}
}

// next returns the next non-empty line with surrounding whitespace removed. At the end
// of the stream it returns the last unterminated line (if any) together with io.EOF.
func (l *lineReader) next() ([]byte, error) {
	for {
		line, err := l.r.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 || err != nil {
			return trimmed, err
		}
	}
}

// parseStreamLine decodes one NDJSON line. Malformed lines return false and are logged at debug level.
func parseStreamLine(line []byte) (*streamMessage, bool) {
	var msg streamMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		log.Debug("Skipping malformed Claude Code stream line", "error", err, "bytes", len(line))
		return nil, false
	}
	if msg.Type == "" {
		log.Debug("Skipping Claude Code stream line without a type", "bytes", len(line))
		return nil, false
	}
	return &msg, true
}

// isEOF reports whether err marks the normal end of the stream.
func isEOF(err error) bool {
	return errors.Is(err, io.EOF)
}

// toolInfo remembers a tool call so its result event can carry the tool name.
type toolInfo struct {
	name    string
	summary string
}

// progressTracker converts stream messages into approval.Event notifications.
type progressTracker struct {
	handler func(approval.Event)
	tools   map[string]toolInfo
}

// newProgressTracker creates a tracker. A nil handler makes it a no-op.
func newProgressTracker(handler func(approval.Event)) *progressTracker {
	return &progressTracker{handler: handler, tools: make(map[string]toolInfo)}
}

// observe emits ToolStart for tool_use blocks in assistant messages and ToolDone for
// tool_result blocks in user messages.
func (t *progressTracker) observe(msg *streamMessage) {
	if t.handler == nil {
		return
	}
	switch msg.Type {
	case msgTypeAssistant:
		for _, block := range msg.Message.blocks() {
			if block.Type != blockTypeToolUse {
				continue
			}
			info := toolInfo{name: block.Name, summary: summarize(block.Name, block.Input)}
			t.tools[block.ID] = info
			t.handler(approval.Event{Kind: approval.ToolStart, Tool: info.name, Summary: info.summary})
		}
	case msgTypeUser:
		for _, block := range msg.Message.blocks() {
			if block.Type != blockTypeToolResult {
				continue
			}
			info := t.tools[block.ToolUseID]
			delete(t.tools, block.ToolUseID)
			t.handler(approval.Event{Kind: approval.ToolDone, Tool: info.name, Summary: info.summary})
		}
	}
}

// summarize returns a short human-readable description of a tool call.
func summarize(tool string, input map[string]any) string {
	switch tool {
	case "Bash":
		return truncateSummary(firstLine(stringField(input, "command")), tool)
	case "Read", "Edit", "Write", "MultiEdit", "NotebookEdit":
		return truncateSummary(stringField(input, "file_path"), tool)
	case "Grep", "Glob":
		return truncateSummary(stringField(input, "pattern"), tool)
	case "WebFetch":
		return truncateSummary(stringField(input, "url"), tool)
	default:
		return tool
	}
}

// truncateSummary shortens s to summaryMaxLen runes and falls back to the tool name when s is empty.
func truncateSummary(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	runes := []rune(s)
	if len(runes) <= summaryMaxLen {
		return s
	}
	return string(runes[:summaryMaxLen-1]) + summaryEllipsis
}

// firstLine returns the first line of s.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// stringField returns input[key] when it is a string.
func stringField(input map[string]any, key string) string {
	v, ok := input[key].(string)
	if !ok {
		return ""
	}
	return v
}
