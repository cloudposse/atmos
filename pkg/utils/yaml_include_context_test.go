package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/policy/guard"
)

func TestIncludeScriptSourcePreservesIncludedDataContext(t *testing.T) {
	cfg, manifest := scriptSourceProject(t)
	content := "- type: script\n  interpreter: starlark\n  script: !include.raw ./hook.star\n"
	require.NoError(t, os.WriteFile(filepath.Join(cfg.BasePath, "scripts", "steps.yaml"), []byte(content), 0o600))
	for _, section := range []string{"vars", "settings", "env", "metadata", "backend", "steps"} {
		t.Run(section, func(t *testing.T) {
			got, err := UnmarshalYAMLFromFile[map[string]any](cfg, section+": !include scripts/steps.yaml | eval\n", manifest)
			require.NoError(t, err)
			items, ok := got[section].([]any)
			require.True(t, ok)
			require.Len(t, items, 1)
			want := map[string]any{"type": "script", "interpreter": "starlark", "script": "print('hook')\n"}
			if section == "steps" {
				want[ScriptSourceKey] = "scripts/hook.star"
				want[ScriptSourceSHA256Key] = ScriptSourceHash("print('hook')\n")
			}
			assert.Equal(t, want, items[0], "included data must stay data; real script steps retain provenance")
		})
	}
}

// A YAML include is spliced as nodes rather than decoded, so its scalars must still be
// attributed to their source for function-provenance policies.
func TestIncludeSplicedYAMLKeepsProvenance(t *testing.T) {
	guard.ResetOrigins()
	t.Cleanup(guard.ResetOrigins)
	cfg, manifest := scriptSourceProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.BasePath, "scripts", "origin.yaml"), []byte("name: spliced-origin-probe\n"), 0o600))
	include := func(t *testing.T) {
		t.Helper()
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!include", Value: "scripts/origin.yaml"}
		require.NoError(t, ProcessIncludeTag(cfg, node, "scripts/origin.yaml", manifest))
		require.Equal(t, yaml.MappingNode, node.Kind, "a YAML include is spliced in as nodes")
	}

	t.Run("local parent manifest", func(t *testing.T) {
		include(t)
		assert.Equal(t, "local", guard.ValueOrigin("spliced-origin-probe").Provenance)
	})

	t.Run("remote parent manifest wins", func(t *testing.T) {
		guard.ResetOrigins()
		guard.RegisterRemote(filepath.Dir(manifest), "https://example.com/stacks")
		include(t)
		assert.Equal(t, "remote", guard.ValueOrigin("spliced-origin-probe").Provenance)
	})
}
