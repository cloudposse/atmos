package markdown

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultWidth = 80

// Renderer is a markdown renderer using Glamour
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
func NewHelpRenderer(atmosConfig schema.AtmosConfiguration, opts ...Option) (*Renderer, error) {
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

	// Get help-specific style
	style, err := GetHelpStyle()
	if err != nil {
		return nil, err
	}

	// Initialize glamour renderer with help style
	// Note: Do NOT use WithAutoStyle() as it overrides our custom styles
	renderer, err := glamour.NewTermRenderer(
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
	return result, err
}

// Render renders markdown content to ANSI styled text.
func (r *Renderer) Render(content string) (string, error) {
	var rendered string
	var err error
	if r.isTTYSupportForStdout() {
		rendered, err = r.renderer.Render(content)
	} else {
		// Fallback to ASCII rendering for non-TTY stdout
		rendered, err = r.RenderAscii(content)
	}
	if err != nil {
		return "", err
	}
	// Remove duplicate URLs and trailing newlines
	lines := strings.Split(rendered, "\n")
	var result []string

	// Create a purple style
	purpleStyle := termenv.Style{}.Foreground(r.profile.Color(Purple)).Bold()

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "$") && term.IsTTYSupportForStdout() {
			// Add custom styling for command examples
			styled := purpleStyle.Styled(line)
			result = append(result, " "+styled)
		} else if trimmed != "" {
			result = append(result, line)
		}
	}

	// Add a single newline at the end plus extra spacing
	return strings.Join(result, "\n"), nil
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
	return renderer.Render(content)
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
	return renderer.Render(content)
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
	if r.isTTYSupportForStderr() {
		return r.Render(content)
	}
	// Fallback to ASCII rendering for non-TTY stderr
	return r.RenderAscii(content)
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
