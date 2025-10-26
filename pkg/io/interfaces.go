package io

import (
	stdio "io"
	"regexp"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Context provides access to I/O channels and masking.
// This is the main entry point for I/O operations.
//
// Key Principle: I/O layer provides CHANNELS and MASKING, not formatting or terminal detection.
// - Channels: Where does output go? (stdout, stderr, stdin)
// - Masking: Secret redaction for security
//
// ALL output must flow through Write() for automatic masking.
// The UI layer (pkg/ui/) handles formatting and rendering.
// The terminal layer (pkg/terminal/) handles TTY detection and capabilities.
type Context interface {
	// Primary output method - ALL writes should go through this for masking
	Write(stream Stream, content string) error

	// Channel access - explicit and clear (deprecated - use Write() instead)
	Data() stdio.Writer  // stdout - for pipeable data (JSON, YAML, results)
	UI() stdio.Writer    // stderr - for human messages (status, errors, prompts)
	Input() stdio.Reader // stdin - for user input

	// Raw channels (unmasked - requires justification)
	RawData() stdio.Writer // Unmasked stdout
	RawUI() stdio.Writer   // Unmasked stderr

	// Configuration
	Config() *Config

	// Output masking
	Masker() Masker

	// Legacy compatibility (deprecated - use Data()/UI() instead)
	Streams() Streams
}

// Streams provides access to input/output streams with automatic masking.
// Deprecated: Use Context.Data()/UI() directly instead.
// This interface exists for backward compatibility during migration.
type Streams interface {
	// Input returns the input stream (typically stdin).
	Input() stdio.Reader

	// Output returns the output stream (typically stdout) with automatic masking.
	// Deprecated: Use Context.Data() instead.
	Output() stdio.Writer

	// Error returns the error stream (typically stderr) with automatic masking.
	// Deprecated: Use Context.UI() instead.
	Error() stdio.Writer

	// RawOutput returns the unmasked output stream.
	// Use only when absolutely necessary (e.g., binary output).
	// Requires explicit justification in code review.
	// Deprecated: Use Context.RawData() instead.
	RawOutput() stdio.Writer

	// RawError returns the unmasked error stream.
	// Use only when absolutely necessary.
	// Requires explicit justification in code review.
	// Deprecated: Use Context.RawUI() instead.
	RawError() stdio.Writer
}

// Masker handles automatic masking of sensitive data in output.
type Masker interface {
	// RegisterValue registers a literal value to be masked.
	// The value will be replaced with ***MASKED*** in all output.
	RegisterValue(value string)

	// RegisterSecret registers a secret with encoding variations.
	// Automatically registers base64, URL, and JSON encoded versions.
	RegisterSecret(secret string)

	// RegisterPattern registers a regex pattern to mask.
	// Returns error if pattern is invalid.
	RegisterPattern(pattern string) error

	// RegisterRegex registers a compiled regex pattern to mask.
	RegisterRegex(pattern *regexp.Regexp)

	// RegisterAWSAccessKey registers an AWS access key and attempts to mask the paired secret key.
	RegisterAWSAccessKey(accessKeyID string)

	// Mask applies all registered masks to the input string.
	Mask(input string) string

	// Clear removes all registered masks.
	Clear()

	// Count returns the number of registered masks.
	Count() int

	// Enabled returns whether masking is enabled.
	Enabled() bool
}

// Config holds I/O configuration for channels and masking.
type Config struct {
	// From global flags
	RedirectStderr string
	DisableMasking bool // --disable-masking flag for debugging

	// From atmos.yaml
	AtmosConfig schema.AtmosConfiguration
}

// StreamType identifies an I/O stream.
// Deprecated: Use Channel instead for clearer semantics.
type StreamType int

const (
	StreamInput  StreamType = iota // stdin
	StreamOutput                   // stdout (Deprecated: use DataChannel)
	StreamError                    // stderr (Deprecated: use UIChannel)
)

// String returns the string representation of the stream type.
func (st StreamType) String() string {
	defer perf.Track(nil, "io.StreamType.String")()

	switch st {
	case StreamInput:
		return "stdin"
	case StreamOutput:
		return "stdout"
	case StreamError:
		return "stderr"
	default:
		return "unknown"
	}
}

// Stream identifies an I/O stream for writing output.
// Used with Context.Write(stream, content) for centralized masking.
type Stream int

const (
	DataStream Stream = iota // stdout - for pipeable data (JSON, YAML, results)
	UIStream                 // stderr - for human messages (status, errors, prompts)
)

// String returns the string representation of the stream.
func (s Stream) String() string {
	defer perf.Track(nil, "io.Stream.String")()

	switch s {
	case DataStream:
		return "data"
	case UIStream:
		return "ui"
	default:
		return "unknown"
	}
}

// Channel identifies an I/O channel with clear semantic meaning.
// Deprecated: Use Stream instead with Context.Write().
// This replaces StreamType with more intuitive naming.
type Channel int

const (
	DataChannel  Channel = iota // stdout - for pipeable data
	UIChannel                   // stderr - for human messages
	InputChannel                // stdin - for user input
)

// String returns the string representation of the channel.
func (c Channel) String() string {
	defer perf.Track(nil, "io.Channel.String")()

	switch c {
	case DataChannel:
		return "data (stdout)"
	case UIChannel:
		return "ui (stderr)"
	case InputChannel:
		return "input (stdin)"
	default:
		return "unknown"
	}
}

// ToStreamType converts Channel to legacy StreamType.
// This supports backward compatibility during migration.
func (c Channel) ToStreamType() StreamType {
	defer perf.Track(nil, "io.Channel.ToStreamType")()

	switch c {
	case DataChannel:
		return StreamOutput
	case UIChannel:
		return StreamError
	case InputChannel:
		return StreamInput
	default:
		return StreamOutput
	}
}
