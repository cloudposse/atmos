package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

const starlarkContextProjectAtmosYAML = `base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{.vars.stage}}"
`

const starlarkContextProjectStack = `locals:
  owner: stack-level
  tier: gold

vars:
  stage: dev

components:
  terraform:
    mock:
      vars:
        message: base
    ctxecho:
      metadata:
        component: mock
      locals:
        owner: component-level
      settings:
        team: plat
      vars:
        message: ctxecho
        echo: !starlark |
          return {
              "component": ctx.component,
              "component_type": ctx.component_type,
              "stack": ctx.stack,
              "locals": ctx.locals,
              "settings_json": json.encode(ctx.settings),
              "stage_json": json.encode(ctx.vars["stage"]),
          }
`

// writeStarlarkContextProject creates a throwaway project whose stack has stack-level locals and an
// instance (ctxecho) of a base component (mock), then makes it the working directory.
func writeStarlarkContextProject(t *testing.T, extraStacks map[string]string) schema.AtmosConfiguration {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"atmos.yaml": starlarkContextProjectAtmosYAML,
		filepath.Join("stacks", "deploy", "dev.yaml"): starlarkContextProjectStack,
		filepath.Join("components", "terraform", "mock", "main.tf"): `variable "message" {
  type    = string
  default = ""
}
`,
	}
	for name, content := range extraStacks {
		files[filepath.Join("stacks", "deploy", name)] = content
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", "")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// TestStarlarkContextMatchesAcrossDescribeCommands guards the ctx.* fields a !starlark value sees:
// describe stacks and describe component must agree, and both must report the instance name,
// the component type, the logical stack name, and stack-level locals under component locals.
func TestStarlarkContextMatchesAcrossDescribeCommands(t *testing.T) {
	atmosConfig := writeStarlarkContextProject(t, nil)

	stacks, err := ExecuteDescribeStacks(&atmosConfig, "", []string{"ctxecho"}, nil, nil, false, true, true, false, nil, nil)
	require.NoError(t, err)
	fromStacks := stacks["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["ctxecho"].(map[string]any)["vars"].(map[string]any)["echo"]

	component, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		AtmosConfig:          &atmosConfig,
		Component:            "ctxecho",
		Stack:                "dev",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	fromComponent := component["vars"].(map[string]any)["echo"]

	want := map[string]any{
		"component":      "ctxecho",
		"component_type": "terraform",
		"stack":          "dev",
		"locals":         map[string]any{"owner": "component-level", "tier": "gold"},
		"settings_json":  `{"team":"plat"}`,
		"stage_json":     `"dev"`,
	}
	assert.Equal(t, want, fromStacks, "describe stacks")
	assert.Equal(t, want, fromComponent, "describe component")
}

// TestStarlarkIdentityIsRejectedEverywhere verifies that a stack name derived from !starlark fails
// with the sentinel from describe stacks and from single-component resolution, and that a project
// with literal names is unaffected.
func TestStarlarkIdentityIsRejectedEverywhere(t *testing.T) {
	computed := `vars:
  stage: !starlark return "computed-stage"

components:
  terraform:
    mock:
      vars:
        message: computed
`
	atmosConfig := writeStarlarkContextProject(t, map[string]string{"computed.yaml": computed})

	t.Run("describe stacks", func(t *testing.T) {
		_, err := ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, true, true, false, nil, nil)
		require.ErrorIs(t, err, errUtils.ErrStarlarkStackIdentity)
	})

	t.Run("describe component by manifest path", func(t *testing.T) {
		_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			AtmosConfig: &atmosConfig, Component: "mock", Stack: "deploy/computed", ProcessTemplates: true, ProcessYamlFunctions: true,
		})
		require.ErrorIs(t, err, errUtils.ErrStarlarkStackIdentity)
	})

	t.Run("a literal stack is still found by its logical name", func(t *testing.T) {
		section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			AtmosConfig: &atmosConfig, Component: "mock", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
		})
		require.NoError(t, err)
		assert.Equal(t, "base", section["vars"].(map[string]any)["message"])
	})

	t.Run("a missing component is not reported as an identity error", func(t *testing.T) {
		_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			AtmosConfig: &atmosConfig, Component: "does-not-exist", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
		})
		require.Error(t, err)
		assert.NotErrorIs(t, err, errUtils.ErrStarlarkStackIdentity)
	})
}
