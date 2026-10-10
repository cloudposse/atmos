package permission

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	renderSpace   = " "
	renderNewline = "\n"
	// The defaultRenderWidth constant is used when the terminal width is unknown.
	defaultRenderWidth = 80
	// The maxRenderWidth constant caps the request block so it stays readable on very wide terminals.
	maxRenderWidth = 100
	// The minRenderWidth constant is the narrowest layout the renderer will produce.
	minRenderWidth = 30
	// The formGutter constant is the number of columns the huh form reserves for its own frame
	// (focus bar and padding), subtracted from the terminal width.
	formGutter = 4

	// The rowIndent constant is the left indent of key/value rows.
	rowIndent = 0
	// The keyGap constant is the number of spaces between the key column and the value column.
	keyGap = 2
	// The maxKeyWidth constant caps the key column so long parameter names cannot squeeze the values.
	maxKeyWidth = 14
	// The minValueWidth constant is the narrowest value column the renderer will wrap to.
	minValueWidth = 10
	// The maxValueLines constant caps the number of lines shown for a single non-command value.
	maxValueLines = 10
	// The continuationIndent constant is the extra indent given to wrapped command lines.
	continuationIndent = "  "
	// The panelPrefix constant is the left border of the command panel.
	panelPrefix = "│ "
	// The panelPrefixWidth constant is the display width of panelPrefix.
	panelPrefixWidth = 2
	// The tabWidth constant is the number of spaces a tab expands to in displayed values.
	tabWidth = 4

	// The requestTitle constant is the heading of the permission request block.
	requestTitle = "Permission required"
	// The mcpToolPrefix constant is the prefix Claude uses for MCP tool names: mcp__<server>__<tool>.
	mcpToolPrefix = "mcp__"
	// The mcpToolSeparator constant separates the server and tool in an MCP tool name.
	mcpToolSeparator = "__"
	// The commandParam constant is the parameter that carries a shell command.
	commandParam = "command"
	// The highlightLanguage constant is the chroma lexer used for the command panel.
	highlightLanguage = "bash"
	// The highlightStyle constant is the chroma style used for the command panel.
	highlightStyle = "dracula"
	// The ellipsis constant marks truncated text.
	ellipsis = "…"
)

// requestWidth returns the width the request block should be rendered to: the real
// terminal width minus the form gutter, falling back to 80 columns and capped at 100.
func requestWidth() int {
	return requestWidthFrom(terminal.New().Width(terminal.Stdout))
}

// requestWidthFrom derives the render width from a detected terminal width.
// A non-positive width means the terminal width is unknown.
func requestWidthFrom(termWidth int) int {
	if termWidth <= 0 {
		return clampWidth(defaultRenderWidth)
	}
	return clampWidth(termWidth - formGutter)
}

// clampWidth keeps a width within the renderer's supported range. Non-positive
// widths fall back to the default.
func clampWidth(width int) int {
	switch {
	case width <= 0:
		return defaultRenderWidth
	case width > maxRenderWidth:
		return maxRenderWidth
	case width < minRenderWidth:
		return minRenderWidth
	default:
		return width
	}
}

// prettyToolName turns Claude's MCP tool names (mcp__<server>__<tool>) into
// "server → tool". Other names are returned unchanged.
func prettyToolName(name string) string {
	rest, ok := strings.CutPrefix(name, mcpToolPrefix)
	if !ok {
		return name
	}
	server, tool, found := strings.Cut(rest, mcpToolSeparator)
	if !found || server == "" || tool == "" {
		return name
	}
	return server + " → " + tool
}

// requestRow is one key/value row of the request block.
type requestRow struct {
	key   string
	value string
}

// renderRequest renders the full permission request block (title plus body) for the
// given width. It is pure apart from reading the current theme, so the terminal
// width is injected by the caller.
func renderRequest(tool Tool, params map[string]interface{}, width int) string {
	title, body := renderRequestParts(tool, params, width)
	return title + renderNewline + body
}

// renderRequestParts renders the request as a title line and a body. The body is
// the aligned key/value rows followed by the command panel, when there is a command.
func renderRequestParts(tool Tool, params map[string]interface{}, width int) (string, string) {
	width = clampWidth(width)

	rows, command := collectRows(tool, params)

	var lines []string
	lines = append(lines, renderRows(rows, width)...)
	if command != "" {
		lines = append(lines, renderCommandPanel(command, width)...)
	}

	return renderTitle(width), strings.Join(lines, renderNewline)
}

