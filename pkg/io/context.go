package io

import (
	"fmt"
	stdio "io"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
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
	defer perf.Track(nil, "io.NewContext")()

	// Build config from flags, env vars, and atmos.yaml
	cfg := buildConfig()

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

// Write is the primary output method - ALL writes go through here for masking.
// This is the central choke point that ensures all output is masked.
func (c *context) Write(stream Stream, content string) error {
	defer perf.Track(nil, "io.context.Write")()

	// Apply masking
	masked := c.masker.Mask(content)

	// Route to appropriate stream
	var writer stdio.Writer
	switch stream {
	case DataStream:
		writer = c.streams.RawOutput() // Use raw since we already masked
	case UIStream:
		writer = c.streams.RawError() // Use raw since we already masked
	default:
		return fmt.Errorf("%w: %v", errUtils.ErrUnknownStream, stream)
	}

	// Write to stream
	_, err := fmt.Fprint(writer, masked)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrWriteToStream, err)
	}
	return nil
}

// Channel access - explicit and clear.
// Deprecated: Use Write() instead for automatic masking.
func (c *context) Data() stdio.Writer {
	defer perf.Track(nil, "io.context.Data")()

	return c.streams.Output()
}

func (c *context) UI() stdio.Writer {
	defer perf.Track(nil, "io.context.UI")()

	return c.streams.Error()
}

func (c *context) Input() stdio.Reader {
	defer perf.Track(nil, "io.context.Input")()

	return c.streams.Input()
}

// Raw channels (unmasked).
func (c *context) RawData() stdio.Writer {
	defer perf.Track(nil, "io.context.RawData")()

	return c.streams.RawOutput()
}

func (c *context) RawUI() stdio.Writer {
	defer perf.Track(nil, "io.context.RawUI")()

	return c.streams.RawError()
}

// Legacy compatibility.
func (c *context) Streams() Streams {
	defer perf.Track(nil, "io.context.Streams")()

	return c.streams
}

func (c *context) Config() *Config {
	defer perf.Track(nil, "io.context.Config")()

	return c.config
}

func (c *context) Masker() Masker {
	defer perf.Track(nil, "io.context.Masker")()

	return c.masker
}

// ContextOption configures Context behavior.
type ContextOption func(*context)

// WithStreams sets custom streams (for testing).
func WithStreams(streams Streams) ContextOption {
	defer perf.Track(nil, "io.WithStreams")()

	return func(c *context) {
		c.streams = streams
	}
}

// WithMasker sets a custom masker (for testing).
func WithMasker(masker Masker) ContextOption {
	defer perf.Track(nil, "io.WithMasker")()

	return func(c *context) {
		c.masker = masker
	}
}

// buildConfig constructs Config from all sources.
func buildConfig() *Config {
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

	return cfg
}
