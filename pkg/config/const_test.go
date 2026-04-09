package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAtmosEnvVarPrefixMatchesNamespace is a build-time invariant: the
// AtmosEnvVarPrefix constant MUST be the AtmosEnvVarNamespace followed by an
// underscore. If a future change accidentally drifts these two apart, this
// test fails immediately and prevents shipping a broken Viper env namespace.
func TestAtmosEnvVarPrefixMatchesNamespace(t *testing.T) {
	assert.Equal(t, AtmosEnvVarNamespace+"_", AtmosEnvVarPrefix,
		"AtmosEnvVarPrefix must equal AtmosEnvVarNamespace + '_'")
}

func TestAtmosEnvVarConstants_Values(t *testing.T) {
	// Lock in the literal values to catch accidental renames. The Viper
	// namespace and prefix are part of the public Atmos contract — changing
	// them is a breaking change for users with ATMOS_* env vars in their
	// environment.
	assert.Equal(t, "ATMOS", AtmosEnvVarNamespace)
	assert.Equal(t, "ATMOS_", AtmosEnvVarPrefix)
}
