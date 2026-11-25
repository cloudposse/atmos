package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	tf "github.com/cloudposse/atmos/pkg/terraform"
)

// Compile-time guard for the reserved ownership marker the test relies on.
var _ = schema.ConfigurationOwnedKey

// ownedUnitInfo is a component a configuration provider owns, with the placeholder backend discovery gives it.
func ownedUnitInfo(workdir string) schema.ConfigAndStacksInfo {
	return schema.ConfigAndStacksInfo{
		ComponentFromArg:         "network",
		Stack:                    "live/dev",
		FinalComponent:           "network",
		ComponentBackendType:     "local",
		ComponentBackendSection:  map[string]any{},
		ComponentSettingsSection: map[string]any{"configuration": map[string]any{"provider": "terragrunt", schema.ConfigurationOwnedKey: true}},
		ComponentSection:         map[string]any{provWorkdir.WorkdirPathKey: workdir},
	}
}

// A unit whose backend a configuration provider owns gets no backend.tf.json: the provider writes the
// unit's own backend when the unit runs, and a second backend file would make Terraform fail with
// "Duplicate backend configuration".
func TestExecuteGenerateBackendSkipsProviderOwnedComponents(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "backend.tf"), []byte("terraform {\n  backend \"local\" {}\n}\n"), 0o600))
	host := NewMockOperationServices(gomock.NewController(t))
	host.EXPECT().ProcessStacks(gomock.Any(), gomock.Any(), true, true, true, gomock.Any(), nil).Return(ownedUnitInfo(workdir), nil)

	err := NewOperations(host).ExecuteGenerateBackend(&GenerateBackendOptions{Component: "network", Stack: "live/dev", ProcessingOptions: tf.ProcessingOptions{ProcessTemplates: true, ProcessFunctions: true}}, &schema.AtmosConfiguration{})

	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(workdir, "backend.tf.json"))
	assert.FileExists(t, filepath.Join(workdir, "backend.tf"), "the provider's backend file is left alone")
}

// The ownership guard is the only change: a component no provider owns still gets its backend file.
func TestExecuteGenerateBackendWritesUnownedComponents(t *testing.T) {
	workdir := t.TempDir()
	info := ownedUnitInfo(workdir)
	info.ComponentSettingsSection = map[string]any{"configuration": map[string]any{"note": "user data"}}
	info.ComponentBackendSection = map[string]any{"path": "terraform.tfstate"}
	host := NewMockOperationServices(gomock.NewController(t))
	host.EXPECT().ProcessStacks(gomock.Any(), gomock.Any(), true, true, true, gomock.Any(), nil).Return(info, nil)
	t.Chdir(workdir)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "main.tf"), []byte("# component\n"), 0o600))

	err := NewOperations(host).ExecuteGenerateBackend(&GenerateBackendOptions{Component: "network", Stack: "live/dev", ProcessingOptions: tf.ProcessingOptions{ProcessTemplates: true, ProcessFunctions: true}}, &schema.AtmosConfiguration{})

	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(workdir, "backend.tf.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "terraform.tfstate")
}

// Bulk backend generation skips provider-owned units too, before it renders them: their backend in
// stack configuration is only a discovery placeholder.
func TestGenerateComponentBackendSkipsProviderOwnedComponents(t *testing.T) {
	section := map[string]any{
		"backend_type": "local",
		"backend":      map[string]any{},
		"settings":     map[string]any{"configuration": map[string]any{schema.ConfigurationOwnedKey: true}},
	}

	// No host is configured: any rendering or write would fail the call.
	err := (&Operations{}).generateComponentBackend(&generateRun{atmosConfig: &schema.AtmosConfiguration{}, processed: map[string]any{}}, "live/dev", "network", section)

	require.NoError(t, err)
}
