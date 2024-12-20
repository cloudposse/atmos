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

	// Initialize glamour renderer
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(int(r.width)),
		glamour.WithStylesFromJSONBytes(DefaultStyle),
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
func (r *Renderer) RenderError(title, details, examples string) (string, error) {
	var content string

	if details != "" {
		content += fmt.Sprintf("%s\n\n", details)
	}

	if examples != "" {
		if !strings.Contains(examples, "## Examples") {
			content += fmt.Sprintf("## Examples\n\n%s", examples)
		} else {
			content += examples
		}
	}

	return r.Render(content)
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
