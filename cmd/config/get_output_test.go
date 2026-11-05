package config

import (
	"os"
	"path/filepath"
	"strings"
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

// --format follows the standard precedence: the flag, then ATMOS_CONFIG_GET_FORMAT, then raw.
func TestConfigGetFormatPrecedence(t *testing.T) {
	file := filepath.Join(t.TempDir(), "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("base_path: \"false\"\nlogs:\n  level: Info\n"), 0o600))
	stdout := initConfigTestWriter(t)
	originalArgs := os.Args
	reset := func() {
		viper.Reset()
		require.NoError(t, configGetCmd.Flags().Set("format", "raw"))
		configGetCmd.Flags().Lookup("format").Changed = false
	}
	t.Cleanup(func() {
		os.Args = originalArgs
		reset()
	})
	os.Args = []string{"atmos", "--config", file, "config", "get", "base_path"}
	get := func() string {
		stdout.Reset()
		require.NoError(t, configGetCmd.RunE(configGetCmd, []string{"base_path"}))
		return strings.TrimSpace(stdout.String())
	}

	reset()
	assert.Equal(t, "false", get(), "the default is raw")

	t.Run("the environment variable selects the format", func(t *testing.T) {
		reset()
		t.Setenv("ATMOS_CONFIG_GET_FORMAT", "json")
		assert.Equal(t, `"false"`, get())
	})
	t.Run("the flag beats the environment variable", func(t *testing.T) {
		reset()
		t.Setenv("ATMOS_CONFIG_GET_FORMAT", "json")
		require.NoError(t, configGetCmd.Flags().Set("format", "raw"))
		assert.Equal(t, "false", get())
	})
	t.Run("a generic ATMOS_FORMAT does not change the output", func(t *testing.T) {
		reset()
		t.Setenv("ATMOS_FORMAT", "yaml")
		assert.Equal(t, "false", get())
	})
	t.Run("the shorthand is registered", func(t *testing.T) {
		flag := configGetCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "f", flag.Shorthand)
	})
}
