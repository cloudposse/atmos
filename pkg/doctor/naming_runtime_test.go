package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	stackexec "github.com/cloudposse/atmos/internal/exec"
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
				t.Cleanup(stackexec.ClearFindStacksMapCache)
				resolve := func(field, value string) string {
					t.Helper()
					require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o700))
					manifest := "vars:\n  stage: dev\n  region: us-east-1\nmetadata:\n  " + field + ": '" + value + "'\ncomponents:\n  terraform:\n    service/app: {}\n"
					require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(manifest), 0o600))
					stackexec.ClearFindStacksMapCache()
					stackexec.ClearFileContentCache()
					info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "service/app", ComponentType: cfg.TerraformComponentType, AuthDisabled: true}
					config, err := cfg.InitCliConfig(info, true)
					require.NoError(t, err)
					predicted, err := staticNamingResults(context.Background(), &config, stackexec.StaticStackInputs{Files: []string{"dev.yaml"}})
					require.NoError(t, err)
					actual, err := stackexec.ProcessStacks(&config, info, true, true, false, nil, nil)
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

// Legacy tokens read the root naming context even when a Go template rebinds dot.
func TestDoctorNamingUpgradePreservesScopedTokenContext(t *testing.T) {
	scope := map[string]any{
		"vars":            map[string]any{"stage": "wrong", "attributes": []any{"wrong"}},
		"atmos_component": "wrong", "component": "wrong", "empty": []any{}, "enabled": true,
	}
	vars := map[string]any{
		"stage": "dev", "attributes": []any{"one", "two"},
		"scope": scope, "items": []any{scope}, "empty": []any{},
	}
	data := map[string]any{"vars": vars, "atmos_component": "app", "component": "base"}
	legacy := cfg.GetContextFromVars(vars)
	legacy.Component, legacy.BaseComponent = "app", "base"
	for _, tc := range []struct{ name, pattern, want string }{
		{"range body", `{{ range .vars.items }}{stage}-{component}-{base-component}-{attributes}{{ end }}`, "dev-app-base-one-two"},
		{"with body", `{{ with .vars.scope }}{stage}-{component}-{base-component}-{attributes}{{ end }}`, "dev-app-base-one-two"},
		{"if inside range", `{{ range .vars.items }}{{ if .enabled }}{stage}{{ else }}{component}{{ end }}{{ end }}`, "dev"},
		{"if else inside with", `{{ with .vars.scope }}{{ if .empty }}{stage}{{ else }}{component}{{ end }}{{ end }}`, "app"},
		{"with else inside range", `{{ range .vars.items }}{{ with .empty }}{stage}{{ else }}{component}{{ end }}{{ end }}`, "app"},
		{"range else inside with", `{{ with .vars.scope }}{{ range .empty }}{stage}{{ else }}{component}{{ end }}{{ end }}`, "app"},
		{"root with else", `{{ with .vars.empty }}{component}{{ else }}{stage}{{ end }}`, "dev"},
		{"root range else", `{{ range .vars.empty }}{component}{{ else }}{stage}{{ end }}`, "dev"},
		{"scope restored", `{{ with .vars.scope }}{stage}{{ end }}-{component}`, "dev-app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			before, err := stackexec.ProcessTmpl(config, "legacy-scoped-naming", cfg.ReplaceContextTokens(legacy, tc.pattern), data, false)
			require.NoError(t, err)
			require.Equal(t, tc.want, before)
			replacement, tokens, unsupported := naming.Upgrade(tc.pattern)
			require.NotEmpty(t, tokens)
			require.Empty(t, unsupported)
			after, err := stackexec.ProcessTmpl(config, "upgraded-scoped-naming", replacement, data, false)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.False(t, naming.SafeTemplate(replacement), "scoped templates must still require manual migration review")
		})
	}
}
