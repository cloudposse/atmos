package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

func TestRemoteConfigExecProvenanceBlocksSideEffect(t *testing.T) {
	guard.ResetOrigins()
	t.Cleanup(guard.ResetOrigins)
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: provenance}
spec:
  policies:
    remote-exec:
      on: function
      action: deny
      rule: "input.name == 'exec' && input.provenance != 'local'"
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	configFile := filepath.Join(t.TempDir(), "remote.yaml")
	sentinel := filepath.Join(t.TempDir(), "side-effect")
	content := []byte("base_path: !exec touch " + sentinel + "\n")
	require.NoError(t, os.WriteFile(configFile, content, 0o600))
	guard.RegisterRemote(configFile, "https://example.com/remote.yaml")
	err := preprocessAtmosYamlFuncExceptCommands(content, viper.New(), configFile)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	_, err = os.Stat(sentinel)
	require.True(t, os.IsNotExist(err))
}

func TestEarlyDotenvMergeIncludeConfinement(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: include}
spec:
  policies:
    deny:
      on: source
      action: deny
      rule: "input.operation == 'include'"
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	file := filepath.Join(t.TempDir(), "secret.env")
	require.NoError(t, os.WriteFile(file, []byte("SECRET=hidden\n"), 0o600))
	_, err := loadDotenvIncludeAsYAMLNode("atmos.yaml", file)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
}
