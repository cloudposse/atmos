package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevcontainerExecutableKeyCase(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)
	dir := t.TempDir()
	t.Setenv("DEVCONTAINER_CASE_TEST", "resolved")
	files := map[string]string{
		"base.yaml": `devcontainer:
  app:
    spec:
      image: alpine
      containerEnv:
        INHERITED: base
        SHARED: base
      build:
        dockerfile: Dockerfile
        args:
          INHERITED_ARG: base
`,
		"atmos.yaml": `import: [base.yaml]
devcontainer:
  app:
    spec:
      containerEnv:
        UPPER: !env DEVCONTAINER_CASE_TEST
        upper: lowercase
        SHARED: override
      build:
        args:
          ARG_NAME: upper
          arg_name: lower
  included:
    spec: !include spec.yaml
  aliases:
    spec:
      image: alpine
      containerEnv: &env
        ALIAS_ENV: alias
      build:
        dockerfile: Dockerfile
        args: *env
  included_list:
    spec:
      - !include spec.yaml
      - containerEnv:
          LIST_OVERRIDE: override
  listed:
    spec:
      - image: alpine
        containerEnv:
          LIST_ENV: value
      - build:
          dockerfile: Dockerfile
          args:
            LIST_ARG: value
`,
		"spec.yaml": `image: alpine
containerEnv:
  INCLUDE_ENV: value
build:
  dockerfile: Dockerfile
  args:
    INCLUDE_ARG: value
`,
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	cfg := loadConfigViaMergeConfig(t, dir)
	app := cfg.Devcontainer["app"].(map[string]any)["spec"].(map[string]any)
	require.Equal(t, map[string]any{"INHERITED": "base", "UPPER": "resolved", "upper": "lowercase", "SHARED": "override"}, app["containerenv"])
	require.Equal(t, map[string]any{"INHERITED_ARG": "base", "ARG_NAME": "upper", "arg_name": "lower"}, app["build"].(map[string]any)["args"])
	included := cfg.Devcontainer["included"].(map[string]any)["spec"].(map[string]any)
	require.Equal(t, map[string]any{"INCLUDE_ENV": "value"}, included["containerenv"])
	require.Equal(t, map[string]any{"INCLUDE_ARG": "value"}, included["build"].(map[string]any)["args"])
	aliases := cfg.Devcontainer["aliases"].(map[string]any)["spec"].(map[string]any)
	require.Equal(t, map[string]any{"ALIAS_ENV": "alias"}, aliases["containerenv"])
	require.Equal(t, map[string]any{"ALIAS_ENV": "alias"}, aliases["build"].(map[string]any)["args"])
	includedList := cfg.Devcontainer["included_list"].(map[string]any)["spec"].([]any)
	require.Equal(t, map[string]any{"INCLUDE_ENV": "value"}, includedList[0].(map[string]any)["containerenv"])
	listed := cfg.Devcontainer["listed"].(map[string]any)["spec"].([]any)
	require.Equal(t, map[string]any{"LIST_ENV": "value"}, listed[0].(map[string]any)["containerenv"])
	require.Equal(t, map[string]any{"LIST_ARG": "value"}, listed[1].(map[string]any)["build"].(map[string]any)["args"])
}

func TestExecutableKeyCaseCrossSectionAliases(t *testing.T) {
	for _, first := range []string{"devcontainer", "mcp"} {
		for _, merge := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/merge=%t", first, merge), func(t *testing.T) {
				anchor := "&shared\n        UPPER: value\n        upper: lower\n"
				alias := "*shared\n"
				if merge {
					alias = "\n        <<: *shared\n"
				}
				devEnv, mcpEnv := anchor, alias
				if first == "mcp" {
					devEnv, mcpEnv = alias, anchor
				}
				dev := "devcontainer:\n  app:\n    spec:\n      containerEnv: " + devEnv
				mcp := "mcp:\n  servers:\n    local:\n      command: echo\n      env: " + mcpEnv
				content := dev + mcp
				if first == "mcp" {
					content = mcp + dev
				}
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(content), 0o600))
				config := loadConfigViaMergeConfig(t, dir)
				spec := config.Devcontainer["app"].(map[string]any)["spec"].(map[string]any)
				require.Equal(t, map[string]any{"UPPER": "value", "upper": "lower"}, spec["containerenv"])
				require.Equal(t, map[string]string{"UPPER": "value", "upper": "lower"}, config.MCP.Servers["local"].Env)
			})
		}
	}
}
