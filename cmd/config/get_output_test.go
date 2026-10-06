package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigGetJSON(t *testing.T) {
	file := filepath.Join(t.TempDir(), "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("base_path: \"false\"\nlogs:\n  level: Info\nstacks:\n  included_paths: [one, two]\n"), 0o600))
	stdout := initConfigTestWriter(t)
	viper.Reset()
	originalArgs := os.Args
	t.Cleanup(func() {
		os.Args = originalArgs
		viper.Reset()
		require.NoError(t, configGetCmd.Flags().Set("format", "raw"))
		configGetCmd.Flags().Lookup("format").Changed = false
	})
	os.Args = []string{"atmos", "--config", file, "config", "get", "base_path"}
	require.NoError(t, configGetCmd.Flags().Set("format", "json"))
	for _, tc := range []struct{ path, expected string }{
		{"base_path", `"false"`}, {"stacks.included_paths", `["one","two"]`},
	} {
		stdout.Reset()
		require.NoError(t, configGetCmd.RunE(configGetCmd, []string{tc.path}))
		assert.JSONEq(t, tc.expected, stdout.String())
	}
}
