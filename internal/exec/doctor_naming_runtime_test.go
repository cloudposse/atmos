package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compare Doctor's static results with the full command pipeline, including the later
// component template pass. This guards evaluation order and disabled-templating behavior.
func TestDoctorNamingMatchesTerraformCommandPipeline(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, tc := range []struct{ name, field, value string }{
			{"legacy", "terraform_workspace_pattern", "{stage}-{component}"},
			{"mixed-pattern", "terraform_workspace_pattern", "{{ .vars.region }}-{component}"},
			{"mixed-template", "terraform_workspace_template", "{{ .vars.region }}-{component}"},
		} {
			t.Run(fmt.Sprintf("%s/templates=%t", tc.name, enabled), func(t *testing.T) {
				root := writeStackFixture(t, map[string]string{
					"atmos.yaml": fmt.Sprintf("base_path: .\ncomponents:\n  terraform:\n    base_path: components/terraform\nstacks:\n  base_path: stacks\n  included_paths: ['**/*']\n  name_pattern: '{stage}'\ntemplates:\n  settings:\n    enabled: %t\n", enabled),
					"components/terraform/service/app/main.tf": "",
				})
				t.Chdir(root)
				t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
				t.Setenv("ATMOS_BASE_PATH", root)
				t.Cleanup(ClearFindStacksMapCache)
				resolve := func(field, value string) string {
					t.Helper()
					require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o700))
					manifest := "vars:\n  stage: dev\n  region: us-east-1\nmetadata:\n  " + field + ": '" + value + "'\ncomponents:\n  terraform:\n    service/app: {}\n"
					require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(manifest), 0o600))
					ClearFindStacksMapCache()
					ClearFileContentCache()
					info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "service/app", ComponentType: cfg.TerraformComponentType, AuthDisabled: true}
					config, err := cfg.InitCliConfig(info, true)
					require.NoError(t, err)
					predicted, err := StaticNamingResults(context.Background(), &config, StaticStackInputs{Files: []string{"dev.yaml"}})
					require.NoError(t, err)
					actual, err := ProcessStacks(&config, info, true, true, false, nil, nil)
					require.NoError(t, err)
					require.Equal(t, actual.TerraformWorkspace, predicted[naming.Identity{Manifest: "dev", ComponentType: "terraform", Component: "service/app"}].Workspace)
					return actual.TerraformWorkspace
				}
				before := resolve(tc.field, tc.value)
				replacement, _, _ := naming.Upgrade(tc.value)
				after := resolve("terraform_workspace_template", replacement)
				switch tc.name {
				case "legacy":
					require.Equal(t, before, after)
				case "mixed-pattern":
					require.Equal(t, enabled, before == after)
				case "mixed-template":
					require.NotEqual(t, before, after)
				}
			})
		}
	}
}
