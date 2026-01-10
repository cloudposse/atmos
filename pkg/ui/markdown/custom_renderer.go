// Package markdown provides custom markdown rendering with extended syntax support.
package markdown

import (
	"encoding/json"
	"reflect"
	"unsafe"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark"

	"github.com/cloudposse/atmos/pkg/ui/markdown/extensions"
)

// CustomRenderer is a markdown renderer that wraps glamour's TermRenderer
// with custom goldmark extensions for enhanced terminal formatting.
//
// It supports the following custom syntax:
//   - ((muted)) - Dark gray text for subtle/secondary info
//   - ~~strikethrough~~ - Also renders as muted gray (GFM strikethrough restyle)
//   - ==highlight== - Yellow background for emphasis
//   - [!BADGE text] - Styled badges with colored backgrounds
//   - > [!NOTE] - GitHub-style admonitions (NOTE, WARNING, TIP, etc.)
type CustomRenderer struct {
	glamour *glamour.TermRenderer
}

// CustomRendererOption configures the CustomRenderer.
type CustomRendererOption func(*customRendererConfig)

// customRendererConfig holds configuration for building the renderer.
type customRendererConfig struct {
	wordWrap         int
	colorProfile     termenv.Profile
	styles           *ansi.StyleConfig
	preserveNewLines bool
}

// WithWordWrap sets the word wrap width.
func WithWordWrap(width int) CustomRendererOption {
	return func(c *customRendererConfig) {
		c.wordWrap = width
	}
}

// WithColorProfile sets the terminal color profile.
func WithColorProfile(profile termenv.Profile) CustomRendererOption {
	return func(c *customRendererConfig) {
		c.colorProfile = profile
	}
}

// WithStyles sets the ANSI style configuration.
func WithStyles(styles *ansi.StyleConfig) CustomRendererOption {
	return func(c *customRendererConfig) {
		c.styles = styles
	}
}

// WithPreservedNewLines preserves newlines in the output.
func WithPreservedNewLines() CustomRendererOption {
	return func(c *customRendererConfig) {
		c.preserveNewLines = true
	}
}

// WithStylesFromJSONBytes sets the ANSI style configuration from JSON bytes.
func WithStylesFromJSONBytes(jsonBytes []byte) CustomRendererOption {
	return func(c *customRendererConfig) {
		var styles ansi.StyleConfig
		if err := json.Unmarshal(jsonBytes, &styles); err == nil {
			c.styles = &styles
		}
	}
}

// NewCustomRenderer creates a new CustomRenderer with the specified options.
// It builds a glamour renderer and extends it with custom goldmark extensions.
//
// Custom syntax support:
//   - ((text)) - Muted gray text for subtle/secondary information
//
// The muted syntax works via AST transformation: ((text)) is parsed into a
// custom Muted node, then transformed to a Strikethrough node before rendering.
// This integrates cleanly with glamour's ANSI renderer since it already knows
// how to handle Strikethrough (styled as muted gray in our config).
func NewCustomRenderer(opts ...CustomRendererOption) (*CustomRenderer, error) {
	// Default configuration.
	cfg := &customRendererConfig{
		wordWrap:     defaultWidth,
		colorProfile: termenv.TrueColor, // Default to TrueColor for best output.
	}

	// Apply options.
	for _, opt := range opts {
		opt(cfg)
	}

	// Build glamour options.
	glamourOpts := []glamour.TermRendererOption{
		glamour.WithWordWrap(cfg.wordWrap),
		glamour.WithColorProfile(cfg.colorProfile),
		glamour.WithEmoji(),
	}

	// Add styles if provided.
	if cfg.styles != nil {
		styleBytes, err := json.Marshal(cfg.styles)
		if err == nil {
			glamourOpts = append(glamourOpts, glamour.WithStylesFromJSONBytes(styleBytes))
		}
	} else {
		// Use built-in default style.
		defaultStyleBytes, err := getBuiltinDefaultStyle()
		if err != nil {
			return nil, err
		}
		glamourOpts = append(glamourOpts, glamour.WithStylesFromJSONBytes(defaultStyleBytes))
	}

	// Add preserve newlines if requested.
	if cfg.preserveNewLines {
		glamourOpts = append(glamourOpts, glamour.WithPreservedNewLines())
	}

	// Create glamour renderer.
	renderer, err := glamour.NewTermRenderer(glamourOpts...)
	if err != nil {
		return nil, err
	}

	// Access glamour's internal goldmark via reflection to add our extensions.
	// This is necessary because glamour doesn't expose its goldmark instance.
	md := getGlamourGoldmark(renderer)
	if md != nil {
		// Add our custom muted extension (parser + AST transformer).
		// The transformer converts ((muted)) to strikethrough, which glamour
		// already knows how to render with our muted gray style.
		extensions.NewMutedExtension().Extend(md)

		// Override the default Linkify email regex with a stricter pattern.
		// This prevents package references like foo/bar@1.0.0 from being
		// converted to mailto: links while preserving valid email auto-linking.
		extensions.NewStrictLinkifyExtension().Extend(md)
	}

	return &CustomRenderer{glamour: renderer}, nil
}

// getGlamourGoldmark extracts the internal goldmark.Markdown from a glamour.TermRenderer.
// This uses reflection because glamour doesn't expose its internal goldmark instance.
// Returns nil if the reflection fails (e.g., if glamour's internal structure changes).
func getGlamourGoldmark(renderer *glamour.TermRenderer) goldmark.Markdown {
	val := reflect.ValueOf(renderer).Elem()
	mdField := val.FieldByName("md")
	if !mdField.IsValid() {
		return nil
	}
	return *(*goldmark.Markdown)(unsafe.Pointer(mdField.UnsafeAddr()))
}

// Render converts markdown content to ANSI styled text.
func (r *CustomRenderer) Render(content string) (string, error) {
	return r.glamour.Render(content)
}

// Close is a no-op for compatibility with glamour.TermRenderer interface.
func (r *CustomRenderer) Close() error {
	return nil
}
