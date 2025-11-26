package flags

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Flag represents a command-line flag configuration.
// This is used by FlagRegistry to store reusable flag definitions.
type Flag interface {
	// GetName returns the long flag name (without -- prefix).
	GetName() string

	// GetShorthand returns the short flag name (single character, without - prefix).
	// Returns empty string if no shorthand.
	GetShorthand() string

	// GetDescription returns the help text for this flag.
	GetDescription() string

	// GetDefault returns the default value for this flag.
	GetDefault() interface{}

	// IsRequired returns true if this flag is required.
	IsRequired() bool

	// GetNoOptDefVal returns the value to use when flag is provided without a value.
	// Returns empty string if NoOptDefVal is not supported for this flag type.
	// This is used for the identity pattern: --identity (alone) vs --identity value.
	GetNoOptDefVal() string

	// GetEnvVars returns the list of environment variable names to bind to this flag.
	// Returns nil if no env vars.
	GetEnvVars() []string

	// GetCompletionFunc returns the custom completion function for this flag.
	// Returns nil if no custom completion is configured.
	// Custom completions enable dynamic shell completion based on codebase state.
	GetCompletionFunc() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// StringFlag represents a string-valued flag.
type StringFlag struct {
	Name           string
	Shorthand      string
	Default        string
	Description    string
	Required       bool
	NoOptDefVal    string   // Value when flag used without argument (identity pattern).
	EnvVars        []string // Environment variables to bind.
	ValidValues    []string // Valid values for this flag (enforced during validation).
	CompletionFunc func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// GetName implements Flag.
func (f *StringFlag) GetName() string {
	defer perf.Track(nil, "flags.StringFlag.GetName")()

	return f.Name
}

// GetShorthand implements Flag.
func (f *StringFlag) GetShorthand() string {
	defer perf.Track(nil, "flags.StringFlag.GetShorthand")()

	return f.Shorthand
}

// GetDescription implements Flag.
func (f *StringFlag) GetDescription() string {
	defer perf.Track(nil, "flags.StringFlag.GetDescription")()

	return f.Description
}

// GetDefault implements Flag.
func (f *StringFlag) GetDefault() interface{} {
	defer perf.Track(nil, "flags.StringFlag.GetDefault")()

	return f.Default
}

// IsRequired implements Flag.
func (f *StringFlag) IsRequired() bool {
	defer perf.Track(nil, "flags.StringFlag.IsRequired")()

	return f.Required
}

// GetNoOptDefVal implements Flag.
func (f *StringFlag) GetNoOptDefVal() string {
	defer perf.Track(nil, "flags.StringFlag.GetNoOptDefVal")()

	return f.NoOptDefVal
}

// GetEnvVars implements Flag.
func (f *StringFlag) GetEnvVars() []string {
	defer perf.Track(nil, "flags.StringFlag.GetEnvVars")()

	return f.EnvVars
}

// GetValidValues returns the list of valid values for this flag.
// Returns nil if no validation is needed.
func (f *StringFlag) GetValidValues() []string {
	defer perf.Track(nil, "flags.StringFlag.GetValidValues")()

	return f.ValidValues
}

// GetCompletionFunc implements Flag.
func (f *StringFlag) GetCompletionFunc() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "flags.StringFlag.GetCompletionFunc")()

	return f.CompletionFunc
}

// BoolFlag represents a boolean-valued flag.
type BoolFlag struct {
	Name        string
	Shorthand   string
	Default     bool
	Description string
	EnvVars     []string
}

// GetName implements Flag.
func (f *BoolFlag) GetName() string {
	defer perf.Track(nil, "flags.BoolFlag.GetName")()

	return f.Name
}

// GetShorthand implements Flag.
func (f *BoolFlag) GetShorthand() string {
	defer perf.Track(nil, "flags.BoolFlag.GetShorthand")()

	return f.Shorthand
}

// GetDescription implements Flag.
func (f *BoolFlag) GetDescription() string {
	defer perf.Track(nil, "flags.BoolFlag.GetDescription")()

	return f.Description
}

// GetDefault implements Flag.
func (f *BoolFlag) GetDefault() interface{} {
	defer perf.Track(nil, "flags.BoolFlag.GetDefault")()

	return f.Default
}

// IsRequired implements Flag.
func (f *BoolFlag) IsRequired() bool {
	defer perf.Track(nil, "flags.BoolFlag.IsRequired")()

	return false // Bool flags are never required
}

// GetNoOptDefVal implements Flag.
func (f *BoolFlag) GetNoOptDefVal() string {
	defer perf.Track(nil, "flags.BoolFlag.GetNoOptDefVal")()

	return "" // Bool flags don't use NoOptDefVal
}

