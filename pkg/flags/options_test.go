package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithStringFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithStringFlag("format", "f", "yaml", "Output format")
	opt(cfg)

	flag := cfg.registry.Get("format")
	assert.NotNil(t, flag)

	strFlag, ok := flag.(*StringFlag)
	assert.True(t, ok)
	assert.Equal(t, "format", strFlag.Name)
	assert.Equal(t, "f", strFlag.Shorthand)
	assert.Equal(t, "yaml", strFlag.Default)
}

func TestWithBoolFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithBoolFlag("verbose", "v", false, "Verbose output")
	opt(cfg)

	flag := cfg.registry.Get("verbose")
	assert.NotNil(t, flag)

	boolFlag, ok := flag.(*BoolFlag)
	assert.True(t, ok)
	assert.Equal(t, "verbose", boolFlag.Name)
	assert.Equal(t, "v", boolFlag.Shorthand)
	assert.Equal(t, false, boolFlag.Default)
}

func TestWithIntFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithIntFlag("timeout", "t", 30, "Timeout in seconds")
	opt(cfg)

	flag := cfg.registry.Get("timeout")
	assert.NotNil(t, flag)

	intFlag, ok := flag.(*IntFlag)
	assert.True(t, ok)
	assert.Equal(t, "timeout", intFlag.Name)
	assert.Equal(t, "t", intFlag.Shorthand)
	assert.Equal(t, 30, intFlag.Default)
}

func TestWithStackFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithStackFlag()
	opt(cfg)

	flag := cfg.registry.Get("stack")
	assert.NotNil(t, flag)
	assert.Equal(t, "s", flag.GetShorthand())
}

func TestWithDryRunFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithDryRunFlag()
	opt(cfg)

	flag := cfg.registry.Get("dry-run")
	assert.NotNil(t, flag)
}

func TestWithHelmfileFlags(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithHelmfileFlags()
	opt(cfg)

	// Should have 2 flags: stack, dry-run
	// Global flags are inherited from RootCmd, not in registry
	assert.Equal(t, 2, cfg.registry.Count())
	assert.NotNil(t, cfg.registry.Get("stack"))
	assert.NotNil(t, cfg.registry.Get("dry-run"))
	assert.Nil(t, cfg.registry.Get("identity"), "identity should be inherited from RootCmd")
}

func TestWithPackerFlags(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithPackerFlags()
	opt(cfg)

	// Should have 2 flags: stack, dry-run
	// Global flags are inherited from RootCmd, not in registry
	assert.Equal(t, 2, cfg.registry.Count())
	assert.NotNil(t, cfg.registry.Get("stack"))
	assert.NotNil(t, cfg.registry.Get("dry-run"))
	assert.Nil(t, cfg.registry.Get("identity"), "identity should be inherited from RootCmd")
}

func TestWithEnvVars(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// First add a flag
	WithStringFlag("format", "f", "yaml", "Output format")(cfg)

	// Then add env vars
	opt := WithEnvVars("format", "ATMOS_FORMAT", "FORMAT")
	opt(cfg)

	flag := cfg.registry.Get("format")
	assert.NotNil(t, flag)

	strFlag, ok := flag.(*StringFlag)
	assert.True(t, ok)
	assert.Equal(t, []string{"ATMOS_FORMAT", "FORMAT"}, strFlag.EnvVars)
}

func TestWithNoOptDefVal(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// First add a string flag
	WithStringFlag("identity", "i", "", "Identity name")(cfg)

	// Then set NoOptDefVal
	opt := WithNoOptDefVal("identity", "__SELECT__")
	opt(cfg)

	flag := cfg.registry.Get("identity")
	assert.NotNil(t, flag)

	strFlag, ok := flag.(*StringFlag)
	assert.True(t, ok)
	assert.Equal(t, "__SELECT__", strFlag.NoOptDefVal)
}

func TestWithRegistry(t *testing.T) {
	// Create a custom registry
	customRegistry := NewFlagRegistry()
	customRegistry.Register(&StringFlag{Name: "custom", Shorthand: "c"})

	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithRegistry(customRegistry)
	opt(cfg)

	assert.Equal(t, customRegistry, cfg.registry)
	assert.NotNil(t, cfg.registry.Get("custom"))
}

func TestWithStringSliceFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithStringSliceFlag("components", "c", []string{"vpc", "eks"}, "Filter by components")
	opt(cfg)

	flag := cfg.registry.Get("components")
	assert.NotNil(t, flag)

	sliceFlag, ok := flag.(*StringSliceFlag)
	assert.True(t, ok)
	assert.Equal(t, "components", sliceFlag.Name)
	assert.Equal(t, "c", sliceFlag.Shorthand)
	assert.Equal(t, []string{"vpc", "eks"}, sliceFlag.Default)
	assert.Equal(t, "Filter by components", sliceFlag.Description)
}

func TestWithRequiredStringFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithRequiredStringFlag("stack", "s", "Stack name (required)")
	opt(cfg)

	flag := cfg.registry.Get("stack")
	assert.NotNil(t, flag)

	strFlag, ok := flag.(*StringFlag)
	assert.True(t, ok)
	assert.Equal(t, "stack", strFlag.Name)
	assert.Equal(t, "s", strFlag.Shorthand)
	assert.Equal(t, "", strFlag.Default, "required flags should have empty default")
	assert.True(t, strFlag.Required, "flag should be marked as required")
}

func TestWithIdentityFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithIdentityFlag()
	opt(cfg)

	flag := cfg.registry.Get("identity")
	// The identity flag is registered in GlobalFlagsRegistry.
	// If it exists there, it should be added to this registry.
	if GlobalFlagsRegistry().Get("identity") != nil {
		assert.NotNil(t, flag, "identity flag should be registered when available in global registry")
	}
}

func TestWithCommonFlags(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithCommonFlags()
	opt(cfg)

	// Should have common flags like stack and dry-run.
	assert.NotNil(t, cfg.registry.Get("stack"), "should have stack flag")
	assert.NotNil(t, cfg.registry.Get("dry-run"), "should have dry-run flag")

	// Verify no duplicate registration (identity may be in both global and common).
	count := cfg.registry.Count()
	assert.Greater(t, count, 0, "should have registered flags")
}

func TestWithViperPrefix(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithViperPrefix("terraform")
	opt(cfg)

	assert.Equal(t, "terraform", cfg.viperPrefix, "should set viper prefix")
}

func TestWithValidValues(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// First add a string flag.
	WithStringFlag("format", "f", "yaml", "Output format")(cfg)

	// Then set valid values.
	opt := WithValidValues("format", "json", "yaml", "table")
	opt(cfg)

	flag := cfg.registry.Get("format")
	assert.NotNil(t, flag)

	strFlag, ok := flag.(*StringFlag)
	assert.True(t, ok)
	assert.Equal(t, []string{"json", "yaml", "table"}, strFlag.ValidValues)
}

func TestWithValidValues_NonExistentFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// Try to set valid values for a flag that doesn't exist.
	opt := WithValidValues("nonexistent", "value1", "value2")
	opt(cfg)

	// Should not panic and flag should not exist.
	assert.Nil(t, cfg.registry.Get("nonexistent"))
}

func TestWithNoOptDefVal_NonExistentFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// Try to set NoOptDefVal for a flag that doesn't exist.
	opt := WithNoOptDefVal("nonexistent", "__SELECT__")
	opt(cfg)

	// Should not panic and flag should not exist.
	assert.Nil(t, cfg.registry.Get("nonexistent"))
}

func TestWithEnvVars_BoolFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// First add a bool flag.
	WithBoolFlag("verbose", "v", false, "Verbose output")(cfg)

	// Then add env vars.
	opt := WithEnvVars("verbose", "ATMOS_VERBOSE", "VERBOSE")
	opt(cfg)

	flag := cfg.registry.Get("verbose")
	assert.NotNil(t, flag)

	boolFlag, ok := flag.(*BoolFlag)
	assert.True(t, ok)
	assert.Equal(t, []string{"ATMOS_VERBOSE", "VERBOSE"}, boolFlag.EnvVars)
}

func TestWithEnvVars_IntFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// First add an int flag.
	WithIntFlag("timeout", "t", 30, "Timeout in seconds")(cfg)

	// Then add env vars.
	opt := WithEnvVars("timeout", "ATMOS_TIMEOUT", "TIMEOUT")
	opt(cfg)

	flag := cfg.registry.Get("timeout")
	assert.NotNil(t, flag)

	intFlag, ok := flag.(*IntFlag)
	assert.True(t, ok)
	assert.Equal(t, []string{"ATMOS_TIMEOUT", "TIMEOUT"}, intFlag.EnvVars)
}

func TestWithEnvVars_StringSliceFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// First add a string slice flag.
	WithStringSliceFlag("components", "", []string{}, "Components list")(cfg)

	// Then add env vars.
	opt := WithEnvVars("components", "ATMOS_COMPONENTS")
	opt(cfg)

	flag := cfg.registry.Get("components")
	assert.NotNil(t, flag)

	sliceFlag, ok := flag.(*StringSliceFlag)
	assert.True(t, ok)
	assert.Equal(t, []string{"ATMOS_COMPONENTS"}, sliceFlag.EnvVars)
}

func TestWithEnvVars_NonExistentFlag(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	// Try to add env vars to a flag that doesn't exist.
	opt := WithEnvVars("nonexistent", "ATMOS_NONEXISTENT")
	opt(cfg)

	// Should not panic and flag should not exist.
	assert.Nil(t, cfg.registry.Get("nonexistent"))
}
