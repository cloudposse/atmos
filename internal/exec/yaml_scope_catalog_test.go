package exec

import (
	"os"
	"path/filepath"
	"testing"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestWorkflowFunctionScopeMatchesDispatcher(t *testing.T) {
	t.Setenv("ATMOS_SCOPE_TEST", "resolved")
	config := &schema.AtmosConfiguration{}
	for _, value := range []struct {
		name, input string
		resolves    bool
	}{
		{"env", "!env ATMOS_SCOPE_TEST", true},
		{"cwd", "!cwd", false},
		{"random", "!random 1 2", false},
		{"store", "!store account key", false},
	} {
		t.Run(value.name, func(t *testing.T) {
			result, err := resolveStepFunctionString(config, value.input)
			require.NoError(t, err)
			require.Equal(t, value.resolves, result != value.input)
			support := fntag.Scope(value.name, fntag.WorkflowInteractiveContext)
			require.Equal(t, value.resolves, support.StringForm)
		})
	}
}

func TestVendorFunctionScopeMatchesLoader(t *testing.T) {
	t.Setenv("ATMOS_SCOPE_VENDOR", "should-not-be-resolved")
	path := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(path, []byte("apiVersion: atmos/v1\nkind: AtmosVendorConfig\nspec:\n  sources:\n    - component: example\n      source: !env ATMOS_SCOPE_VENDOR\n      targets: [components/example]\n"), 0o600))
	config, err := mergeVendorConfigFiles([]string{path})
	require.NoError(t, err)
	require.Equal(t, "ATMOS_SCOPE_VENDOR", config.Spec.Sources[0].Source)
	require.False(t, fntag.Scope("env", fntag.VendorContext).NativeTag)
}

func TestStackFunctionScopeMatchesStructuralConsumers(t *testing.T) {
	t.Setenv("ATMOS_SCOPE_STRUCTURAL", "resolved")
	expression := "!env ATMOS_SCOPE_STRUCTURAL"
	dir := t.TempDir()
	input := map[string]any{
		"extension": expression,
		"vars":      map[string]any{"control": expression},
		"components": map[string]any{"terraform": map[string]any{"app": map[string]any{
			"metadata": map[string]any{"component": expression},
		}}},
	}
	config := &schema.AtmosConfiguration{}
	merged, _, err := ProcessStackConfig(config, dir, "", "", "", "", filepath.Join(dir, "dev.yaml"), input, false, false, "", map[string]map[string][]string{}, map[string]map[string]any{}, false)
	require.NoError(t, err)
	require.NotContains(t, merged, "extension", "unknown root fields do not reach the component evaluator")
	app := merged["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
	metadata, base, abstract, enabled, locked := ProcessComponentMetadata("app", app)
	require.Equal(t, expression, metadata["component"])
	require.False(t, abstract)
	require.True(t, enabled)
	require.False(t, locked)
	require.Equal(t, expression, base, "component selection consumes the unresolved value")
	evaluated, err := ProcessCustomYamlTags(config, app, "dev", nil, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	require.Equal(t, "resolved", evaluated["vars"].(map[string]any)["control"])
	require.False(t, fntag.Scope("env", fntag.StackStructuralContext).NativeTag)
	require.True(t, fntag.Scope("env", fntag.StackContext).NativeTag)
}
