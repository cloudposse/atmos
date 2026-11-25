package env

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

var errTestSetenv = errors.New("setenv rejected")

// TestPromoteAtmosEnvWith_SetFailureSkipsOnlyThatKey verifies that a variable the
// environment refuses to set is not reported as promoted, and that the failure does
// not stop the remaining ATMOS_* variables from being promoted.
func TestPromoteAtmosEnvWith_SetFailureSkipsOnlyThatKey(t *testing.T) {
	envMap := map[string]string{
		"ATMOS_LOGS_LEVEL": "Debug",
		"ATMOS_BAD":        "rejected",
		"ATMOS_PROFILE":    "dev",
	}
	set := map[string]string{}
	lookup := func(string) (string, bool) { return "", false }
	setter := func(key, value string) error {
		if key == "ATMOS_BAD" {
			return errTestSetenv
		}
		set[key] = value
		return nil
	}

	promoted := PromoteAtmosEnvWith(envMap, lookup, setter)
	sort.Strings(promoted)

	assert.Equal(t, []string{"ATMOS_LOGS_LEVEL", "ATMOS_PROFILE"}, promoted)
	assert.Equal(t, map[string]string{"ATMOS_LOGS_LEVEL": "Debug", "ATMOS_PROFILE": "dev"}, set)
}