// GetEnvVars implements Flag.
func (f *BoolFlag) GetEnvVars() []string {
	defer perf.Track(nil, "flags.BoolFlag.GetEnvVars")()

	return f.EnvVars
}

// GetCompletionFunc implements Flag.
func (f *BoolFlag) GetCompletionFunc() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "flags.BoolFlag.GetCompletionFunc")()

	return nil // Bool flags don't use custom completion.
}

// IntFlag represents an integer-valued flag.
type IntFlag struct {
	Name        string
	Shorthand   string
	Default     int
	Description string
	Required    bool
	EnvVars     []string
}

// GetName implements Flag.
func (f *IntFlag) GetName() string {
	defer perf.Track(nil, "flags.IntFlag.GetName")()

	return f.Name
}

// GetShorthand implements Flag.
func (f *IntFlag) GetShorthand() string {
	defer perf.Track(nil, "flags.IntFlag.GetShorthand")()

	return f.Shorthand
}

// GetDescription implements Flag.
func (f *IntFlag) GetDescription() string {
	defer perf.Track(nil, "flags.IntFlag.GetDescription")()

	return f.Description
}

// GetDefault implements Flag.
func (f *IntFlag) GetDefault() interface{} {
	defer perf.Track(nil, "flags.IntFlag.GetDefault")()

	return f.Default
}

// IsRequired implements Flag.
func (f *IntFlag) IsRequired() bool {
	defer perf.Track(nil, "flags.IntFlag.IsRequired")()

	return f.Required
}

// GetNoOptDefVal implements Flag.
func (f *IntFlag) GetNoOptDefVal() string {
	defer perf.Track(nil, "flags.IntFlag.GetNoOptDefVal")()

	return "" // Int flags don't use NoOptDefVal
}

// GetEnvVars implements Flag.
func (f *IntFlag) GetEnvVars() []string {
	defer perf.Track(nil, "flags.IntFlag.GetEnvVars")()

	return f.EnvVars
}

// GetCompletionFunc implements Flag.
func (f *IntFlag) GetCompletionFunc() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "flags.IntFlag.GetCompletionFunc")()

	return nil // Int flags don't use custom completion.
}

// StringSliceFlag represents a string slice flag.
// This allows a flag to be provided multiple times to build a list:
//
//	--config file1.yaml --config file2.yaml
//
// or with comma-separated values:
//
//	--config file1.yaml,file2.yaml
type StringSliceFlag struct {
	Name        string
	Shorthand   string
	Default     []string
	Description string
	Required    bool
	EnvVars     []string // Environment variables to bind.
}

// GetName implements Flag.
func (f *StringSliceFlag) GetName() string {
	defer perf.Track(nil, "flags.StringSliceFlag.GetName")()

	return f.Name
}

// GetShorthand implements Flag.
func (f *StringSliceFlag) GetShorthand() string {
	defer perf.Track(nil, "flags.StringSliceFlag.GetShorthand")()

	return f.Shorthand
}

// GetDescription implements Flag.
func (f *StringSliceFlag) GetDescription() string {
	defer perf.Track(nil, "flags.StringSliceFlag.GetDescription")()

	return f.Description
}

// GetDefault implements Flag.
func (f *StringSliceFlag) GetDefault() interface{} {
	defer perf.Track(nil, "flags.StringSliceFlag.GetDefault")()

	return f.Default
}

// IsRequired implements Flag.
func (f *StringSliceFlag) IsRequired() bool {
	defer perf.Track(nil, "flags.StringSliceFlag.IsRequired")()

	return f.Required
}

// GetNoOptDefVal implements Flag.
func (f *StringSliceFlag) GetNoOptDefVal() string {
	defer perf.Track(nil, "flags.StringSliceFlag.GetNoOptDefVal")()

	return "" // StringSlice flags don't use NoOptDefVal.
}

// GetEnvVars implements Flag.
func (f *StringSliceFlag) GetEnvVars() []string {
	defer perf.Track(nil, "flags.StringSliceFlag.GetEnvVars")()

	return f.EnvVars
}

// GetCompletionFunc implements Flag.
func (f *StringSliceFlag) GetCompletionFunc() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "flags.StringSliceFlag.GetCompletionFunc")()

	return nil // StringSlice flags don't use custom completion.
}

// positionalArgsConfig stores positional argument configuration.
// Used by both StandardOptionsBuilder and StandardFlagParser.
type positionalArgsConfig struct {
	specs     []*PositionalArgSpec
	validator cobra.PositionalArgs
	usage     string
}
