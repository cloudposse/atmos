package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStackComponentMapsForUI(t *testing.T) {
	stack := func(names ...string) map[string]any {
		components := map[string]any{"abstract": map[string]any{"metadata": map[string]any{"type": "abstract"}}}
		for _, name := range names {
			components[name] = map[string]any{}
		}
		return map[string]any{"components": map[string]any{"terraform": components}}
	}
	stacks, components := stackComponentMapsForUI(map[string]any{
		"prod": stack("vpc", "dns"), "dev": stack("vpc"),
		"invalid": "invalid", "empty": map[string]any{},
		"helmfile": map[string]any{"components": map[string]any{"helmfile": map[string]any{"chart": map[string]any{}}}},
	})
	require.Equal(t, []string{"dns", "vpc"}, stacks["prod"])
	require.Equal(t, []string{"vpc"}, stacks["dev"])
	require.Empty(t, stacks["invalid"])
	require.Empty(t, stacks["empty"])
	require.Empty(t, stacks["helmfile"])
	require.Equal(t, map[string][]string{"dns": {"prod"}, "vpc": {"dev", "prod"}}, components)
}

func TestAtmosUISelectionDescribeAndValidate(t *testing.T) {
	for _, command := range []string{"describe component", "describe dependents", "validate component"} {
		t.Run(command, func(t *testing.T) {
			ac, _, _ := setupDeferredCacheTarget(t)
			require.NoError(t, executeAtmosUISelection(ac, command, "target", "dev"))
			require.Error(t, executeAtmosUISelection(ac, command, "missing", "dev"))
		})
	}
}

func TestAtmosUISelectionUnrecognizedCommandDoesNotExecute(t *testing.T) {
	require.NoError(t, executeAtmosUISelection(&schema.AtmosConfiguration{}, "unknown", "target", "dev"))
}

func TestAtmosUIRejectsInvalidStacksBeforeStarting(t *testing.T) {
	ac, _, _ := setupDeferredCacheTarget(t)
	ac.StackConfigFilesAbsolutePaths = []string{filepath.Join(t.TempDir(), "missing.yaml")}
	require.ErrorContains(t, ExecuteAtmosCmdWithConfig(ac), "missing.yaml")
}
