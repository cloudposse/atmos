//go:build test

package telemetry

import (
	"os"
	"testing"
)

// DisableCIEnvVars unsets CI-related environment variables for the duration
// of the test. It registers cleanup functions via t.Setenv so that all
// variables are restored to their original values when the test ends.
func DisableCIEnvVars(t testing.TB) {
	t.Helper()

	unset := func(key string) {
		if val, ok := os.LookupEnv(key); ok {
			t.Setenv(key, val)
		} else {
			t.Setenv(key, "")
		}
		os.Unsetenv(key)
	}

	for key := range ciProvidersEnvVarsExists {
		unset(key)
	}
	for _, values := range ciProvidersEnvVarsEquals {
		for key := range values {
			unset(key)
		}
	}
	unset(ciEnvVar)
}
