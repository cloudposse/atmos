package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// EditorConfigOptions provides strongly-typed access to validate editorconfig command flags.
// Used for the validate editorconfig command which wraps the editorconfig-checker library.
//
// Embeds GlobalFlags for global Atmos flags (chdir, config, logs, etc.).
// Provides editorconfig-specific command fields (Exclude, Init, Format, Disable flags, etc.).
type EditorConfigOptions struct {
	GlobalFlags // Embedded global flags (chdir, config, logs, etc.)

	// Common editorconfig flags.
	Exclude        string // Regex to exclude files from checking (--exclude)
	Init           bool   // Create an initial configuration (--init)
	IgnoreDefaults bool   // Ignore default excludes (--ignore-defaults)
	DryRun         bool   // Show which files would be checked (--dry-run)
	ShowVersion    bool   // Print the version number (--version)
	Format         string // Output format: default, gcc (--format)

	// Disable flags (disable specific checks).
	DisableTrimTrailingWhitespace bool // Disable trailing whitespace check (--disable-trim-trailing-whitespace)
	DisableEndOfLine              bool // Disable end-of-line check (--disable-end-of-line)
	DisableInsertFinalNewline     bool // Disable final newline check (--disable-insert-final-newline)
	DisableIndentation            bool // Disable indentation check (--disable-indentation)
	DisableIndentSize             bool // Disable indent size check (--disable-indent-size)
	DisableMaxLineLength          bool // Disable max line length check (--disable-max-line-length)

	// Tracking which flags were explicitly provided via CLI.
	// These enable proper precedence: CLI > config > default.
	InitProvided                          bool
	IgnoreDefaultsProvided                bool
	DryRunProvided                        bool
	DisableTrimTrailingWhitespaceProvided bool
	DisableEndOfLineProvided              bool
	DisableInsertFinalNewlineProvided     bool
	DisableIndentationProvided            bool
	DisableIndentSizeProvided             bool
	DisableMaxLineLengthProvided          bool
}

// Value returns the resolved editorconfig options for use in command execution.
func (e *EditorConfigOptions) Value() EditorConfigOptions {
	defer perf.Track(nil, "flags.EditorConfigOptions.Value")()

	return *e
}