// renderTitle renders the heading followed by a muted rule that fills the width.
func renderTitle(width int) string {
	styles := theme.GetCurrentStyles()
	fill := width - runewidth.StringWidth(requestTitle) - 1
	if fill <= 0 {
		return styles.Title.Render(requestTitle)
	}
	return styles.Title.Render(requestTitle) + renderSpace + styles.Muted.Render(strings.Repeat("─", fill))
}

// collectRows builds the key/value rows (Tool, Why, then sorted parameters) and
// extracts the shell command, if any. A row whose value repeats an earlier value is
// omitted, so the description is shown only once.
func collectRows(tool Tool, params map[string]interface{}) ([]requestRow, string) {
	rows := []requestRow{{key: "Tool", value: prettyToolName(tool.Name())}}
	seen := map[string]bool{}

	if why := strings.TrimSpace(tool.Description()); why != "" {
		rows = append(rows, requestRow{key: "Why", value: why})
		seen[why] = true
	}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	command := ""
	for _, key := range keys {
		value := strings.TrimRight(fmt.Sprintf("%v", params[key]), " \t\r\n")
		if key == commandParam {
			command = value
			continue
		}
		normalized := strings.TrimSpace(value)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		rows = append(rows, requestRow{key: key, value: value})
	}

	return rows, command
}

// renderRows renders aligned key/value rows. Values wrap to the width with a hanging
// indent under the value column.
func renderRows(rows []requestRow, width int) []string {
	styles := theme.GetCurrentStyles()

	keyWidth := 0
	for _, row := range rows {
		keyWidth = max(keyWidth, min(runewidth.StringWidth(row.key), maxKeyWidth))
	}
	valueWidth := max(width-rowIndent-keyWidth-keyGap, minValueWidth)
	hanging := strings.Repeat(renderSpace, rowIndent+keyWidth+keyGap)

	var out []string
	for _, row := range rows {
		key := runewidth.FillRight(runewidth.Truncate(row.key, maxKeyWidth, ellipsis), keyWidth)
		wrapped := wrapText(row.value, valueWidth, "")
		wrapped = capLines(wrapped, maxValueLines)
		for i, line := range wrapped {
			if i == 0 {
				out = append(out, strings.Repeat(renderSpace, rowIndent)+styles.Muted.Render(key)+strings.Repeat(renderSpace, keyGap)+line)
				continue
			}
			out = append(out, hanging+line)
		}
	}
	return out
}

// renderCommandPanel renders a shell command in its own indented panel with a left
// border. The command is wrapped to the width (continuation lines are indented) and
// syntax highlighted line by line when color is available.
func renderCommandPanel(command string, width int) []string {
	styles := theme.GetCurrentStyles()
	bar := styles.Muted.Render(panelPrefix)

	out := []string{styles.Muted.Render(strings.TrimRight(panelPrefix, renderSpace))}
	for _, line := range wrapText(command, width-panelPrefixWidth, continuationIndent) {
		out = append(out, bar+highlightShell(line))
	}
	return append(out, styles.Muted.Render(strings.TrimRight(panelPrefix, renderSpace)))
}

// highlightShell applies bash syntax highlighting to a single line, returning the
// line unchanged when color is disabled or highlighting fails.
func highlightShell(line string) string {
	if strings.TrimSpace(line) == "" || ui.GetColorProfile() == termenv.Ascii {
		return line
	}
	highlighted, err := uiutils.HighlightCode(line, highlightLanguage, highlightStyle)
	if err != nil {
		return line
	}
	return strings.TrimRight(highlighted, renderNewline)
}

// capLines limits lines to max entries, replacing the overflow with a summary line.
func capLines(lines []string, limit int) []string {
	if len(lines) <= limit {
		return lines
	}
	hidden := len(lines) - (limit - 1)
	out := append([]string{}, lines[:limit-1]...)
	return append(out, fmt.Sprintf("%s %d more lines", ellipsis, hidden))
}

// wrapText wraps text to width display columns. Existing newlines are preserved.
// Lines are broken at spaces; a word longer than the line is split. Continuation
// lines are prefixed with contIndent (in addition to the line's own indentation).
// No returned line is wider than width.
func wrapText(text string, width int, contIndent string) []string {
	text = strings.ReplaceAll(text, "\r\n", renderNewline)
	text = strings.ReplaceAll(text, "\t", strings.Repeat(renderSpace, tabWidth))
	width = max(width, 1)

	var out []string
	for _, line := range strings.Split(text, renderNewline) {
		out = append(out, wrapLine(line, width, contIndent)...)
	}
	return out
}

