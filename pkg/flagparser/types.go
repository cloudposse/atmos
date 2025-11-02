package flagparser

import (
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
}

// StringFlag represents a string-valued flag.
type StringFlag struct {
	Name        string
	Shorthand   string
	Default     string
	Description string
	Required    bool
	NoOptDefVal string   // Value when flag used without argument (identity pattern)
	EnvVars     []string // Environment variables to bind
}

// GetName implements Flag.
func (f *StringFlag) GetName() string {
	defer perf.Track(nil, "flagparser.StringFlag.GetName")()

	return f.Name
}

// GetShorthand implements Flag.
func (f *StringFlag) GetShorthand() string {
	defer perf.Track(nil, "flagparser.StringFlag.GetShorthand")()

	return f.Shorthand
}

// GetDescription implements Flag.
func (f *StringFlag) GetDescription() string {
	defer perf.Track(nil, "flagparser.StringFlag.GetDescription")()

	return f.Description
}

// GetDefault implements Flag.
func (f *StringFlag) GetDefault() interface{} {
	defer perf.Track(nil, "flagparser.StringFlag.GetDefault")()

	return f.Default
}

// IsRequired implements Flag.
func (f *StringFlag) IsRequired() bool {
	defer perf.Track(nil, "flagparser.StringFlag.IsRequired")()

	return f.Required
}

// GetNoOptDefVal implements Flag.
func (f *StringFlag) GetNoOptDefVal() string {
	defer perf.Track(nil, "flagparser.StringFlag.GetNoOptDefVal")()

	return f.NoOptDefVal
}

// GetEnvVars implements Flag.
func (f *StringFlag) GetEnvVars() []string {
	defer perf.Track(nil, "flagparser.StringFlag.GetEnvVars")()

	return f.EnvVars
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
	defer perf.Track(nil, "flagparser.BoolFlag.GetName")()

	return f.Name
}

// GetShorthand implements Flag.
func (f *BoolFlag) GetShorthand() string {
	defer perf.Track(nil, "flagparser.BoolFlag.GetShorthand")()

	return f.Shorthand
}

// GetDescription implements Flag.
func (f *BoolFlag) GetDescription() string {
	defer perf.Track(nil, "flagparser.BoolFlag.GetDescription")()

	return f.Description
}

// GetDefault implements Flag.
func (f *BoolFlag) GetDefault() interface{} {
	defer perf.Track(nil, "flagparser.BoolFlag.GetDefault")()

	return f.Default
}

// IsRequired implements Flag.
func (f *BoolFlag) IsRequired() bool {
	defer perf.Track(nil, "flagparser.BoolFlag.IsRequired")()

	return false // Bool flags are never required
}

// GetNoOptDefVal implements Flag.
func (f *BoolFlag) GetNoOptDefVal() string {
	defer perf.Track(nil, "flagparser.BoolFlag.GetNoOptDefVal")()

	return "" // Bool flags don't use NoOptDefVal
}

// GetEnvVars implements Flag.
func (f *BoolFlag) GetEnvVars() []string {
	defer perf.Track(nil, "flagparser.BoolFlag.GetEnvVars")()

	return f.EnvVars
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
	defer perf.Track(nil, "flagparser.IntFlag.GetName")()

	return f.Name
}

// GetShorthand implements Flag.
func (f *IntFlag) GetShorthand() string {
	defer perf.Track(nil, "flagparser.IntFlag.GetShorthand")()

	return f.Shorthand
}

// GetDescription implements Flag.
func (f *IntFlag) GetDescription() string {
	defer perf.Track(nil, "flagparser.IntFlag.GetDescription")()

	return f.Description
}

// GetDefault implements Flag.
func (f *IntFlag) GetDefault() interface{} {
	defer perf.Track(nil, "flagparser.IntFlag.GetDefault")()

	return f.Default
}

// IsRequired implements Flag.
func (f *IntFlag) IsRequired() bool {
	defer perf.Track(nil, "flagparser.IntFlag.IsRequired")()

	return f.Required
}

// GetNoOptDefVal implements Flag.
func (f *IntFlag) GetNoOptDefVal() string {
	defer perf.Track(nil, "flagparser.IntFlag.GetNoOptDefVal")()

	return "" // Int flags don't use NoOptDefVal
}

// GetEnvVars implements Flag.
func (f *IntFlag) GetEnvVars() []string {
	defer perf.Track(nil, "flagparser.IntFlag.GetEnvVars")()

	return f.EnvVars
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
	defer perf.Track(nil, "flagparser.StringSliceFlag.GetName")()

	return f.Name
}

// GetShorthand implements Flag.
func (f *StringSliceFlag) GetShorthand() string {
	defer perf.Track(nil, "flagparser.StringSliceFlag.GetShorthand")()

	return f.Shorthand
}

// GetDescription implements Flag.
func (f *StringSliceFlag) GetDescription() string {
	defer perf.Track(nil, "flagparser.StringSliceFlag.GetDescription")()

	return f.Description
}

// GetDefault implements Flag.
func (f *StringSliceFlag) GetDefault() interface{} {
	defer perf.Track(nil, "flagparser.StringSliceFlag.GetDefault")()

	return f.Default
}

// IsRequired implements Flag.
func (f *StringSliceFlag) IsRequired() bool {
	defer perf.Track(nil, "flagparser.StringSliceFlag.IsRequired")()

	return f.Required
}

// GetNoOptDefVal implements Flag.
func (f *StringSliceFlag) GetNoOptDefVal() string {
	defer perf.Track(nil, "flagparser.StringSliceFlag.GetNoOptDefVal")()

	return "" // StringSlice flags don't use NoOptDefVal.
}

// GetEnvVars implements Flag.
func (f *StringSliceFlag) GetEnvVars() []string {
	defer perf.Track(nil, "flagparser.StringSliceFlag.GetEnvVars")()

	return f.EnvVars
}
