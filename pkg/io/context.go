package io

import (
	"fmt"
	stdio "io"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// context implements the Context interface.
type context struct {
	streams  Streams
	terminal Terminal
	config   *Config
	masker   Masker
}

// NewContext creates a new I/O context with default configuration.
func NewContext(opts ...ContextOption) (Context, error) {
	// Build config from flags, env vars, and atmos.yaml
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build I/O config: %w", err)
	}

	// Create masker
	masker := newMasker(cfg)

	// Create streams with masking
	streams := newStreams(masker, cfg)

	// Create terminal detector
	terminal := newTerminal(cfg)

	ctx := &context{
		streams:  streams,
		terminal: terminal,
		config:   cfg,
		masker:   masker,
	}

	// Apply options
	for _, opt := range opts {
		opt(ctx)
	}

	return ctx, nil
}

// Channel access - explicit and clear.
func (c *context) Data() stdio.Writer {
	return c.streams.Output()
}

func (c *context) UI() stdio.Writer {
	return c.streams.Error()
}

func (c *context) Input() stdio.Reader {
	return c.streams.Input()
}

// Raw channels (unmasked).
func (c *context) RawData() stdio.Writer {
	return c.streams.RawOutput()
}

func (c *context) RawUI() stdio.Writer {
	return c.streams.RawError()
}

// Legacy compatibility.
func (c *context) Streams() Streams {
	return c.streams
}

func (c *context) Terminal() Terminal {
	return c.terminal
}

func (c *context) Config() *Config {
	return c.config
}

func (c *context) Masker() Masker {
	return c.masker
}

// ContextOption configures Context behavior.
type ContextOption func(*context)

// WithStreams sets custom streams (for testing).
func WithStreams(streams Streams) ContextOption {
	return func(c *context) {
		c.streams = streams
	}
}

// WithTerminal sets a custom terminal (for testing).
func WithTerminal(terminal Terminal) ContextOption {
	return func(c *context) {
		c.terminal = terminal
	}
}

// WithMasker sets a custom masker (for testing).
func WithMasker(masker Masker) ContextOption {
	return func(c *context) {
		c.masker = masker
	}
}

// buildConfig constructs Config from all sources.
func buildConfig() (*Config, error) {
	cfg := &Config{
		// From flags (bound via viper in cmd/root.go)
		NoColor:        viper.GetBool("no-color"),
		Color:          viper.GetBool("color"),
		RedirectStderr: viper.GetString("redirect-stderr"),
		DisableMasking: viper.GetBool("disable-masking"),

		// From environment variables
		EnvNoColor:       os.Getenv("NO_COLOR") != "",
		EnvCLIColor:      os.Getenv("CLICOLOR"),
		EnvCLIColorForce: os.Getenv("CLICOLOR_FORCE") != "",
		EnvTerm:          os.Getenv("TERM"),
		EnvColorTerm:     os.Getenv("COLORTERM"),
	}

	// Load atmos.yaml config (if available)
	// This may not be loaded yet during early initialization
	if viper.IsSet("settings") {
		var atmosConfig schema.AtmosConfiguration
		if err := viper.Unmarshal(&atmosConfig); err == nil {
			cfg.AtmosConfig = atmosConfig
		}
	}

	return cfg, nil
}

// ShouldUseColor determines if color should be used based on config priority.
// Priority (highest to lowest):
// 1. NO_COLOR env var - disables all color
// 2. CLICOLOR=0 - disables color (unless CLICOLOR_FORCE is set)
// 3. CLICOLOR_FORCE - forces color even for non-TTY
// 4. --no-color flag
// 5. --color flag
// 6. atmos.yaml terminal.no_color (deprecated)
// 7. atmos.yaml terminal.color
// 8. Default (true for TTY, false for non-TTY)
func (c *Config) ShouldUseColor(isTTY bool) bool {
	// 1. NO_COLOR always wins
	if c.EnvNoColor {
		return false
	}

	// 2. CLICOLOR=0 (unless forced)
	if c.EnvCLIColor == "0" && !c.EnvCLIColorForce {
		return false
	}

	// 3. CLICOLOR_FORCE overrides TTY detection
	if c.EnvCLIColorForce {
		return true
	}

	// 4. --no-color flag
	if c.NoColor {
		return false
	}

	// 5. --color flag
	if c.Color {
		return true
	}

	// 6-7. atmos.yaml config
	if c.AtmosConfig.Settings.Terminal.NoColor {
		return false
	}
	if !c.AtmosConfig.Settings.Terminal.Color {
		return false
	}

	// 8. Default based on TTY
	return isTTY
}

// DetectColorProfile determines the terminal's color capabilities.
func (c *Config) DetectColorProfile(isTTY bool) ColorProfile {
	// If color is disabled, return ColorNone
	if !c.ShouldUseColor(isTTY) {
		return ColorNone
	}

	// Check for truecolor support
	colorTerm := strings.ToLower(c.EnvColorTerm)
	if colorTerm == "truecolor" || colorTerm == "24bit" {
		return ColorTrue
	}

	// Check TERM for 256 color support
	term := strings.ToLower(c.EnvTerm)
	if strings.Contains(term, "256color") || strings.Contains(term, "256") {
		return Color256
	}

	// Check for any color support
	if strings.Contains(term, "color") || term == "xterm" || term == "screen" {
		return Color16
	}

	// Default to 16 colors if TTY and color enabled
	if isTTY {
		return Color16
	}

	return ColorNone
}
