package exec

import (
	"os"
	"strings"
	"testing"
)

func TestMergeEnvVars(t *testing.T) {
	// Save the original environment and restore at the end
	originalEnv := os.Environ()

	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	// Set the initial system environment
	os.Clearenv()
	os.Setenv("PATH", "/usr/bin")
	os.Setenv("TF_CLI_ARGS_plan", "-lock=false")
	os.Setenv("HOME", "/home/test")

	// Atmos environment variables to merge
	componentEnv := []string{
		"TF_CLI_ARGS_plan=-compact-warnings",
		"ATMOS_VAR=value",
		"HOME=/overridden/home",
		"NEW_VAR=newvalue",
	}

	merged := mergeEnvVars(componentEnv)

	// Convert the merged list back to a map for easier assertions
	mergedMap := make(map[string]string)
	for _, env := range merged {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			mergedMap[parts[0]] = parts[1]
		}
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"PATH", "/usr/bin"}, // should be preserved
		{"TF_CLI_ARGS_plan", "-compact-warnings -lock=false"}, // prepended
		{"ATMOS_VAR", "value"},                                // new variable
		{"HOME", "/overridden/home"},                          // overridden
		{"NEW_VAR", "newvalue"},                               // added
	}

	for _, test := range tests {
		if val, ok := mergedMap[test.key]; !ok {
			t.Errorf("Missing key %q in merged environment", test.key)
		} else if val != test.expected {
			t.Errorf("Incorrect value for %q: expected %q, got %q", test.key, test.expected, val)
		}
	}
}
