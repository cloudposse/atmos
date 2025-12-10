package markdown

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultWidth = 80

// trimTrailingSpaces removes trailing spaces and tabs from each line while preserving blank lines.
// IMPORTANT: This function ONLY removes whitespace at the END of lines (before the \n).
// It NEVER removes newlines themselves. Newlines must always be preserved.
// Only trailing spaces and tabs (horizontal whitespace) are removed.
//
// Line breaks and spacing should be controlled by:
//   - Markdown content itself (blank lines between paragraphs, etc.)
//   - Markdown stylesheets (renderer configuration)
//   - NOT by post-processing that removes newlines
func trimTrailingSpaces(s string) string {
	lines := strings.Split(s, newline)
	for i, line := range lines {
		// Only trim trailing spaces and tabs, NOT newlines.
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, newline)
}

// Renderer is a markdown renderer using Glamour.
type Renderer struct {
	renderer              *glamour.TermRenderer
	width                 uint
	profile               termenv.Profile
	atmosConfig           *schema.AtmosConfiguration
	isTTYSupportForStdout func() bool
	isTTYSupportForStderr func() bool
}

// NewRenderer creates a new Markdown renderer with the given options.
func NewRenderer(atmosConfig schema.AtmosConfiguration, opts ...Option) (*Renderer, error) {
	r := &Renderer{
		width:                 defaultWidth,           // default width
		profile:               termenv.ColorProfile(), // default color profile
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
		isTTYSupportForStderr: term.IsTTYSupportForStderr,
		atmosConfig:           &atmosConfig,
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	if atmosConfig.Settings.Terminal.NoColor {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(styles.AsciiStyle),
			glamour.WithWordWrap(int(r.width)),
			glamour.WithColorProfile(r.profile),
			glamour.WithEmoji(),
		)
		if err != nil {
			return nil, err
		}

		r.renderer = renderer
		return r, nil
	}

	// Get default style
	style, err := GetDefaultStyle(atmosConfig)
	if err != nil {
		return nil, err
	}

	// Initialize glamour renderer
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(int(r.width)),
		glamour.WithStylesFromJSONBytes(style),
		glamour.WithColorProfile(r.profile),
		glamour.WithEmoji(),
	)
	if err != nil {
		return nil, err
	}

	r.renderer = renderer
	return r, nil
}

// NewHelpRenderer creates a new Markdown renderer specifically for command help text.
// This uses the Cloud Posse color scheme (grayscale + purple) with transparent backgrounds.
func NewHelpRenderer(atmosConfig *schema.AtmosConfiguration, opts ...Option) (*Renderer, error) {
	defer perf.Track(atmosConfig, "markdown.NewHelpRenderer")()

	r := &Renderer{
		width:                 defaultWidth,           // default width
		profile:               termenv.ColorProfile(), // default color profile
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
		isTTYSupportForStderr: term.IsTTYSupportForStderr,
		atmosConfig:           atmosConfig,
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	// Convert width safely from uint to int.
	width := r.width
	maxInt := ^uint(0) >> 1
	if width > maxInt {
		width = maxInt
	}
	wordWrap := int(width) // #nosec G115 -- width is validated above

	if atmosConfig.Settings.Terminal.NoColor {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(styles.AsciiStyle),
			glamour.WithWordWrap(wordWrap),
			glamour.WithColorProfile(r.profile),
			glamour.WithEmoji(),
		)
		if err != nil {
			return nil, err
		}

		r.renderer = renderer
		return r, nil
	}

	// Get help-specific style.
	style, err := GetHelpStyle()
	if err != nil {
		return nil, err
	}

	// Initialize glamour renderer with help style.
	// Note: Do NOT use WithAutoStyle() as it overrides our custom styles.
	renderer, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(wordWrap),
		glamour.WithStylesFromJSONBytes(style),
		glamour.WithColorProfile(r.profile),
		glamour.WithEmoji(),
	)
	if err != nil {
		return nil, err
	}

	r.renderer = renderer
	return r, nil
}

func (r *Renderer) RenderWithoutWordWrap(content string) (string, error) {
	// Render without line wrapping
	var out *glamour.TermRenderer
	var err error
	if r.atmosConfig.Settings.Terminal.NoColor {
		out, err = glamour.NewTermRenderer(
			glamour.WithStandardStyle(styles.AsciiStyle),
			glamour.WithWordWrap(0),
			glamour.WithColorProfile(r.profile),
			glamour.WithEmoji(),
		)
		if err != nil {
			return "", err
		}
	} else {
		// Get default style
		style, err := GetDefaultStyle(*r.atmosConfig)
		if err != nil {
			return "", err
		}
		out, err = glamour.NewTermRenderer(
			glamour.WithAutoStyle(), // Uses terminal's default style
			glamour.WithWordWrap(0),
			glamour.WithStylesFromJSONBytes(style),
			glamour.WithColorProfile(r.profile),
			glamour.WithEmoji(),
		)
		if err != nil {
			return "", err
		}
	}
	result := ""
	if r.isTTYSupportForStdout() {
		result, err = out.Render(content)
	} else {
		// Fallback to ASCII rendering for non-TTY stdout
		result, err = r.RenderAsciiWithoutWordWrap(content)
	}
	if err == nil {
		result = trimTrailingSpaces(result)
	}
	return result, err
}

