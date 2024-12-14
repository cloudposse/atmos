package markdown

import (
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

// Option is a function that configures the renderer
type Option func(*Renderer)
