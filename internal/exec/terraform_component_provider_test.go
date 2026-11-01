package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=../../pkg/component/provider.go -destination=mock_terraform_provider_test.go -package=exec

import (
	bytes "bytes"
	context "context"
	os "os"
	filepath "path/filepath"
	testing "testing"

	errUtils "github.com/cloudposse/atmos/errors"
	component "github.com/cloudposse/atmos/pkg/component"
	terraformcomponent "github.com/cloudposse/atmos/pkg/component/terraform"
	schema "github.com/cloudposse/atmos/pkg/schema"
	cobra "github.com/spf13/cobra"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func replaceTerraformProvider(t *testing.T) *MockComponentProvider {
	t.Helper()
	original, ok := component.GetProvider("terraform")
	require.True(t, ok, "the host must register the built-in Terraform provider")
	t.Cleanup(func() { require.NoError(t, component.Register(original)) })
	provider := NewMockComponentProvider(gomock.NewController(t))
	provider.EXPECT().GetType().Return("terraform")
	require.NoError(t, component.Register(provider))
	return provider
}

func TestTerraformRegistryDispatch(t *testing.T) {
	provider := replaceTerraformProvider(t)
	var stdout, stderr bytes.Buffer
	invoking := &cobra.Command{Use: "plan"}
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "dev/app", Stack: "live", SubCommand: "plan", Command: "tofu",
		AdditionalArgsAndFlags: []string{"-detailed-exitcode"},
		ComponentSection:       map[string]any{"vars": map[string]any{"region": "test"}},
		ComponentEnvList:       []string{"AUTH_TOKEN=component"}, SanitizedEnv: []string{},
	}
	provider.EXPECT().Execute(gomock.Any()).DoAndReturn(func(request *component.ExecutionContext) error {
		require.Equal(t, info, request.ConfigAndStacksInfo)
		assert.Equal(t, "terraform", request.ComponentType)
		assert.Equal(t, "dev/app", request.Component)
		assert.Equal(t, "live", request.Stack)
		assert.Equal(t, "tofu", request.Command)
		assert.Equal(t, "plan", request.SubCommand)
		assert.Equal(t, info.AdditionalArgsAndFlags, request.Args)
		require.Same(t, t.Context(), request.Context)
		var options component.ExecutionOptions
		for _, option := range request.Options {
			option(&options)
		}
		require.Same(t, &stdout, options.StdoutCapture)
		require.Same(t, &stderr, options.StderrCapture)
		require.NotNil(t, options.ProcessEnv)
		assert.Empty(t, options.ProcessEnv)
		require.Same(t, invoking, options.InvokingCmd)
		assert.Equal(t, "plan:changes", options.ExecMetadataParser("plan", 2, "changes"))
		return errUtils.ErrPlanHasDiff
	})
	err := ExecuteTerraform(info, WithProcessContext(t.Context()), WithStdoutCapture(&stdout), WithStderrCapture(&stderr),
		WithEnvironment([]string{}), WithInvokingCommand(invoking), WithExecMetadataParser(func(command string, _ int, output string) any { return command + ":" + output }))
	require.ErrorIs(t, err, errUtils.ErrPlanHasDiff)
}

func TestRegisteredTerraformCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// The actual provider must stop before config loading, authentication, or writes.
	require.ErrorIs(t, ExecuteTerraform(schema.ConfigAndStacksInfo{}, WithProcessContext(ctx)), context.Canceled)
}

func TestRegisteredTerraformGeneratesBackend(t *testing.T) {
	provider, ok := component.GetProvider("terraform")
	require.True(t, ok)
	require.IsType(t, &terraformcomponent.Provider{}, provider)
	directory := t.TempDir()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	request := &component.ExecutionContext{
		Context: t.Context(), AtmosConfig: config, WorkingDir: directory,
		ConfigAndStacksInfo: schema.ConfigAndStacksInfo{
			ComponentBackendType: "local", ComponentBackendSection: map[string]any{"path": "state.tfstate"},
		},
	}
	require.NoError(t, provider.GenerateArtifacts(request))
	content, err := os.ReadFile(filepath.Join(directory, "backend.tf.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"terraform":{"backend":{"local":{"path":"state.tfstate"}}}}`, string(content))
	_, err = os.Stat(filepath.Join(directory, ".terraform"))
	require.True(t, os.IsNotExist(err), "artifact generation must not initialize Terraform")
}

// validatePlanfileFormat delegates to the Terraform component implementation.
func validatePlanfileFormat(format *string) error {
	return terraformcomponent.ValidatePlanfileFormat(format)
}

// validateOriginalPlanFile delegates to the Terraform component implementation.
func validateOriginalPlanFile(origPlanFile string, componentPath string) (string, error) {
	return terraformcomponent.ValidateOriginalPlanFile(origPlanFile, componentPath)
}

// validateComponent delegates to the Terraform component implementation.
func validateComponent(component string) error {
	return terraformcomponent.ValidateComponent(component)
}

// runTerraformShowWithExecutor delegates to the Terraform component implementation.
func runTerraformShowWithExecutor(info *schema.ConfigAndStacksInfo, planFile string, executor terraformPlanDiffExecutor) (string, error) {
	return terraformOperations.RunTerraformShowWithExecutor(info, planFile, executor)
}

// parsePlanDiffFlags delegates to the Terraform component implementation.
func parsePlanDiffFlags(arg1 []string) (string, string, error) {
	return terraformcomponent.ParsePlanDiffFlags(arg1)
}

// filterPlanDiffFlags delegates to the Terraform component implementation.
func filterPlanDiffFlags(arg1 []string) []string { return terraformcomponent.FilterPlanDiffFlags(arg1) }

// extractJSONFromOutput delegates to the Terraform component implementation.
func extractJSONFromOutput(output string) (string, error) {
	return terraformcomponent.ExtractJSONFromOutput(output)
}

// extractBackendConfig delegates to the Terraform component implementation.
func extractBackendConfig(componentSection map[string]any) backendConfig {
	return terraformcomponent.ExtractBackendConfig(componentSection)
}

// ensureTerraformComponentExists delegates to the Terraform component implementation.
func ensureTerraformComponentExists(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	return terraformcomponent.EnsureTerraformComponentExists(atmosConfig, info)
}

// copyPlanFileIfNeeded delegates to the Terraform component implementation.
func copyPlanFileIfNeeded(planFile string, componentPath string) (string, func(), error) {
	return terraformcomponent.CopyPlanFileIfNeeded(planFile, componentPath)
}

// comparePlansAndGenerateDiff delegates to the Terraform component implementation.
func comparePlansAndGenerateDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, origPlanFile string, newPlanFile string) error {
	return terraformOperations.ComparePlansAndGenerateDiff(atmosConfig, info, componentPath, origPlanFile, newPlanFile)
}

// checkBackendTypeAfterProcessing delegates to the Terraform component implementation.
func checkBackendTypeAfterProcessing(backendType string) error {
	return terraformcomponent.CheckBackendTypeAfterProcessing(backendType)
}
