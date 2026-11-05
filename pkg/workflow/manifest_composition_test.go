package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestManifestComposedStepsKeepIncludedScriptSource(t *testing.T) {
	base, manifest := manifestProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(base, "scripts", "lib"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(base, "scripts", "lib", "util.star"), []byte("x = 42\n"), 0o600))
	step := "type: script\ninterpreter: starlark\nscript: !include ./main.star\n"
	require.NoError(t, os.WriteFile(filepath.Join(base, "scripts", "step.yaml"), []byte(step), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(base, "scripts", "steps.yaml"), []byte("- !include ./step.yaml\n"), 0o600))
	t.Chdir(t.TempDir())
	for _, tc := range []struct{ name, content string }{
		{"merge", "base: &base\n  type: script\n  interpreter: starlark\n  script: !include scripts/main.star\nworkflows:\n  w:\n    steps:\n      - <<: *base\n        name: merged\n"},
		{"alias", "base: &base\n  type: script\n  interpreter: starlark\n  script: !include scripts/main.star\nworkflows:\n  w:\n    steps: [*base]\n"},
		{"merged interpreter", "base: &base\n  type: script\n  interpreter: starlark\nworkflows:\n  w:\n    steps:\n      - <<: *base\n        script: !include scripts/main.star\n"},
		{"included step", "workflows:\n  w:\n    steps:\n      - !include scripts/step.yaml\n"},
		{"included list", "workflows:\n  w:\n    steps: !include scripts/steps.yaml\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := loadTestManifest(t, base, manifest, tc.content)
			require.NoError(t, err)
			steps := parsed.Workflows["w"].Steps
			require.Len(t, steps, 1)
			require.Equal(t, filepath.Join(base, "scripts", "main.star"), steps[0].ScriptSource)
			engine, ok := script.Get("starlark")
			require.True(t, ok)
			_, err = engine.Execute(context.Background(), script.Spec{Source: steps[0].Script, SourcePath: steps[0].ScriptSource})
			require.NoError(t, err, "load must use the included file's directory, not cwd")
		})
	}
}

func TestManifestMergedInlineOverrideHasNoSource(t *testing.T) {
	base, manifest := manifestProject(t)
	parsed, err := loadTestManifest(t, base, manifest, "base: &base\n  type: script\n  interpreter: starlark\n  script: !include scripts/main.star\nworkflows:\n  w:\n    steps:\n      - <<: *base\n        script: print('inline')\n")
	require.NoError(t, err)
	require.Empty(t, parsed.Workflows["w"].Steps[0].ScriptSource)
}
