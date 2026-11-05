package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadScaffoldTemplatesConfigErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		wantErr error
	}{
		{name: "unrelated typed configuration", content: "components: invalid\n"},
		{name: "malformed YAML", content: "scaffold: [\n", wantErr: errUtils.ErrReadScaffoldConfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(tc.content), 0o600))
			_, err := cfg.LoadConfig(&schema.ConfigAndStacksInfo{})
			require.Error(t, err, "the full configuration loader must reproduce the failure")

			configs, _, _, ui, err := loadScaffoldTemplates("", "")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, configs)
			require.NotNil(t, ui)
		})
	}
}
