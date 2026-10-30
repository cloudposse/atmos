package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCleanupTerraformFiles_InspectionPreservesSavedPlan(t *testing.T) {
	for _, command := range []string{"plan-diff", "generate", "output", "state", "validate", "fmt", "graph", "providers", "version"} {
		t.Run(command, func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			info := &schema.ConfigAndStacksInfo{SubCommand: command, Component: "demo", ContextPrefix: "dev"}
			path := constructTerraformComponentPlanfilePath(config, info)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			original := []byte("saved binary plan for later apply")
			require.NoError(t, os.WriteFile(path, original, 0o600))
			cleanupTerraformFiles(config, info)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, after)
		})
	}
}

func TestCopyPlanFileIfNeeded_PreservesExistingSameName(t *testing.T) {
	dir := t.TempDir()
	component := filepath.Join(dir, "component")
	external := filepath.Join(dir, "external")
	require.NoError(t, os.MkdirAll(component, 0o755))
	require.NoError(t, os.MkdirAll(external, 0o755))
	source := filepath.Join(external, "saved.planfile")
	existing := filepath.Join(component, "saved.planfile")
	require.NoError(t, os.WriteFile(source, []byte("external plan"), 0o600))
	require.NoError(t, os.WriteFile(existing, []byte("existing plan"), 0o600))
	copied, cleanup, err := copyPlanFileIfNeeded(source, component)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()
	require.NotEqual(t, existing, copied)
	data, err := os.ReadFile(copied)
	require.NoError(t, err)
	require.Equal(t, "external plan", string(data))
	cleanup()
	require.NoFileExists(t, copied)
	data, err = os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "existing plan", string(data))
	data, err = os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, "external plan", string(data))
}
