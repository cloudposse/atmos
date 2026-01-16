package env

import "github.com/cloudposse/atmos/pkg/perf"

// Option configures formatting behavior.
type Option func(*config)

// config holds the configuration for formatting operations.
type config struct {
	uppercase        bool
	flatten          bool
	flattenSeparator string
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
