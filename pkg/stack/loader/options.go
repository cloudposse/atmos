package loader

import "github.com/cloudposse/atmos/pkg/perf"

// LoadOption configures the loading behavior.
type LoadOption func(*LoadOptions)

// LoadOptions contains options for loading configuration files.
type LoadOptions struct {
	// StrictMode enables strict parsing (e.g., fail on unknown keys).
	StrictMode bool

	// AllowDuplicateKeys allows duplicate keys in maps (last one wins).
	AllowDuplicateKeys bool

	// PreserveComments preserves comments in the parsed structure.
	PreserveComments bool

	// SourceFile is the path to the source file for error messages.
	SourceFile string
}

// DefaultLoadOptions returns the default load options.
func DefaultLoadOptions() *LoadOptions {
	defer perf.Track(nil, "loader.DefaultLoadOptions")()

	return &LoadOptions{
		StrictMode:         false,
		AllowDuplicateKeys: true,
		PreserveComments:   false,
	}
}

// WithStrictMode enables strict parsing mode.
func WithStrictMode(strict bool) LoadOption {
	defer perf.Track(nil, "loader.WithStrictMode")()

	return func(o *LoadOptions) {
		o.StrictMode = strict
	}
}

// WithAllowDuplicateKeys configures whether duplicate keys are allowed.
func WithAllowDuplicateKeys(allow bool) LoadOption {
	defer perf.Track(nil, "loader.WithAllowDuplicateKeys")()

	return func(o *LoadOptions) {
		o.AllowDuplicateKeys = allow
	}
}

// WithPreserveComments configures whether comments should be preserved.
func WithPreserveComments(preserve bool) LoadOption {
	defer perf.Track(nil, "loader.WithPreserveComments")()

	return func(o *LoadOptions) {
		o.PreserveComments = preserve
	}
}

// WithSourceFile sets the source file path for error messages.
func WithSourceFile(path string) LoadOption {
	defer perf.Track(nil, "loader.WithSourceFile")()

	return func(o *LoadOptions) {
		o.SourceFile = path
	}
}

// ApplyLoadOptions applies the given options to the base options.
func ApplyLoadOptions(opts ...LoadOption) *LoadOptions {
	defer perf.Track(nil, "loader.ApplyLoadOptions")()

	o := DefaultLoadOptions()
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// EncodeOption configures the encoding behavior.
type EncodeOption func(*EncodeOptions)

// EncodeOptions contains options for encoding data to configuration format.
type EncodeOptions struct {
	// Indent specifies the indentation string (e.g., "  " for 2 spaces).
	Indent string

	// SortKeys sorts map keys alphabetically.
	SortKeys bool

	// IncludeComments includes comments in the output.
	IncludeComments bool

	// CompactOutput minimizes whitespace in the output.
	CompactOutput bool
}

// DefaultEncodeOptions returns the default encode options.
func DefaultEncodeOptions() *EncodeOptions {
	defer perf.Track(nil, "loader.DefaultEncodeOptions")()

	return &EncodeOptions{
		Indent:          "  ",
		SortKeys:        false,
		IncludeComments: false,
		CompactOutput:   false,
	}
}

// WithIndent sets the indentation string.
func WithIndent(indent string) EncodeOption {
	defer perf.Track(nil, "loader.WithIndent")()

	return func(o *EncodeOptions) {
		o.Indent = indent
	}
}

// WithSortKeys enables alphabetical sorting of map keys.
func WithSortKeys(sort bool) EncodeOption {
	defer perf.Track(nil, "loader.WithSortKeys")()

	return func(o *EncodeOptions) {
		o.SortKeys = sort
	}
}

// WithIncludeComments enables including comments in the output.
func WithIncludeComments(include bool) EncodeOption {
	defer perf.Track(nil, "loader.WithIncludeComments")()

	return func(o *EncodeOptions) {
		o.IncludeComments = include
	}
}

// WithCompactOutput enables compact output with minimal whitespace.
func WithCompactOutput(compact bool) EncodeOption {
	defer perf.Track(nil, "loader.WithCompactOutput")()

	return func(o *EncodeOptions) {
		o.CompactOutput = compact
	}
}

// ApplyEncodeOptions applies the given options to the base options.
func ApplyEncodeOptions(opts ...EncodeOption) *EncodeOptions {
	defer perf.Track(nil, "loader.ApplyEncodeOptions")()

	o := DefaultEncodeOptions()
	for _, opt := range opts {
		opt(o)
	}
	return o
}
