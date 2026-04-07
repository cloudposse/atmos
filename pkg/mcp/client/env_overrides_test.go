package client

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsetForTest unsets the given env vars and registers cleanup that restores
// them to their original state. Use when t.Setenv isn't appropriate (e.g., when
// the test asserts the variable is *unset* after some operation).
func unsetForTest(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		old, had := os.LookupEnv(k)
		require.NoError(t, os.Unsetenv(k))
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
}

func TestApplyAtmosEnvOverrides_AppliesAndRestores(t *testing.T) {
	unsetForTest(t, "ATMOS_PROFILE", "ATMOS_BASE_PATH")
	t.Setenv("FOO_NOT_ATMOS", "untouched")

	restore := ApplyAtmosEnvOverrides(map[string]string{
		"ATMOS_PROFILE":   "managers",
		"ATMOS_BASE_PATH": "/tmp/base",
		"FOO_NOT_ATMOS":   "should-be-ignored",
	})

	// ATMOS_* values should be applied.
	assert.Equal(t, "managers", os.Getenv("ATMOS_PROFILE"))
	assert.Equal(t, "/tmp/base", os.Getenv("ATMOS_BASE_PATH"))
	// Non-ATMOS_ entries should NOT be touched.
	assert.Equal(t, "untouched", os.Getenv("FOO_NOT_ATMOS"))

	restore()

	// After restore, ATMOS_* should be unset (they had no prior value).
	_, hadProfile := os.LookupEnv("ATMOS_PROFILE")
	_, hadBase := os.LookupEnv("ATMOS_BASE_PATH")
	assert.False(t, hadProfile, "ATMOS_PROFILE should be unset after restore")
	assert.False(t, hadBase, "ATMOS_BASE_PATH should be unset after restore")
	// FOO_NOT_ATMOS untouched.
	assert.Equal(t, "untouched", os.Getenv("FOO_NOT_ATMOS"))
}

func TestApplyAtmosEnvOverrides_RestoresPreviousValues(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "original")

	restore := ApplyAtmosEnvOverrides(map[string]string{
		"ATMOS_PROFILE": "overridden",
	})
	assert.Equal(t, "overridden", os.Getenv("ATMOS_PROFILE"))

	restore()
	assert.Equal(t, "original", os.Getenv("ATMOS_PROFILE"))
}

func TestApplyAtmosEnvOverrides_EmptyMap_NoOp(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "stable")

	restore := ApplyAtmosEnvOverrides(nil)
	assert.Equal(t, "stable", os.Getenv("ATMOS_PROFILE"))
	restore()
	assert.Equal(t, "stable", os.Getenv("ATMOS_PROFILE"))

	restore2 := ApplyAtmosEnvOverrides(map[string]string{})
	assert.Equal(t, "stable", os.Getenv("ATMOS_PROFILE"))
	restore2()
	assert.Equal(t, "stable", os.Getenv("ATMOS_PROFILE"))
}

func TestApplyAtmosEnvOverrides_NoAtmosKeys_NoOp(t *testing.T) {
	unsetForTest(t, "FOO", "BAR")
	t.Setenv("ATMOS_PROFILE", "stable")

	restore := ApplyAtmosEnvOverrides(map[string]string{
		"FOO": "x",
		"BAR": "y",
	})
	defer restore()

	// ATMOS_PROFILE untouched, FOO/BAR not set by overrides.
	assert.Equal(t, "stable", os.Getenv("ATMOS_PROFILE"))
	_, hadFoo := os.LookupEnv("FOO")
	_, hadBar := os.LookupEnv("BAR")
	assert.False(t, hadFoo)
	assert.False(t, hadBar)
}

func TestApplyAtmosEnvOverrides_RestoreIsIdempotent(t *testing.T) {
	unsetForTest(t, "ATMOS_PROFILE")

	restore := ApplyAtmosEnvOverrides(map[string]string{
		"ATMOS_PROFILE": "managers",
	})
	assert.Equal(t, "managers", os.Getenv("ATMOS_PROFILE"))

	restore()
	_, had := os.LookupEnv("ATMOS_PROFILE")
	assert.False(t, had)

	// Calling restore again must not blow up or re-mutate state.
	t.Setenv("ATMOS_PROFILE", "set-after-restore")
	restore()
	assert.Equal(t, "set-after-restore", os.Getenv("ATMOS_PROFILE"))
}

func TestApplyAtmosEnvOverrides_AppliesAllAtmosPrefixedKeys(t *testing.T) {
	keys := []string{"ATMOS_PROFILE", "ATMOS_CLI_CONFIG_PATH", "ATMOS_BASE_PATH", "ATMOS_LOGS_LEVEL"}
	unsetForTest(t, keys...)

	restore := ApplyAtmosEnvOverrides(map[string]string{
		"ATMOS_PROFILE":         "managers",
		"ATMOS_CLI_CONFIG_PATH": "/etc/atmos",
		"ATMOS_BASE_PATH":       "/srv/repo",
		"ATMOS_LOGS_LEVEL":      "Debug",
	})
	defer restore()

	for k, v := range map[string]string{
		"ATMOS_PROFILE":         "managers",
		"ATMOS_CLI_CONFIG_PATH": "/etc/atmos",
		"ATMOS_BASE_PATH":       "/srv/repo",
		"ATMOS_LOGS_LEVEL":      "Debug",
	} {
		assert.Equal(t, v, os.Getenv(k), "key %s", k)
	}
}
