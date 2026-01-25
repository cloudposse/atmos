// Package markdown provides custom markdown rendering with extended syntax support.
package markdown

import (
	"encoding/json"
	"os"
	"reflect"
	"unsafe"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark"

	"github.com/cloudposse/atmos/pkg/perf"
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
	defer perf.Track(nil, "markdown.NewCustomRenderer")()

	cfg := newDefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	glamourOpts, err := buildGlamourOptions(cfg)
	if err != nil {
		return nil, err
	}

	renderer, err := glamour.NewTermRenderer(glamourOpts...)
	if err != nil {
		return nil, err
	}

	// Extend glamour with custom extensions.
	extendGlamourWithCustomExtensions(renderer)

	return &CustomRenderer{glamour: renderer}, nil
}

// newDefaultConfig creates a customRendererConfig with default values.
func newDefaultConfig() *customRendererConfig {
	return &customRendererConfig{
		wordWrap:     defaultWidth,
		colorProfile: termenv.TrueColor,
	}
}

// buildGlamourOptions builds glamour renderer options from config.
func buildGlamourOptions(cfg *customRendererConfig) ([]glamour.TermRendererOption, error) {
	glamourOpts := []glamour.TermRendererOption{
		glamour.WithWordWrap(cfg.wordWrap),
		glamour.WithColorProfile(cfg.colorProfile),
		glamour.WithEmoji(),
	}

	// Only apply custom styles when color output is enabled.
	// Glamour with Ascii profile correctly strips ANSI codes when no styles are applied,
	// but custom styles with Bold/Italic/Underline still produce ANSI sequences.
	// Skip styles for Ascii profile to ensure clean plaintext output for NO_COLOR mode.
	styleOpts, err := buildStyleOptions(cfg)
	if err != nil {
		return nil, err
	}
	glamourOpts = append(glamourOpts, styleOpts...)

	if cfg.preserveNewLines {
		glamourOpts = append(glamourOpts, glamour.WithPreservedNewLines())
	}

	return glamourOpts, nil
}

// buildStyleOptions returns glamour style options based on config.
// Returns empty slice for Ascii profile (NO_COLOR mode) to avoid ANSI output.
func buildStyleOptions(cfg *customRendererConfig) ([]glamour.TermRendererOption, error) {
	// Skip styles for Ascii profile to ensure clean plaintext output.
	if cfg.colorProfile == termenv.Ascii {
		return nil, nil
	}

	// Use custom styles if provided.
	if cfg.styles != nil {
		styleBytes, err := json.Marshal(cfg.styles)
		if err == nil {
			return []glamour.TermRendererOption{glamour.WithStylesFromJSONBytes(styleBytes)}, nil
		}
		// Fall through to use default styles if custom styles fail to marshal.
		// This is a defensive fallback since StyleConfig should always marshal.
	}

	// Fall back to builtin default style.
	defaultStyleBytes, err := getBuiltinDefaultStyle()
	if err != nil {
		return nil, err
	}
	return []glamour.TermRendererOption{glamour.WithStylesFromJSONBytes(defaultStyleBytes)}, nil
}

// extendGlamourWithCustomExtensions adds custom goldmark extensions to the renderer.
func extendGlamourWithCustomExtensions(renderer *glamour.TermRenderer) {
	md := getGlamourGoldmark(renderer)
	if md == nil {
		return
	}
	// Add admonition extension (converts > [!NOTE] etc. to styled callouts).
	extensions.NewAdmonitionExtension().Extend(md)
	// Add muted extension (converts ((text)) to muted gray text).
	extensions.NewMutedExtension().Extend(md)
	// Add highlight extension (converts ==text== to highlighted text).
	extensions.NewHighlightExtension().Extend(md)
	// Add badge extension (converts [!BADGE text] to styled badges).
	extensions.NewBadgeExtension().Extend(md)
	// Add strict linkify (prevents foo/bar@1.0.0 from becoming mailto: links).
	extensions.NewStrictLinkifyExtension().Extend(md)
}

// getGlamourGoldmark extracts the internal goldmark.Markdown from a glamour.TermRenderer.
// This uses reflection because glamour doesn't expose its internal goldmark instance.
// Returns nil if the reflection fails (e.g., if glamour's internal structure changes).
// Tested against glamour v0.10.0 - revisit if glamour is upgraded.
func getGlamourGoldmark(renderer *glamour.TermRenderer) goldmark.Markdown {
	val := reflect.ValueOf(renderer).Elem()
	mdField := val.FieldByName("md")
	if !mdField.IsValid() {
		return nil
	}
	return *(*goldmark.Markdown)(unsafe.Pointer(mdField.UnsafeAddr()))
}

// Render converts markdown content to ANSI styled text.
// Stdout is temporarily suppressed during rendering to prevent glamour from printing
// "Warning: unhandled element" messages when it encounters markdown elements it can't handle.
func (r *CustomRenderer) Render(content string) (string, error) {
	// Suppress glamour warnings by temporarily redirecting stdout to /dev/null.
	// Glamour's internal ANSI renderer prints "Warning: unhandled element" directly to stdout
	// via fmt.Println when it encounters nodes it doesn't know how to handle.
	// These warnings are not useful to users and can be confusing.
	oldStdout := os.Stdout
	devNull, err := os.Open(os.DevNull)
	if err == nil {
		os.Stdout = devNull
		defer func() {
			os.Stdout = oldStdout
			devNull.Close()
		}()
	}

	return r.glamour.Render(content)
}

// Close is a no-op for compatibility with glamour.TermRenderer interface.
func (r *CustomRenderer) Close() error {
	return nil
}
