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

func TestWithTerraformFlags(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	opt := WithTerraformFlags()
	opt(cfg)

	// Should have 5 flags: stack, dry-run, upload-status, skip-init, from-plan
	// Global flags (identity, etc.) are inherited from RootCmd, not in registry
	assert.True(t, cfg.registry.Count() >= 5)
	assert.NotNil(t, cfg.registry.Get("upload-status"))
	assert.NotNil(t, cfg.registry.Get("stack"))
	assert.Nil(t, cfg.registry.Get("identity"), "identity should be inherited from RootCmd")
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
