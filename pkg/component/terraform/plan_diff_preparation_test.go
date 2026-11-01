package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFilterPlanDiffFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "no flags to filter",
			args:     []string{"--var=foo=bar", "--auto-approve"},
			expected: []string{"--var=foo=bar", "--auto-approve"},
		},
		{
			name:     "with --orig= and --new= flags",
			args:     []string{"--orig=orig.plan", "--new=new.plan", "--var=foo=bar"},
			expected: []string{"--var=foo=bar"},
		},
		{
			name:     "with --orig and --new flags with space",
			args:     []string{"--orig", "orig.plan", "--var=foo=bar", "--new", "new.plan"},
			expected: []string{"--var=foo=bar"},
		},
		{
			name:     "with --orig flag but missing value",
			args:     []string{"--orig", "--var=foo=bar", "--new=new.plan"},
			expected: []string{"--var=foo=bar"},
		},
		{
			name:     "with mixed flags",
			args:     []string{"--var=foo=bar", "--orig=orig.plan", "--auto-approve", "--new=new.plan"},
			expected: []string{"--var=foo=bar", "--auto-approve"},
		},
		{
			name:     "with -var flag using space (integration test format)",
			args:     []string{"--orig=orig.plan", "-var", "foo=new-value"},
			expected: []string{"-var", "foo=new-value"},
		},
		{
			name:     "preserves multiple -var flags",
			args:     []string{"--orig", "orig.plan", "-var", "foo=bar", "-var", "baz=qux", "--new", "new.plan"},
			expected: []string{"-var", "foo=bar", "-var", "baz=qux"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := FilterPlanDiffFlags(tc.args)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCleanupTerraformFiles_InspectionPreservesSavedPlan(t *testing.T) {
	for _, command := range []string{"plan-diff", "generate", "output", "state", "validate", "fmt", "graph", "providers", "version"} {
		t.Run(command, func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			info := &schema.ConfigAndStacksInfo{SubCommand: command, Component: "demo", ContextPrefix: "dev"}
			path := ConstructTerraformComponentPlanfilePath(config, info)
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
	copied, cleanup, err := CopyPlanFileIfNeeded(source, component)
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
