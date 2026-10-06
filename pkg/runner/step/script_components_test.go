package step

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/custom"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestScriptComponentProviderPathAndSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const componentType = "starlark-path-test"
	require.NoError(t, component.Register(custom.NewProvider(componentType, "custom-apps")))
	ref := script.ComponentRef{Name: "api-dev", Stack: "dev", Type: componentType}
	section := map[string]any{"component": "api-dev", "metadata": map[string]any{"component": "shared-app"}, "vars": map[string]any{"region": "test"}}
	resolved, err := scriptComponent(&schema.AtmosConfiguration{BasePath: dir}, ref, &schema.ConfigAndStacksInfo{
		ComponentSection: section, ComponentEnvSection: map[string]any{"VERSION": 42, "EMPTY": nil, "NULL": "null"},
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "custom-apps", "shared-app"), resolved.Path)
	assert.Equal(t, "api-dev", resolved.Name)
	assert.Equal(t, "shared-app", resolved.Implementation)
	assert.Equal(t, map[string]string{"VERSION": "42"}, resolved.Env)
	resolved.Config["vars"].(map[string]any)["region"] = "changed"
	assert.Equal(t, "test", section["vars"].(map[string]any)["region"])
	section["component"] = "other"
	assert.Equal(t, "api-dev", resolved.Config["component"])
}

func TestScriptComponentReferenceAndResolver(t *testing.T) {
	t.Parallel()
	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	vars.SetTemplateData(map[string]any{"Component": map[string]any{"atmos_component": "api", "atmos_stack": "dev", "component_type": "application"}})
	ref := ScriptComponentRef(vars)
	require.Equal(t, &script.ComponentRef{Name: "api", Stack: "dev", Type: "application"}, ref)
	vars.SetScriptComponentInfoResolver(func(_ context.Context, name, stack, componentType string) (*schema.ConfigAndStacksInfo, error) {
		assert.Equal(t, "api", name)
		assert.Equal(t, "dev", stack)
		assert.Equal(t, "application", componentType)
		return &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"component": "shared"}}, nil
	})
	result, err := ScriptComponentResolver(vars)(context.Background(), *ref)
	require.NoError(t, err)
	assert.Equal(t, "shared", result.Implementation)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ScriptComponentResolver(vars)(ctx, *ref)
	require.ErrorIs(t, err, context.Canceled)
}
