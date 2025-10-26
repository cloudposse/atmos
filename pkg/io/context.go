package io

import (
	"fmt"
	stdio "io"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// context implements the Context interface.
type context struct {
	streams Streams
	config  *Config
	masker  Masker
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

	ctx := &context{
		streams: streams,
		config:  cfg,
		masker:  masker,
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
		RedirectStderr: viper.GetString("redirect-stderr"),
		DisableMasking: viper.GetBool("disable-masking"),
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
