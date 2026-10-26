package env

import "github.com/cloudposse/atmos/pkg/perf"

// Option configures formatting behavior.
type Option func(*config)

// config holds the configuration for formatting operations.
type config struct {
	uppercase        bool
	flatten          bool
	flattenSeparator string
	exportPrefix     *bool // nil = use format default, explicit value overrides.
}

// WithUppercase converts all keys to uppercase.
func WithUppercase() Option {
	defer perf.Track(nil, "env.WithUppercase")()

	return func(c *config) {
		c.uppercase = true
	}
}

// WithFlatten flattens nested maps using the specified separator.
// For example, {"a": {"b": "c"}} with separator "_" becomes {"a_b": "c"}.
func WithFlatten(separator string) Option {
	defer perf.Track(nil, "env.WithFlatten")()

	return func(c *config) {
		c.flatten = true
		c.flattenSeparator = separator
	}
}

// WithExport controls whether bash format includes the 'export' prefix.
// Default is true (export KEY='value'). Set to false for KEY='value' without export.
// This option only affects FormatBash; other formats ignore it.
func WithExport(export bool) Option {
	defer perf.Track(nil, "env.WithExport")()

	return func(c *config) {
		c.exportPrefix = &export
	}
}
