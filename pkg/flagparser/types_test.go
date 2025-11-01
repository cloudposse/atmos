package flagparser

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestStringFlag(t *testing.T) {
	flag := &StringFlag{
		Name:        "test",
		Shorthand:   "t",
		Default:     "default",
		Description: "Test flag",
		Required:    true,
		NoOptDefVal: "__SELECT__",
		EnvVars:     []string{"TEST_VAR"},
	}

	assert.Equal(t, "test", flag.GetName())
	assert.Equal(t, "t", flag.GetShorthand())
	assert.Equal(t, "default", flag.GetDefault())
	assert.Equal(t, "Test flag", flag.GetDescription())
	assert.True(t, flag.IsRequired())
	assert.Equal(t, "__SELECT__", flag.GetNoOptDefVal())
	assert.Equal(t, []string{"TEST_VAR"}, flag.GetEnvVars())
}

func TestBoolFlag(t *testing.T) {
	flag := &BoolFlag{
		Name:        "verbose",
		Shorthand:   "v",
		Default:     false,
		Description: "Verbose output",
		EnvVars:     []string{"VERBOSE"},
	}

	assert.Equal(t, "verbose", flag.GetName())
	assert.Equal(t, "v", flag.GetShorthand())
	assert.Equal(t, false, flag.GetDefault())
	assert.Equal(t, "Verbose output", flag.GetDescription())
	assert.False(t, flag.IsRequired()) // Bool flags are never required
	assert.Equal(t, "", flag.GetNoOptDefVal())
	assert.Equal(t, []string{"VERBOSE"}, flag.GetEnvVars())
}

func TestIntFlag(t *testing.T) {
	flag := &IntFlag{
		Name:        "count",
		Shorthand:   "c",
		Default:     10,
		Description: "Count",
		Required:    true,
		EnvVars:     []string{"COUNT"},
	}

	assert.Equal(t, "count", flag.GetName())
	assert.Equal(t, "c", flag.GetShorthand())
	assert.Equal(t, 10, flag.GetDefault())
	assert.Equal(t, "Count", flag.GetDescription())
	assert.True(t, flag.IsRequired())
	assert.Equal(t, "", flag.GetNoOptDefVal())
	assert.Equal(t, []string{"COUNT"}, flag.GetEnvVars())
}

func TestIdentityFlag(t *testing.T) {
	// Identity flag is a special string flag with NoOptDefVal
	flag := &StringFlag{
		Name:        cfg.IdentityFlagName,
		Shorthand:   "i",
		Default:     "",
		Description: "Identity to use",
		Required:    false,
		NoOptDefVal: cfg.IdentityFlagSelectValue,
		EnvVars:     []string{"ATMOS_IDENTITY", "IDENTITY"},
	}

	assert.Equal(t, "identity", flag.GetName())
	assert.Equal(t, "i", flag.GetShorthand())
	assert.Equal(t, "", flag.GetDefault())
	assert.False(t, flag.IsRequired())
	assert.Equal(t, "__SELECT__", flag.GetNoOptDefVal())
	assert.Equal(t, []string{"ATMOS_IDENTITY", "IDENTITY"}, flag.GetEnvVars())
}
