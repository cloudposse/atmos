package packer

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFlags(t *testing.T) {
	registry := BuildFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "BuildFlags should return non-nil registry")
}

func TestBuildPositionalArgs(t *testing.T) {
	builder := BuildPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "BuildPositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "BuildPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "BuildPositionalArgs usage should be <component>")
}

func TestInitFlags(t *testing.T) {
	registry := InitFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "InitFlags should return non-nil registry")
}

func TestInitPositionalArgs(t *testing.T) {
	builder := InitPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "InitPositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "InitPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "InitPositionalArgs usage should be <component>")
}

func TestValidateFlags(t *testing.T) {
	registry := ValidateFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "ValidateFlags should return non-nil registry")
}

func TestValidatePositionalArgs(t *testing.T) {
	builder := ValidatePositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "ValidatePositionalArgs should return non-nil builder")

	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "ValidatePositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "ValidatePositionalArgs usage should be <component>")
}

func TestParser_RegisterPersistentFlags(t *testing.T) {
	parser := NewParser()
	cmd := &cobra.Command{Use: "packer"}

	parser.RegisterPersistentFlags(cmd)

	assert.NotNil(t, parser.cmd, "Parser should have cmd reference after RegisterPersistentFlags")

	// Verify persistent flags were registered
	assert.NotNil(t, cmd.PersistentFlags().Lookup("stack"), "stack flag should be registered as persistent")
}
