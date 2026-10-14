package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHookControlChildrenKeepWorkingDirectoryAndProcessEnv(t *testing.T) {
	root := t.TempDir()
	component := filepath.Join(root, "components", "test-component")
	require.NoError(t, os.MkdirAll(filepath.Join(component, "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(component, "marker.txt"), []byte("component"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(component, "sub", "marker.txt"), []byte("sub"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "marker.txt"), []byte("invocation"), 0o600))
	t.Chdir(root)

	for _, kind := range []string{"parallel", "matrix"} {
		t.Run(kind, func(t *testing.T) {
			payload := map[string]any{
				"env": map[string]string{"FIELD_PARENT": "parent", "FIELD_CHILD": "parent"},
				"steps": []any{
					map[string]any{"name": "default", "type": "script", "interpreter": "starlark", "script": `if fs.read_file("marker.txt") != "component": fail("default cwd lost")`},
					map[string]any{"name": "bare", "type": "script", "interpreter": "starlark", "working_directory": "sub", "script": `if fs.read_file("marker.txt") != "sub": fail("bare cwd lost")`},
					map[string]any{"name": "dot", "type": "script", "interpreter": "starlark", "working_directory": ".", "script": `if fs.read_file("marker.txt") != "invocation": fail("dot cwd lost")`},
					map[string]any{"name": "environment", "type": "shell", "env": map[string]string{"FIELD_CHILD": "child"}, "command": `test "$FIELD_PARENT" = parent && test "$FIELD_CHILD" = child`},
				},
			}
			if kind == "matrix" {
				payload["matrix"] = map[string][]string{"region": {"dev", "prod"}}
			}
			ctx := stepExecContext(&Hook{Kind: stepKindName, Type: kind, OnFailure: OnFailureFail, With: payload})
			ctx.AtmosConfig = &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
			_, err := stepEngine{}.Run(ctx)
			require.NoError(t, err)
		})
	}
}

func TestHookControlAtmosChildKeepsInvocationDirectory(t *testing.T) {
	ctx := stepExecContext(&Hook{})
	ctx.AtmosConfig = &schema.AtmosConfiguration{TerraformDirAbsolutePath: t.TempDir()}
	group := &schema.WorkflowStep{Type: schema.TaskTypeParallel, Steps: []schema.WorkflowStep{
		{Type: schema.TaskTypeAtmos},
		{Type: schema.TaskTypeScript},
	}}
	setDefaultStepWorkingDirectory(ctx, group)
	require.Empty(t, group.Steps[0].WorkingDirectory)
	require.Equal(t, ComponentPath(ctx), group.Steps[1].WorkingDirectory)
}
