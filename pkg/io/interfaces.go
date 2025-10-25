package io

import (
	stdio "io"
	"regexp"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Context provides access to all I/O primitives.
// This is the main entry point for I/O operations.
//
// Key Principle: I/O layer provides CHANNELS and CAPABILITIES, not formatting.
// - Channels: Where does output go? (stdout, stderr, stdin)
// - Capabilities: What can the terminal do? (color, TTY, width)
//
// The UI layer (pkg/ui/) handles formatting and rendering.
type Context interface {
	// Channel access - explicit and clear
	Data() stdio.Writer    // stdout - for pipeable data (JSON, YAML, results)
	UI() stdio.Writer      // stderr - for human messages (status, errors, prompts)
	Input() stdio.Reader   // stdin - for user input

	// Raw channels (unmasked - requires justification)
	RawData() stdio.Writer  // Unmasked stdout
	RawUI() stdio.Writer    // Unmasked stderr

	// Terminal capabilities
	Terminal() Terminal

	// Configuration
	Config() *Config

	// Output masking
	Masker() Masker

	// Legacy compatibility (deprecated - use Data()/UI() instead)
	Streams() Streams
}

// Streams provides access to input/output streams with automatic masking.
// DEPRECATED: Use Context.Data()/UI() directly instead.
// This interface exists for backward compatibility during migration.
type Streams interface {
	// Input returns the input stream (typically stdin).
	Input() stdio.Reader

	// Output returns the output stream (typically stdout) with automatic masking.
	// DEPRECATED: Use Context.Data() instead.
	Output() stdio.Writer

	// Error returns the error stream (typically stderr) with automatic masking.
	// DEPRECATED: Use Context.UI() instead.
	Error() stdio.Writer

	// RawOutput returns the unmasked output stream.
	// Use only when absolutely necessary (e.g., binary output).
	// Requires explicit justification in code review.
	// DEPRECATED: Use Context.RawData() instead.
	RawOutput() stdio.Writer

	// RawError returns the unmasked error stream.
	// Use only when absolutely necessary.
	// Requires explicit justification in code review.
	// DEPRECATED: Use Context.RawUI() instead.
	RawError() stdio.Writer
}

// Terminal provides terminal capability detection and operations.
// NO FORMATTING - only capabilities and detection.
type Terminal interface {
	// IsTTY returns whether the given stream is a TTY.
	// Accepts either Channel or StreamType for backward compatibility.
	IsTTY(stream interface{}) bool

	// ColorProfile returns the terminal's color capabilities.
	ColorProfile() ColorProfile

	// Width returns the terminal width for the given stream.
	// Returns 0 if width cannot be determined.
	// Accepts either Channel or StreamType for backward compatibility.
	Width(stream interface{}) int

	// Height returns the terminal height for the given stream.
	// Returns 0 if height cannot be determined.
	// Accepts either Channel or StreamType for backward compatibility.
	Height(stream interface{}) int

	// SetTitle sets the terminal window title (if supported).
	// Does nothing if terminal doesn't support titles or if disabled in config.
	SetTitle(title string)

	// RestoreTitle restores the original terminal title.
	RestoreTitle()

	// Alert emits a terminal bell/alert (if supported and enabled).
	Alert()
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

// Config holds I/O configuration from flags, environment variables, and atmos.yaml.
type Config struct {
	// From global flags
	NoColor        bool
	Color          bool
	RedirectStderr string
	DisableMasking bool // --disable-masking flag for debugging

	// From environment variables
	EnvNoColor       bool   // NO_COLOR
	EnvCLIColor      string // CLICOLOR
	EnvCLIColorForce bool   // CLICOLOR_FORCE
	EnvTerm          string // TERM
	EnvColorTerm     string // COLORTERM

	// From atmos.yaml
	AtmosConfig schema.AtmosConfiguration
}

// ColorProfile represents terminal color capabilities.
type ColorProfile int

const (
	ColorNone  ColorProfile = iota // No color (Ascii)
	Color16                        // 16 colors (ANSI)
	Color256                       // 256 colors (ANSI256)
	ColorTrue                      // 24-bit color (TrueColor)
)

// String returns the string representation of the color profile.
func (cp ColorProfile) String() string {
	switch cp {
	case ColorNone:
		return "None"
	case Color16:
		return "16"
	case Color256:
		return "256"
	case ColorTrue:
		return "TrueColor"
	default:
		return "Unknown"
	}
}

// StreamType identifies an I/O stream.
// DEPRECATED: Use Channel instead for clearer semantics.
type StreamType int

const (
	StreamInput  StreamType = iota // stdin
	StreamOutput                   // stdout (DEPRECATED: use DataChannel)
	StreamError                    // stderr (DEPRECATED: use UIChannel)
)

// String returns the string representation of the stream type.
func (st StreamType) String() string {
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

// Channel identifies an I/O channel with clear semantic meaning.
// This replaces StreamType with more intuitive naming.
type Channel int

const (
	DataChannel  Channel = iota // stdout - for pipeable data
	UIChannel                   // stderr - for human messages
	InputChannel                // stdin - for user input
)

// String returns the string representation of the channel.
func (c Channel) String() string {
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
