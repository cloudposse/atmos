package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// EditorConfigOptionsBuilder provides a type-safe, fluent interface for building EditorConfigParser
// with strongly-typed flag definitions that map directly to EditorConfigOptions fields.
//
// Example:
//
//	parser := flags.NewEditorConfigOptionsBuilder().
//	    WithExclude().
//	    WithInit().
//	    WithIgnoreDefaults().
//	    WithDryRun().
//	    WithFormat("default").
//	    WithShowVersion().
//	    WithDisableTrimTrailingWhitespace().
//	    WithDisableEndOfLine().
//	    WithDisableInsertFinalNewline().
//	    WithDisableIndentation().
//	    WithDisableIndentSize().
//	    WithDisableMaxLineLength().
//	    Build()
//
//	opts, _ := parser.Parse(ctx, args)
//	if opts.Init {
//	    // Create initial configuration
//	}
type EditorConfigOptionsBuilder struct {
	options []Option
}

// NewEditorConfigOptionsBuilder creates a new builder for EditorConfigParser.
func NewEditorConfigOptionsBuilder() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.NewEditorConfigOptionsBuilder")()

	return &EditorConfigOptionsBuilder{
		options: []Option{},
	}
}

// WithExclude adds the exclude flag.
// Maps to EditorConfigOptions.Exclude field.
func (b *EditorConfigOptionsBuilder) WithExclude() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithStringFlag("exclude", "", "", "Regex to exclude files from checking"))
	b.options = append(b.options, WithEnvVars("exclude", "ATMOS_EDITORCONFIG_EXCLUDE"))
	return b
}

// WithInit adds the init flag.
// Maps to EditorConfigOptions.Init field.
func (b *EditorConfigOptionsBuilder) WithInit() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("init", "", false, "Create an initial configuration"))
	b.options = append(b.options, WithEnvVars("init", "ATMOS_EDITORCONFIG_INIT"))
	return b
}

// WithIgnoreDefaults adds the ignore-defaults flag.
// Maps to EditorConfigOptions.IgnoreDefaults field.
func (b *EditorConfigOptionsBuilder) WithIgnoreDefaults() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("ignore-defaults", "", false, "Ignore default excludes"))
	b.options = append(b.options, WithEnvVars("ignore-defaults", "ATMOS_EDITORCONFIG_IGNORE_DEFAULTS"))
	return b
}

// WithDryRun adds the dry-run flag.
// Maps to EditorConfigOptions.DryRun field.
func (b *EditorConfigOptionsBuilder) WithDryRun() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("dry-run", "", false, "Show which files would be checked"))
	b.options = append(b.options, WithEnvVars("dry-run", "ATMOS_EDITORCONFIG_DRY_RUN"))
	return b
}

// WithShowVersion adds the show-version flag.
// Maps to EditorConfigOptions.ShowVersion field.
func (b *EditorConfigOptionsBuilder) WithShowVersion() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("show-version", "", false, "Print the version number"))
	b.options = append(b.options, WithEnvVars("show-version", "ATMOS_EDITORCONFIG_SHOW_VERSION"))
	return b
}

// WithFormat adds the format flag with specified default value.
// Maps to EditorConfigOptions.Format field.
//
// Parameters:
//   - defaultValue: default format (e.g., "default", "gcc")
func (b *EditorConfigOptionsBuilder) WithFormat(defaultValue string) *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithStringFlag("format", "", defaultValue, "Specify the output format: default, gcc"))
	b.options = append(b.options, WithEnvVars("format", "ATMOS_EDITORCONFIG_FORMAT"))
	return b
}

// WithDisableTrimTrailingWhitespace adds the disable-trim-trailing-whitespace flag.
// Maps to EditorConfigOptions.DisableTrimTrailingWhitespace field.
func (b *EditorConfigOptionsBuilder) WithDisableTrimTrailingWhitespace() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("disable-trim-trailing-whitespace", "", false, "Disable trailing whitespace check"))
	b.options = append(b.options, WithEnvVars("disable-trim-trailing-whitespace", "ATMOS_EDITORCONFIG_DISABLE_TRIM_TRAILING_WHITESPACE"))
	return b
}

// WithDisableEndOfLine adds the disable-end-of-line flag.
// Maps to EditorConfigOptions.DisableEndOfLine field.
func (b *EditorConfigOptionsBuilder) WithDisableEndOfLine() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("disable-end-of-line", "", false, "Disable end-of-line check"))
	b.options = append(b.options, WithEnvVars("disable-end-of-line", "ATMOS_EDITORCONFIG_DISABLE_END_OF_LINE"))
	return b
}

// WithDisableInsertFinalNewline adds the disable-insert-final-newline flag.
// Maps to EditorConfigOptions.DisableInsertFinalNewline field.
func (b *EditorConfigOptionsBuilder) WithDisableInsertFinalNewline() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("disable-insert-final-newline", "", false, "Disable final newline check"))
	b.options = append(b.options, WithEnvVars("disable-insert-final-newline", "ATMOS_EDITORCONFIG_DISABLE_INSERT_FINAL_NEWLINE"))
	return b
}

// WithDisableIndentation adds the disable-indentation flag.
// Maps to EditorConfigOptions.DisableIndentation field.
func (b *EditorConfigOptionsBuilder) WithDisableIndentation() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("disable-indentation", "", false, "Disable indentation check"))
	b.options = append(b.options, WithEnvVars("disable-indentation", "ATMOS_EDITORCONFIG_DISABLE_INDENTATION"))
	return b
}

// WithDisableIndentSize adds the disable-indent-size flag.
// Maps to EditorConfigOptions.DisableIndentSize field.
func (b *EditorConfigOptionsBuilder) WithDisableIndentSize() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("disable-indent-size", "", false, "Disable indent size check"))
	b.options = append(b.options, WithEnvVars("disable-indent-size", "ATMOS_EDITORCONFIG_DISABLE_INDENT_SIZE"))
	return b
}

// WithDisableMaxLineLength adds the disable-max-line-length flag.
// Maps to EditorConfigOptions.DisableMaxLineLength field.
func (b *EditorConfigOptionsBuilder) WithDisableMaxLineLength() *EditorConfigOptionsBuilder {
	b.options = append(b.options, WithBoolFlag("disable-max-line-length", "", false, "Disable max line length check"))
	b.options = append(b.options, WithEnvVars("disable-max-line-length", "ATMOS_EDITORCONFIG_DISABLE_MAX_LINE_LENGTH"))
	return b
}

// Build creates the EditorConfigParser with all configured flags.
func (b *EditorConfigOptionsBuilder) Build() *EditorConfigParser {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.Build")()

	return &EditorConfigParser{
		parser: NewStandardFlagParser(b.options...),
	}
}