// Render renders markdown content to ANSI styled text.
func (r *Renderer) Render(content string) (string, error) {
	var rendered string
	var err error
	if r.isTTYSupportForStdout() {
		rendered, err = r.renderer.Render(content)
	} else {
		// Fallback to ASCII rendering for non-TTY stdout.
		rendered, err = r.RenderAscii(content)
	}
	if err != nil {
		return "", err
	}
	// Post-process the rendered output to handle trailing newlines and command styling.
	lines := strings.Split(rendered, newline)
	var result []string

	// Create a purple style for command examples.
	purpleStyle := termenv.Style{}.Foreground(r.profile.Color(Purple)).Bold()

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "$") && term.IsTTYSupportForStdout() {
			// Add custom styling for command examples.
			styled := purpleStyle.Styled(line)
			result = append(result, " "+styled)
		} else {
			// Keep all lines including blank lines for proper markdown paragraph spacing.
			result = append(result, line)
		}
	}

	// Remove only trailing blank lines.
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	// Join lines and trim trailing spaces from each line.
	output := strings.Join(result, newline)
	return trimTrailingSpaces(output), nil
}

func (r *Renderer) RenderAsciiWithoutWordWrap(content string) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styles.AsciiStyle),
		glamour.WithWordWrap(0),
		glamour.WithColorProfile(r.profile),
		glamour.WithEmoji(),
	)
	if err != nil {
		return "", err
	}
	result, err := renderer.Render(content)
	if err == nil {
		result = trimTrailingSpaces(result)
	}
	return result, err
}

func (r *Renderer) RenderAscii(content string) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styles.AsciiStyle),
		glamour.WithWordWrap(int(r.width)),
		glamour.WithColorProfile(r.profile),
		glamour.WithEmoji(),
	)
	if err != nil {
		return "", err
	}
	result, err := renderer.Render(content)
	if err == nil {
		result = trimTrailingSpaces(result)
	}
	return result, err
}

// RenderWorkflow renders workflow documentation with specific styling.
func (r *Renderer) RenderWorkflow(content string) (string, error) {
	// Add workflow header
	content = "# Workflow\n\n" + content
	return r.Render(content)
}

// RenderError renders an error message with specific styling.
func (r *Renderer) RenderError(title, details, suggestion string) (string, error) {
	var content string

	if title != "" {
		content += fmt.Sprintf("\n# %s\n", title)
	}

	if details != "" {
		content += fmt.Sprintf("%s", details)
	}

	if suggestion != "" {
		if strings.HasPrefix(suggestion, "https://") || strings.HasPrefix(suggestion, "http://") {
			content += fmt.Sprintf("\n\nFor more information, refer to the [docs](%s)", suggestion)
		} else {
			content += suggestion
		}
	}
	return r.RenderErrorf(content)
}

// RenderErrorf renders an error message with specific styling.
func (r *Renderer) RenderErrorf(content string, args ...interface{}) (string, error) {
	var result string
	var err error
	if r.isTTYSupportForStderr() {
		result, err = r.Render(content)
	} else {
		// Fallback to ASCII rendering for non-TTY stderr
		result, err = r.RenderAscii(content)
	}
	// Note: trimTrailingSpaces already applied in Render() and RenderAscii()
	return result, err
}

// RenderSuccess renders a success message with specific styling.
func (r *Renderer) RenderSuccess(title, details string) (string, error) {
	content := fmt.Sprintf("# %s\n\n", title)

	if details != "" {
		content += fmt.Sprintf("## Details\n%s\n\n", details)
	}

	return r.Render(content)
}

// Option is a function that configures the renderer.
type Option func(*Renderer)

// WithWidth sets the word wrap width for the renderer.
func WithWidth(width uint) Option {
	return func(r *Renderer) {
		r.width = width
	}
}

func NewTerminalMarkdownRenderer(atmosConfig schema.AtmosConfiguration) (*Renderer, error) {
	maxWidth := atmosConfig.Settings.Docs.MaxWidth
	// Create a terminal writer to get the optimal width
	termWriter := term.NewResponsiveWriter(os.Stdout)
	var wr *term.TerminalWriter
	var ok bool
	var screenWidth uint = 1000
	if wr, ok = termWriter.(*term.TerminalWriter); ok {
		screenWidth = wr.GetWidth()
	}
	if maxWidth > 0 && ok {
		screenWidth = uint(min(maxWidth, int(wr.GetWidth())))
	} else if maxWidth > 0 {
		// Fallback: if type assertion fails, use maxWidth as the screen width.
		screenWidth = uint(maxWidth)
	}
	return NewRenderer(
		atmosConfig,
		WithWidth(screenWidth),
	)
}
