package output

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveConfigurationStateRejectsProviderWithoutStateOrError(t *testing.T) {
	config := validAtmosConfig(t)
	sections := validSections()
	sections["settings"] = map[string]any{"configuration": true}
	config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
		return nil, nil
	}}
	state, err := ResolveConfigurationState(t.Context(), config, &ConfigurationStateRequest{Sections: sections, Component: "network", Stack: "live"})
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol, "a provider that owns a component must always identify its state")
	require.Nil(t, state)
}

func TestSetupEnvironmentFiltersProhibitedVariablesFromBaseEnvironment(t *testing.T) {
	setup := &defaultEnvironmentSetup{}
	result, err := setup.SetupEnvironment(&ComponentConfig{BaseEnvironment: []string{
		"KEEP=value", "WITH_EQUALS=a=b", "malformed-entry-without-separator",
		"TF_INPUT=0", "TF_WORKSPACE=other", "TF_LOG=trace",
		"TF_VAR_region=us-east-1", "TF_CLI_ARGS_plan=-lock=false", "TF_CLI_ARGS=-no-color",
	}}, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"KEEP": "value", "WITH_EQUALS": "a=b"}, result, "only terraform-exec owned variables are removed")
}

func TestSetupEnvironmentBaseEnvironmentReplacesAmbientEnvironment(t *testing.T) {
	t.Setenv("AMBIENT_ONLY", "must-not-leak")
	result, err := (&defaultEnvironmentSetup{}).SetupEnvironment(&ComponentConfig{BaseEnvironment: []string{}}, nil)
	require.NoError(t, err)
	assert.NotContains(t, result, "AMBIENT_ONLY", "an explicitly empty sanitized base is authoritative")
	assert.Empty(t, result)
}

func TestPluginOutputFailsBeforeRunnerWhenScratchDirectoryCannotBeCreated(t *testing.T) {
	config := validAtmosConfig(t)
	stateRoot := t.TempDir()
	config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
		return &schema.ComponentConfigurationState{BackendType: "local", Backend: map[string]any{"path": filepath.Join(stateRoot, "default.tfstate")}, Workspace: "default", WorkingDirectory: stateRoot}, nil
	}}
	sections := validSections()
	sections["settings"] = map[string]any{"configuration": true}
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("TMPDIR", missing)
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)
	runnerCreated := false
	executor := NewExecutor(nil, WithRunnerFactory(func(string, string) (TerraformRunner, error) {
		runnerCreated = true
		return nil, nil
	}))
	result, err := executor.execute(t.Context(), config, "network", "live", sections, nil, &OutputOptions{SkipInit: true}, true)
	require.Error(t, err)
	require.Nil(t, result)
	require.False(t, runnerCreated, "no Terraform runner may start without its isolated backend directory")
	entries, readErr := os.ReadDir(stateRoot)
	require.NoError(t, readErr)
	require.Empty(t, entries, "the plugin's state directory is never used as scratch space")
}
