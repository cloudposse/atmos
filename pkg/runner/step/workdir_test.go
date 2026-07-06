package step

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkdirHandlerExecuteProvisionsLocalSource(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("demo\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(sourceDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "nested", "file.txt"), []byte("nested\n"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "workdir")
	handler := &WorkdirHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false)}

	result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "fixture",
		Type:   schema.TaskTypeWorkdir,
		Source: sourceDir,
		Path:   targetDir,
		Reset:  true,
	}, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, targetDir, result.Value)
	assert.Equal(t, sourceDir, result.Metadata["source"])

	content, err := os.ReadFile(filepath.Join(targetDir, "nested", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested\n", string(content))
}

func TestWorkdirHandlerExecuteRequiresResetForExistingTarget(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("demo\n"), 0o644))

	targetDir := t.TempDir()
	handler := &WorkdirHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false)}

	_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "fixture",
		Type:   schema.TaskTypeWorkdir,
		Source: map[string]any{"uri": sourceDir},
		Path:   targetDir,
	}, NewVariables())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set reset: true")
}

func TestWorkdirHandlerExecuteNormalizesRelativeTargetPath(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("demo\n"), 0o644))

	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(nested))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(previous))
	})

	handler := &WorkdirHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false)}
	result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "fixture",
		Type:   schema.TaskTypeWorkdir,
		Source: sourceDir,
		Path:   "../workdir",
		Reset:  true,
	}, NewVariables())
	require.NoError(t, err)

	expected, err := filepath.Abs("../workdir")
	require.NoError(t, err)
	assert.Equal(t, expected, result.Value)
	assert.FileExists(t, filepath.Join(expected, "README.md"))
}

func TestWorkdirHandlerValidateRequiresPathAndSource(t *testing.T) {
	handler := &WorkdirHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false)}

	err := handler.Validate(&schema.WorkflowStep{Name: "fixture", Source: "src"})
	require.Error(t, err)

	err = handler.Validate(&schema.WorkflowStep{Name: "fixture", Path: "target"})
	require.Error(t, err)

	err = handler.Validate(&schema.WorkflowStep{Name: "fixture", Path: "target", Source: "src"})
	require.NoError(t, err)
}

func TestWorkdirHandlerExecuteRequiresResolvedPath(t *testing.T) {
	handler := &WorkdirHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false)}
	_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "fixture",
		Type:   schema.TaskTypeWorkdir,
		Source: t.TempDir(),
		Path:   "",
	}, NewVariables())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path")
}

func TestResolveWorkdirSourceValueResolvesNestedTemplates(t *testing.T) {
	vars := NewVariables()
	root := t.TempDir()
	vars.SetEnv("ROOT", filepath.ToSlash(root))
	vars.SetEnv("REF", "main")

	resolved, err := resolveWorkdirSourceValue(map[string]any{
		"uri":     "{{ .env.ROOT }}/component",
		"version": "{{ .env.REF }}",
		"options": []any{
			"plain",
			map[string]any{"nested": "{{ .env.REF }}"},
		},
	}, vars)
	require.NoError(t, err)

	sourceMap, ok := resolved.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, filepath.ToSlash(filepath.Join(root, "component")), sourceMap["uri"])
	assert.Equal(t, "main", sourceMap["version"])
	options := sourceMap["options"].([]any)
	assert.Equal(t, "plain", options[0])
	assert.Equal(t, "main", options[1].(map[string]any)["nested"])
}

func TestResolveWorkdirSourceValueSupportsYAMLAnyMapsAndRejectsNonStringKeys(t *testing.T) {
	vars := NewVariables()
	src := t.TempDir()
	vars.SetEnv("SRC", filepath.ToSlash(src))

	resolved, err := resolveWorkdirSourceValue(map[any]any{
		"uri": "{{ .env.SRC }}",
	}, vars)
	require.NoError(t, err)
	assert.Equal(t, filepath.ToSlash(src), resolved.(map[string]any)["uri"])

	_, err = resolveWorkdirSourceValue(map[any]any{42: "value"}, vars)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkdirSourceKeyInvalid))
}

func TestResolveSourceSpecRequiresURI(t *testing.T) {
	handler := &WorkdirHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false)}
	_, err := handler.resolveSourceSpec(&schema.WorkflowStep{Name: "fixture", Source: map[string]any{"uri": ""}}, NewVariables())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkdirSourceRequired))
}
