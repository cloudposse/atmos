package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema field this test reads.
var _ = schema.CIConfig{AllowUnsafeForkExecution: true}

func TestAllowUnsafeForkExecutionEnvironmentBinding(t *testing.T) {
	tests := []struct {
		name string
		env  string
		file string
		want bool
	}{
		{name: "unset leaves the gate closed"},
		{name: "env opens the gate", env: "true", want: true},
		{name: "env false keeps it closed", env: "false"},
		{name: "env overrides the file", env: "false", file: "ci:\n  allow_unsafe_fork_execution: true\n"},
		{name: "file applies without env", file: "ci:\n  allow_unsafe_fork_execution: true\n", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("base_path: ./\n"+test.file), 0o644))
			t.Chdir(dir)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
			t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
			t.Setenv("ATMOS_ALLOW_UNSAFE_FORK_EXECUTION", test.env)
			if test.env == "" {
				require.NoError(t, os.Unsetenv("ATMOS_ALLOW_UNSAFE_FORK_EXECUTION"))
			}

			atmosConfig, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)

			require.NoError(t, err)
			assert.Equal(t, test.want, atmosConfig.CI.AllowUnsafeForkExecution)
		})
	}
}
