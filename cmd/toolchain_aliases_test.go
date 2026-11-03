package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

func TestToolchainAliasesCommand(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		env     string
		wantErr error
	}{
		{name: "format flag", args: []string{"--format", "json"}},
		{name: "format environment", env: "json"},
		{name: "flag overrides environment", args: []string{"-f", "json"}, env: "yaml"},
		{name: "invalid format", args: []string{"--format", "xml"}, wantErr: errUtils.ErrInvalidFlagValue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			viper.Reset()
			t.Cleanup(viper.Reset)
			previous := toolchain.GetAtmosConfig()
			t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
			clearIdentityEnvVars(t)
			root := t.TempDir()
			t.Chdir(root)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			t.Setenv("ATMOS_BASE_PATH", root)
			t.Setenv("ATMOS_TOOLCHAIN_ALIASES_FORMAT", tc.env)
			config := "import:\n  - aliases.yaml\ntoolchain:\n  aliases:\n    tofu: custom/opentofu\n  registries:\n    - name: unreachable\n      type: aqua\n      source: https://registry.invalid/never-fetch.yaml\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(config), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "aliases.yaml"), []byte("toolchain:\n  aliases:\n    tofu: imported/opentofu\n    inherited: example/imported\n"), 0o600))
			setComponentCommandArgs(t, append([]string{"toolchain", "aliases"}, tc.args...))
			var commandErr error
			stdout, _ := captureStdoutStderr(t, func() { commandErr = Execute() })
			if tc.wantErr != nil {
				require.ErrorIs(t, commandErr, tc.wantErr)
				return
			}
			require.NoError(t, commandErr)
			var actual map[string]string
			require.NoError(t, json.Unmarshal([]byte(stdout), &actual), stdout)
			require.Equal(t, "custom/opentofu", actual["tofu"])
			require.Equal(t, "example/imported", actual["inherited"])
			require.Equal(t, "cloudposse/atmos", actual["atmos"])
			require.NoFileExists(t, filepath.Join(root, ".tool-versions"))
			require.NoDirExists(t, filepath.Join(root, ".tools"))
			contents, err := os.ReadFile(filepath.Join(root, "atmos.yaml"))
			require.NoError(t, err)
			require.Equal(t, config, string(contents))
		})
	}
}

func TestToolchainAliasesRejectsArguments(t *testing.T) {
	_ = NewTestKit(t)
	command, remaining, err := RootCmd.Find([]string{"toolchain", "aliases", "unexpected"})
	require.NoError(t, err)
	require.Equal(t, "aliases", command.Name())
	require.Error(t, command.ValidateArgs(remaining))
}

func TestToolchainAliasesFormatCompletion(t *testing.T) {
	_ = NewTestKit(t)
	command, _, err := RootCmd.Find([]string{"toolchain", "aliases"})
	require.NoError(t, err)
	complete, ok := command.GetFlagCompletionFunc("format")
	require.True(t, ok)
	values, directive := complete(command, nil, "")
	require.Equal(t, []string{"table", "json", "yaml"}, values)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}