// wrapLine wraps a single logical line.
func wrapLine(line string, width int, contIndent string) []string {
	line = strings.TrimRight(line, renderSpace)
	if runewidth.StringWidth(line) <= width {
		return []string{line}
	}

	lead := line[:len(line)-len(strings.TrimLeft(line, renderSpace))]
	if runewidth.StringWidth(lead)+runewidth.StringWidth(contIndent) > width/2 {
		lead = ""
	}
	indent := lead + contIndent
	if runewidth.StringWidth(indent) > width/2 {
		indent = ""
	}

	w := &lineWrapper{width: width, indent: indent, cur: lead}
	for _, token := range splitSpaces(strings.TrimLeft(line, renderSpace)) {
		w.add(token)
	}
	return w.finish()
}

// lineWrapper accumulates tokens into lines no wider than width.
type lineWrapper struct {
	width   int
	indent  string
	cur     string
	hasWord bool
	out     []string
}

// add appends a token (a run of spaces or a word), starting a new line when needed.
func (w *lineWrapper) add(token string) {
	if strings.HasPrefix(token, renderSpace) {
		w.addSpaces(token)
		return
	}
	w.addWord(token)
}

// addSpaces appends a run of spaces, dropping it when the line breaks here.
func (w *lineWrapper) addSpaces(token string) {
	if !w.hasWord {
		return
	}
	if runewidth.StringWidth(w.cur)+len(token) > w.width {
		w.flush()
		return
	}
	w.cur += token
}

// addWord appends a word, breaking the line or splitting the word when it does not fit.
func (w *lineWrapper) addWord(token string) {
	tokenWidth := runewidth.StringWidth(token)
	if runewidth.StringWidth(w.cur)+tokenWidth > w.width && w.hasWord {
		w.flush()
	}
	for runewidth.StringWidth(w.cur)+runewidth.StringWidth(token) > w.width {
		room := w.width - runewidth.StringWidth(w.cur)
		if room <= 0 {
			w.flush()
			continue
		}
		head := runewidth.Truncate(token, room, "")
		if head == "" {
			// A single rune wider than the room: force progress.
			head = string([]rune(token)[:1])
		}
		w.cur += head
		w.hasWord = true
		token = strings.TrimPrefix(token, head)
		w.flush()
	}
	w.cur += token
	w.hasWord = true
}

// flush ends the current line and starts a continuation line.
func (w *lineWrapper) flush() {
	w.out = append(w.out, strings.TrimRight(w.cur, renderSpace))
	w.cur = w.indent
	w.hasWord = false
}

// finish returns the accumulated lines.
func (w *lineWrapper) finish() []string {
	if w.hasWord || len(w.out) == 0 {
		w.out = append(w.out, strings.TrimRight(w.cur, renderSpace))
	}
	return w.out
}

// splitSpaces splits s into alternating runs of spaces and non-space words.
func splitSpaces(s string) []string {
	var tokens []string
	start := 0
	for i := 1; i <= len(s); i++ {
		if i == len(s) || (s[i] == ' ') != (s[start] == ' ') {
			tokens = append(tokens, s[start:i])
			start = i
		}
	}
	return tokens
}

// summarizeRequest returns a short one-line description of the request, such as
// "Bash: atmos list stacks…", truncated to at most maxWidth display columns.
func summarizeRequest(tool Tool, params map[string]interface{}, maxWidth int) string {
	name := prettyToolName(tool.Name())
	subject := requestSubject(params)
	if subject == "" {
		return name
	}
	budget := maxWidth - runewidth.StringWidth(name) - len(": ")
	if budget < minValueWidth {
		return name
	}
	return name + ": " + truncateOneLine(subject, budget)
}

// subjectParams lists the parameters, in priority order, that best identify what a tool call acts on.
var subjectParams = []string{commandParam, "file_path", "path", "pattern", "url", "query"}

// requestSubject picks the parameter value that best identifies the request.
func requestSubject(params map[string]interface{}) string {
	for _, key := range subjectParams {
		if value, ok := params[key]; ok {
			if s := strings.TrimSpace(fmt.Sprintf("%v", value)); s != "" {
				return s
			}
		}
	}
	return ""
}

// truncateOneLine collapses all whitespace (including newlines) and truncates the
// result to at most maxWidth display columns, marking truncation with an ellipsis.
func truncateOneLine(s string, maxWidth int) string {
	collapsed := strings.Join(strings.Fields(s), renderSpace)
	return runewidth.Truncate(collapsed, maxWidth, ellipsis)
}

// escapeNoteMarkup escapes the characters huh's Note field would otherwise interpret
// as inline markdown (backslash, underscore, asterisk, and backtick), so commands are
// displayed exactly as written.
func escapeNoteMarkup(s string) string {
	return strings.NewReplacer(`\`, `\\`, `_`, `\_`, `*`, `\*`, "`", "\\`").Replace(s)
}
