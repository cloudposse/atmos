package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessStacksSurfacesComponentProcessingErrors verifies that, for name_pattern stacks,
// a component that is defined in the requested stack but fails to process (for example an
// invalid `retry:` block) reports that processing error instead of the generic
// "could not find the component" message.
func TestProcessStacksSurfacesComponentProcessingErrors(t *testing.T) {
	tests := []struct {
		name          string
		manifests     map[string]string
		stack         string
		wantError     error
		wantContains  []string
		wantNotInText string
	}{
		{
			name: "invalid retry key in requested stack",
			manifests: map[string]string{
				"dev.yaml": "vars: {stage: dev}\ncomponents:\n  terraform:\n    app:\n      retry: {max_attempts: 2, backoff: constant}\n      vars: {a: 1}\n",
			},
			stack:         "dev",
			wantError:     schema.ErrInvalidRetryConfig,
			wantContains:  []string{"components.terraform.app.retry", "backoff"},
			wantNotInText: "Could not find the component",
		},
		{
			name: "component missing in requested stack stays not found",
			manifests: map[string]string{
				"dev.yaml":  "vars: {stage: dev}\ncomponents:\n  terraform:\n    other:\n      vars: {a: 1}\n",
				"prod.yaml": "vars: {stage: prod}\ncomponents:\n  terraform:\n    app:\n      retry: {max_attempts: 2, backoff: constant}\n      vars: {a: 1}\n",
			},
			stack:        "dev",
			wantError:    errUtils.ErrInvalidComponent,
			wantContains: []string{"Could not find the component", "prod", "backoff"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "components", "terraform", "app"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`
base_path: "."
stacks:
  base_path: stacks
  included_paths: ["**/*"]
  name_pattern: "{stage}"
components:
  terraform:
    base_path: components/terraform
`), 0o644))
			for name, content := range tt.manifests {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", name), []byte(content), 0o644))
			}
			t.Chdir(dir)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
			ClearFindStacksMapCache()
			t.Cleanup(ClearFindStacksMapCache)

			info := schema.ConfigAndStacksInfo{ComponentFromArg: "app", ComponentType: "terraform", Stack: tt.stack}
			atmosConfig, err := cfg.InitCliConfig(info, true)
			require.NoError(t, err)

			_, err = ProcessStacks(&atmosConfig, info, true, true, true, nil, nil)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.wantError)
			for _, want := range tt.wantContains {
				require.Contains(t, err.Error(), want)
			}
			if tt.wantNotInText != "" {
				require.NotContains(t, err.Error(), tt.wantNotInText)
			}
		})
	}
}
