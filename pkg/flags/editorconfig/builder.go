package editorconfig

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/flags"
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
	options []flags.Option
}

// NewEditorConfigOptionsBuilder creates a new builder for EditorConfigParser.
func NewEditorConfigOptionsBuilder() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.NewEditorConfigOptionsBuilder")()

	return &EditorConfigOptionsBuilder{
		options: []flags.Option{},
	}
}

// WithExclude adds the exclude flag.
// Maps to EditorConfigOptions.Exclude field.
func (b *EditorConfigOptionsBuilder) WithExclude() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithExclude")()

	b.options = append(b.options, flags.WithStringFlag("exclude", "", "", "Regex to exclude files from checking"))
	b.options = append(b.options, flags.WithEnvVars("exclude", "ATMOS_EDITORCONFIG_EXCLUDE"))
	return b
}

// WithInit adds the init flag.
// Maps to EditorConfigOptions.Init field.
func (b *EditorConfigOptionsBuilder) WithInit() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithInit")()

	b.options = append(b.options, flags.WithBoolFlag("init", "", false, "Create an initial configuration"))
	b.options = append(b.options, flags.WithEnvVars("init", "ATMOS_EDITORCONFIG_INIT"))
	return b
}

// WithIgnoreDefaults adds the ignore-defaults flag.
// Maps to EditorConfigOptions.IgnoreDefaults field.
func (b *EditorConfigOptionsBuilder) WithIgnoreDefaults() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithIgnoreDefaults")()

	b.options = append(b.options, flags.WithBoolFlag("ignore-defaults", "", false, "Ignore default excludes"))
	b.options = append(b.options, flags.WithEnvVars("ignore-defaults", "ATMOS_EDITORCONFIG_IGNORE_DEFAULTS"))
	return b
}

// WithDryRun adds the dry-run flag.
// Maps to EditorConfigOptions.DryRun field.
func (b *EditorConfigOptionsBuilder) WithDryRun() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithDryRun")()

	b.options = append(b.options, flags.WithBoolFlag("dry-run", "", false, "Show which files would be checked"))
	b.options = append(b.options, flags.WithEnvVars("dry-run", "ATMOS_EDITORCONFIG_DRY_RUN"))
	return b
}

// WithShowVersion adds the show-version flag.
// Maps to EditorConfigOptions.ShowVersion field.
func (b *EditorConfigOptionsBuilder) WithShowVersion() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithShowVersion")()

	b.options = append(b.options, flags.WithBoolFlag("show-version", "", false, "Print the version number"))
	b.options = append(b.options, flags.WithEnvVars("show-version", "ATMOS_EDITORCONFIG_SHOW_VERSION"))
	return b
}

// WithFormat adds the format flag with specified default value.
// Maps to EditorConfigOptions.Format field.
//
// Parameters:
//   - defaultValue: default format (e.g., "default", "gcc")
func (b *EditorConfigOptionsBuilder) WithFormat(defaultValue string) *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithFormat")()

	b.options = append(b.options, flags.WithStringFlag("format", "", defaultValue, "Specify the output format: default, gcc"))
	b.options = append(b.options, flags.WithEnvVars("format", "ATMOS_EDITORCONFIG_FORMAT"))
	return b
}

// WithDisableTrimTrailingWhitespace adds the disable-trim-trailing-whitespace flag.
// Maps to EditorConfigOptions.DisableTrimTrailingWhitespace field.
func (b *EditorConfigOptionsBuilder) WithDisableTrimTrailingWhitespace() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithDisableTrimTrailingWhitespace")()

	b.options = append(b.options, flags.WithBoolFlag("disable-trim-trailing-whitespace", "", false, "Disable trailing whitespace check"))
	b.options = append(b.options, flags.WithEnvVars("disable-trim-trailing-whitespace", "ATMOS_EDITORCONFIG_DISABLE_TRIM_TRAILING_WHITESPACE"))
	return b
}

// WithDisableEndOfLine adds the disable-end-of-line flag.
// Maps to EditorConfigOptions.DisableEndOfLine field.
func (b *EditorConfigOptionsBuilder) WithDisableEndOfLine() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithDisableEndOfLine")()

	b.options = append(b.options, flags.WithBoolFlag("disable-end-of-line", "", false, "Disable end-of-line check"))
	b.options = append(b.options, flags.WithEnvVars("disable-end-of-line", "ATMOS_EDITORCONFIG_DISABLE_END_OF_LINE"))
	return b
}

// WithDisableInsertFinalNewline adds the disable-insert-final-newline flag.
// Maps to EditorConfigOptions.DisableInsertFinalNewline field.
func (b *EditorConfigOptionsBuilder) WithDisableInsertFinalNewline() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithDisableInsertFinalNewline")()

	b.options = append(b.options, flags.WithBoolFlag("disable-insert-final-newline", "", false, "Disable final newline check"))
	b.options = append(b.options, flags.WithEnvVars("disable-insert-final-newline", "ATMOS_EDITORCONFIG_DISABLE_INSERT_FINAL_NEWLINE"))
	return b
}

// WithDisableIndentation adds the disable-indentation flag.
// Maps to EditorConfigOptions.DisableIndentation field.
func (b *EditorConfigOptionsBuilder) WithDisableIndentation() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithDisableIndentation")()

	b.options = append(b.options, flags.WithBoolFlag("disable-indentation", "", false, "Disable indentation check"))
	b.options = append(b.options, flags.WithEnvVars("disable-indentation", "ATMOS_EDITORCONFIG_DISABLE_INDENTATION"))
	return b
}

// WithDisableIndentSize adds the disable-indent-size flag.
// Maps to EditorConfigOptions.DisableIndentSize field.
func (b *EditorConfigOptionsBuilder) WithDisableIndentSize() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithDisableIndentSize")()

	b.options = append(b.options, flags.WithBoolFlag("disable-indent-size", "", false, "Disable indent size check"))
	b.options = append(b.options, flags.WithEnvVars("disable-indent-size", "ATMOS_EDITORCONFIG_DISABLE_INDENT_SIZE"))
	return b
}

// WithDisableMaxLineLength adds the disable-max-line-length flag.
// Maps to EditorConfigOptions.DisableMaxLineLength field.
func (b *EditorConfigOptionsBuilder) WithDisableMaxLineLength() *EditorConfigOptionsBuilder {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.WithDisableMaxLineLength")()

	b.options = append(b.options, flags.WithBoolFlag("disable-max-line-length", "", false, "Disable max line length check"))
	b.options = append(b.options, flags.WithEnvVars("disable-max-line-length", "ATMOS_EDITORCONFIG_DISABLE_MAX_LINE_LENGTH"))
	return b
}

// Build creates the EditorConfigParser with all configured flags.
func (b *EditorConfigOptionsBuilder) Build() *EditorConfigParser {
	defer perf.Track(nil, "flags.EditorConfigOptionsBuilder.Build")()

	return &EditorConfigParser{
		parser: flags.NewStandardFlagParser(b.options...),
	}
}
