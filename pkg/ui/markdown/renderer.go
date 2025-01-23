package markdown

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"
)

// Renderer is a markdown renderer using Glamour
type Renderer struct {
	renderer *glamour.TermRenderer
	width    uint
	profile  termenv.Profile
}

// NewRenderer creates a new markdown renderer with the given options
func NewRenderer(opts ...Option) (*Renderer, error) {
	r := &Renderer{
		width:   80,                     // default width
		profile: termenv.ColorProfile(), // default color profile
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	// Get default style
	style, err := GetDefaultStyle()
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

// Render renders markdown content to ANSI styled text
func (r *Renderer) Render(content string) (string, error) {
	return r.renderer.Render(content)
}

// RenderWithStyle renders markdown content with a specific style
func (r *Renderer) RenderWithStyle(content string, style []byte) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(int(r.width)),
		glamour.WithStylesFromJSONBytes(style),
		glamour.WithColorProfile(r.profile),
		glamour.WithEmoji(),
	)
	if err != nil {
		return "", err
	}

	return renderer.Render(content)
}

// RenderWorkflow renders workflow documentation with specific styling
func (r *Renderer) RenderWorkflow(content string) (string, error) {
	// Add workflow header
	content = "# Workflow\n\n" + content
	return r.Render(content)
}

// RenderError renders an error message with specific styling
func (r *Renderer) RenderError(title, details, suggestion string) (string, error) {
	var content string

	if title != "" {
		content += fmt.Sprintf("\n# %s\n\n", title)
	}

	if details != "" {
		content += fmt.Sprintf("%s\n\n", details)
	}

	if suggestion != "" {
		if strings.HasPrefix(suggestion, "http") {
			content += fmt.Sprintf("\nFor more information, refer to the [docs](%s)\n", suggestion)
		} else {
			content += suggestion
		}
	}

	rendered, err := r.Render(content)
	if err != nil {
		return "", err
	}

	// Remove duplicate URLs and trailing newlines
	lines := strings.Split(rendered, "\n")
	var result []string
	seenURL := false

	// Create a purple style
	purpleStyle := termenv.Style{}.Foreground(r.profile.Color(Purple)).Bold()

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "https://") {
			if !seenURL {
				seenURL = true
				result = append(result, line)
			}
		} else if strings.HasPrefix(trimmed, "$") {
			// Add custom styling for command examples
			styled := purpleStyle.Styled(strings.TrimSpace(line))
			result = append(result, " "+styled)
		} else if trimmed != "" {
			result = append(result, line)
		}
	}

	// Add a single newline at the end plus extra spacing
	return "\n" + strings.Join(result, "\n") + "\n\n", nil
}

// RenderSuccess renders a success message with specific styling
func (r *Renderer) RenderSuccess(title, details string) (string, error) {
	content := fmt.Sprintf("# %s\n\n", title)

	if details != "" {
		content += fmt.Sprintf("## Details\n%s\n\n", details)
	}

	return r.Render(content)
}

// Option is a function that configures the renderer
type Option func(*Renderer)

// WithWidth sets the word wrap width for the renderer
func WithWidth(width uint) Option {
	return func(r *Renderer) {
		r.width = width
	}
}

// WithColorProfile sets the color profile for the renderer
func WithColorProfile(profile termenv.Profile) Option {
	return func(r *Renderer) {
		r.profile = profile
	}
}
