package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
