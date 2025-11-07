package helmfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyFlags(t *testing.T) {
	registry := ApplyFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "ApplyFlags should return non-nil registry")
}

func TestApplyPositionalArgs(t *testing.T) {
	builder := ApplyPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "ApplyPositionalArgs should return non-nil builder")

	// Verify that component is required
	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "ApplyPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "ApplyPositionalArgs usage should be <component>")
}

func TestDestroyFlags(t *testing.T) {
	registry := DestroyFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "DestroyFlags should return non-nil registry")
}

func TestDestroyPositionalArgs(t *testing.T) {
	builder := DestroyPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "DestroyPositionalArgs should return non-nil builder")

	// Verify that component is required
	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "DestroyPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "DestroyPositionalArgs usage should be <component>")
}

func TestDiffFlags(t *testing.T) {
	registry := DiffFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "DiffFlags should return non-nil registry")
}

func TestDiffPositionalArgs(t *testing.T) {
	builder := DiffPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "DiffPositionalArgs should return non-nil builder")

	// Verify that component is required
	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "DiffPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "DiffPositionalArgs usage should be <component>")
}

func TestSyncFlags(t *testing.T) {
	registry := SyncFlags()

	require.NotNil(t, registry)
	assert.NotNil(t, registry, "SyncFlags should return non-nil registry")
}

func TestSyncPositionalArgs(t *testing.T) {
	builder := SyncPositionalArgs()

	require.NotNil(t, builder)
	assert.NotNil(t, builder, "SyncPositionalArgs should return non-nil builder")

	// Verify that component is required
	specs, validator, usage := builder.Build()
	require.NotNil(t, specs)
	require.NotNil(t, validator)
	assert.Len(t, specs, 1, "SyncPositionalArgs should have 1 spec (component)")
	assert.Equal(t, "<component>", usage, "SyncPositionalArgs usage should be <component>")
}
