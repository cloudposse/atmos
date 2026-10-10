package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// manifestProject writes a project with scripts/main.star, stacks/workflows/local.star and
// stacks/workflows/data.yaml, and returns its base path and the manifest path.
func manifestProject(t *testing.T) (base, manifest string) {
	t.Helper()
	base = t.TempDir()
	files := map[string]string{
		filepath.Join("scripts", "main.star"):                 "load(\"lib/util.star\", \"x\")\n",
		filepath.Join("stacks", "workflows", "local.star"):    "print(\"local\")\n",
		filepath.Join("stacks", "workflows", "data.yaml"):     "value: 1\n",
		filepath.Join("stacks", "workflows", "workflow.yaml"): "",
	}
	for name, content := range files {
		require.NoError(t, os.MkdirAll(filepath.Join(base, filepath.Dir(name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(base, name), []byte(content), 0o600))
	}
	return base, filepath.Join(base, "stacks", "workflows", "workflow.yaml")
}

func loadTestManifest(t *testing.T, base, manifest, content string) (schema.WorkflowManifest, error) {
	t.Helper()
	return LoadManifest(&schema.AtmosConfiguration{BasePath: "./", BasePathAbsolute: base}, manifest, []byte(content))
}

func TestLoadManifestResolvesIncludesIndependentlyOfWorkingDirectory(t *testing.T) {
	base, manifest := manifestProject(t)
	absolute := filepath.Join(base, "scripts", "main.star")
	content := `
workflows:
  bare:
    steps:
      - {name: a, type: script, interpreter: starlark, script: !include scripts/main.star}
  dot:
    steps:
      - {name: a, type: script, interpreter: starlark, script: !include ./local.star}
  abs:
    steps:
      - {name: a, type: script, interpreter: starlark, script: !include ` + filepath.ToSlash(absolute) + `}
  raw:
    steps:
      - {name: a, type: script, interpreter: starlark, script: !include.raw scripts/main.star}
`
	wantSource := map[string]string{
		"bare": absolute,
		"dot":  filepath.Join(base, "stacks", "workflows", "local.star"),
		"abs":  absolute,
		"raw":  absolute,
	}
	wantScript := map[string]string{
		"bare": "load(\"lib/util.star\", \"x\")\n",
		"dot":  "print(\"local\")\n",
		"abs":  "load(\"lib/util.star\", \"x\")\n",
		"raw":  "load(\"lib/util.star\", \"x\")\n",
	}

	// A bare file of the same name in the working directory must not win.
	elsewhere := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(elsewhere, "scripts"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(elsewhere, "scripts", "main.star"), []byte("decoy"), 0o600))

	for _, dir := range []string{base, filepath.Join(base, "stacks"), elsewhere} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Chdir(dir)
			parsed, err := loadTestManifest(t, base, manifest, content)
			require.NoError(t, err)
			for name, source := range wantSource {
				steps := parsed.Workflows[name].Steps
				require.Len(t, steps, 1, name)
				assert.Equal(t, source, steps[0].ScriptSource, name)
				assert.Equal(t, wantScript[name], steps[0].Script, name)
			}
		})
	}
}

func TestLoadManifestRecordsSourcesOnNestedAndMixedSteps(t *testing.T) {
	base, manifest := manifestProject(t)
	absolute := filepath.Join(base, "scripts", "main.star")
	local := filepath.Join(base, "stacks", "workflows", "local.star")
	content := `
workflows:
  other:
    steps:
      - name: untouched
        type: shell
        command: echo hi
  nested:
    steps:
      - name: inline
        type: script
        interpreter: starlark
        script: |
          print("inline")
      - name: group
        type: parallel
        steps:
          - name: first
            type: script
            interpreter: starlark
            script: !include scripts/main.star
          - name: second
            type: script
            interpreter: starlark
            script: print("inline child")
          - name: inner-group
            type: parallel
            steps:
              - name: deep
                type: script
                interpreter: starlark
                script: !include ./local.star
      - name: last
        type: script
        interpreter: starlark
        script: !include scripts/main.star
`
	parsed, err := loadTestManifest(t, base, manifest, content)
	require.NoError(t, err)

	assert.Empty(t, parsed.Workflows["other"].Steps[0].ScriptSource)
	steps := parsed.Workflows["nested"].Steps
	require.Len(t, steps, 3)
	assert.Empty(t, steps[0].ScriptSource, "an inline script has no source file")
	require.Len(t, steps[1].Steps, 3)
	assert.Equal(t, absolute, steps[1].Steps[0].ScriptSource)
	assert.Empty(t, steps[1].Steps[1].ScriptSource)
	require.Len(t, steps[1].Steps[2].Steps, 1)
	assert.Equal(t, local, steps[1].Steps[2].Steps[0].ScriptSource)
	assert.Equal(t, absolute, steps[2].ScriptSource)
}

func TestLoadManifestRecordsNoSourceForQueriesAndNonScriptIncludes(t *testing.T) {
	base, manifest := manifestProject(t)
	content := `
workflows:
  w:
    steps:
      - name: queried
        type: script
        interpreter: starlark
        script: !include ./data.yaml .value
      - name: described
        type: shell
        command: echo hi
        description: !include ./local.star
`
	parsed, err := loadTestManifest(t, base, manifest, content)
	require.NoError(t, err)
	for _, step := range parsed.Workflows["w"].Steps {
		assert.Empty(t, step.ScriptSource, step.Name)
	}
}

func TestLoadManifestErrors(t *testing.T) {
	base, manifest := manifestProject(t)

	t.Run("missing include names the absolute path it looked for", func(t *testing.T) {
		_, err := loadTestManifest(t, base, manifest, "workflows:\n  w:\n    steps:\n      - name: a\n        script: !include scripts/nope.star\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), filepath.Join(base, "scripts", "nope.star"))
	})

	t.Run("syntax errors are returned unchanged", func(t *testing.T) {
		_, err := loadTestManifest(t, base, manifest, "workflows: [\n")
		require.Error(t, err)
	})

	t.Run("decode errors keep the original line numbers", func(t *testing.T) {
		content := "# a comment\n\n\nworkflows:\n  w:\n    steps:\n      - name: a\n        script: !include scripts/main.star\n        retry: not-a-mapping\n"
		_, err := loadTestManifest(t, base, manifest, content)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line 9")
	})

	t.Run("a manifest without workflows loads empty", func(t *testing.T) {
		parsed, err := loadTestManifest(t, base, manifest, "name: empty\n")
		require.NoError(t, err)
		assert.Nil(t, parsed.Workflows)
	})
}

func TestManifestBasePath(t *testing.T) {
	assert.Empty(t, manifestBasePath(nil))
	assert.Equal(t, "/abs", manifestBasePath(&schema.AtmosConfiguration{BasePath: "x", BasePathAbsolute: "/abs"}))
	abs, err := filepath.Abs("rel")
	require.NoError(t, err)
	assert.Equal(t, abs, manifestBasePath(&schema.AtmosConfiguration{BasePath: "rel"}))
}
