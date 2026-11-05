package config

import (
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestConfigFunctionAdmission(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: input.name == 'exec'\n"), 0o600))
	for _, data := range []string{"key: !exec should-never-run", "items: [!exec should-never-run]", "nested:\n  key: !exec should-never-run"} {
		err := preprocessAtmosYamlFunc([]byte(data), viper.New())
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	}
}
