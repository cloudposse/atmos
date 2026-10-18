package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/typedetect"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDirectoryPresenceProbe(t *testing.T) {
	tests := []struct {
		name      string
		terraform string
		wantError error
	}{
		{name: "invalid flags", terraform: "    app:\n      flags: {lock: not-a-bool}\n", wantError: schema.ErrInvalidTerraformFlagsConfig},
		{name: "invalid retry", terraform: "    app:\n      retry: {initial_delay: not-a-duration}\n", wantError: schema.ErrInvalidRetryConfig},
		{name: "duplicate definition", terraform: "    app:\n      vars: {}\n", wantError: errUtils.ErrDuplicateComponentConfig},
		{name: "missing component", terraform: "    other:\n      vars: {}\n"},
		{name: "missing type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks", "deploy"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`
base_path: "."
stacks:
  base_path: stacks
  included_paths: ["**/*"]
components:
  terraform:
    base_path: components/terraform
  helmfile:
    base_path: components/helmfile
`), 0o644))
			manifest := "components:\n  helmfile:\n    app:\n      vars: {}\n"
			if tt.terraform != "" {
				manifest += "  terraform:\n" + tt.terraform
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
			t.Chdir(dir)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
			ClearFindStacksMapCache()
			t.Cleanup(ClearFindStacksMapCache)
			info := schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "deploy/dev"}
			atmosConfig, err := cfg.InitCliConfig(info, true)
			require.NoError(t, err)
			require.Equal(t, "Directory", atmosConfig.StackType)
			componentType, err := typedetect.Resolve("app", info.Stack, []string{"terraform", "helmfile"},
				newManifestPresenceProbe(&atmosConfig, &info, nil))
			if tt.wantError != nil {
				require.ErrorIs(t, err, tt.wantError)
				require.Empty(t, componentType)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "helmfile", componentType)
		})
	}
}
