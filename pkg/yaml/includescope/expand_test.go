package includescope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/utils"
)

func TestExpandLocalYAMLNestedScopeAndRepeatedIncludes(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "steps")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step.yml"), []byte("script: !include ./main.star\ninterpreter: starlark\n"), 0o600))
	var root yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("steps:\n - !include steps/step.yml\n - !include steps/step.yml\n"), &root))
	scope := Scope{File: filepath.Join(base, "workflow.yaml"), BasePath: base}
	require.NoError(t, scope.ExpandLocalYAML(&root))
	for _, step := range root.Content[0].Content[1].Content {
		script := step.Content[1]
		path, ok := LocalFile(script)
		require.True(t, ok, "nested tags must survive expansion")
		require.Equal(t, filepath.Join(dir, "main.star"), path)
	}
}

func TestExpandLocalYAMLFailures(t *testing.T) {
	base := t.TempDir()
	scope := Scope{File: filepath.Join(base, "workflow.yaml"), BasePath: base}
	for name, content := range map[string]string{"cycle.yaml": "x: !include cycle.yaml", "bad.yaml": "[", "empty.yaml": ""} {
		require.NoError(t, os.WriteFile(filepath.Join(base, name), []byte(content), 0o600))
	}
	for _, name := range []string{"missing.yaml", "cycle.yaml", "bad.yaml"} {
		t.Run(name, func(t *testing.T) {
			var root yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("x: !include "+name), &root))
			require.ErrorIs(t, scope.ExpandLocalYAML(&root), utils.ErrIncludeYamlFunctionInvalidFile)
		})
	}
	var root yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("x: !include empty.yaml"), &root))
	require.NoError(t, scope.ExpandLocalYAML(&root))
	require.Equal(t, "!!null", root.Content[0].Content[1].Tag)
	require.NoError(t, scope.ExpandLocalYAML(nil))
}

func TestExpandLocalYAMLLeavesOtherIncludesToLoader(t *testing.T) {
	scope := Scope{File: filepath.Join(t.TempDir(), "workflow.yaml")}
	for _, text := range []string{"!include.raw missing.yaml", "!include missing.yaml .steps", "!include missing.star", "!include https://example.com/workflow.yaml"} {
		t.Run(text, func(t *testing.T) {
			var root yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("x: "+text), &root))
			before := *root.Content[0].Content[1]
			require.NoError(t, scope.ExpandLocalYAML(&root))
			require.Equal(t, before, *root.Content[0].Content[1])
		})
	}
}
