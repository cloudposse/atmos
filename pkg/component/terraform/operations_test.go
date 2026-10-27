package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestOperationsKeepInjectedHostsIndependent(t *testing.T) {
	t.Parallel()

	controller := gomock.NewController(t)
	first, second := NewMockOperationServices(controller), NewMockOperationServices(controller)
	firstOperations, secondOperations := NewOperations(first), NewOperations(second)
	original := schema.ConfigAndStacksInfo{
		ComponentFromArg: "app", Stack: "dev", Command: "tofu", SubCommand: "plan-diff",
		AdditionalArgsAndFlags: []string{"--orig", "old.plan", "-var", "revision=next"},
	}
	writeResult := func(result string) func(schema.ConfigAndStacksInfo, ...component.ExecutionOption) error {
		return func(info schema.ConfigAndStacksInfo, options ...component.ExecutionOption) error {
			assert.Equal(t, original.ComponentFromArg, info.ComponentFromArg)
			assert.Equal(t, original.Stack, info.Stack)
			assert.Equal(t, original.Command, info.Command)
			assert.Equal(t, "show", info.SubCommand)
			assert.Equal(t, []string{"-json", "current.plan"}, info.AdditionalArgsAndFlags)
			var execution component.ExecutionOptions
			for _, option := range options {
				option(&execution)
			}
			require.NotNil(t, execution.StdoutOverride)
			_, err := execution.StdoutOverride.Write([]byte(result))
			return err
		}
	}
	gomock.InOrder(
		first.EXPECT().ExecuteTerraform(gomock.Any(), gomock.Any()).DoAndReturn(writeResult("first")),
		second.EXPECT().ExecuteTerraform(gomock.Any(), gomock.Any()).DoAndReturn(writeResult("second")),
		first.EXPECT().ExecuteTerraform(gomock.Any(), gomock.Any()).DoAndReturn(writeResult("first-again")),
	)
	for _, test := range []struct {
		operations *Operations
		want       string
	}{{firstOperations, "first"}, {secondOperations, "second"}, {firstOperations, "first-again"}} {
		info := original
		output, err := test.operations.runTerraformShow(&info, "current.plan")
		require.NoError(t, err)
		assert.Equal(t, test.want, output)
		assert.Equal(t, original, info, "show must not rewrite the caller's command or arguments")
	}
}

func TestOperationsGeneratePlanUsesInitThenPlan(t *testing.T) {
	t.Parallel()

	host := NewMockOperationServices(gomock.NewController(t))
	operations := NewOperations(host)
	config := &schema.AtmosConfiguration{}
	workingDir, temporaryDir := t.TempDir(), t.TempDir()
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "app", Stack: "dev", SubCommand: "plan-diff", Command: "tofu",
		AdditionalArgsAndFlags: []string{"--orig", "old.plan", "--new=next.plan", "-var", "revision=next", "-input=false"},
		ComponentEnvList:       []string{"TF_VAR_region=test"},
	}
	originalArgs := append([]string{}, info.AdditionalArgsAndFlags...)
	planPath := filepath.Join(temporaryDir, "new.plan")
	gomock.InOrder(
		host.EXPECT().ExecuteTerraform(gomock.Any()).DoAndReturn(func(actual schema.ConfigAndStacksInfo, _ ...component.ExecutionOption) error {
			assert.Equal(t, "init", actual.SubCommand)
			assert.Empty(t, actual.AdditionalArgsAndFlags)
			assert.Equal(t, info.ComponentEnvList, actual.ComponentEnvList)
			return nil
		}),
		host.EXPECT().ExecuteTerraform(gomock.Any()).DoAndReturn(func(actual schema.ConfigAndStacksInfo, _ ...component.ExecutionOption) error {
			assert.Equal(t, "plan", actual.SubCommand)
			assert.Equal(t, []string{"-var", "revision=next", "-input=false", "-out=" + planPath}, actual.AdditionalArgsAndFlags)
			assert.Equal(t, info.ComponentFromArg, actual.ComponentFromArg)
			assert.Equal(t, info.Stack, actual.Stack)
			assert.Equal(t, info.Command, actual.Command)
			assert.Equal(t, info.ComponentEnvList, actual.ComponentEnvList)
			assert.True(t, actual.ProcessTemplates)
			assert.True(t, actual.ProcessFunctions)
			return nil
		}),
	)
	result, err := operations.GenerateNewPlanFile(config, &info, workingDir, temporaryDir)
	require.NoError(t, err)
	assert.Equal(t, planPath, result)
	assert.Equal(t, "plan-diff", info.SubCommand)
	assert.Equal(t, originalArgs, info.AdditionalArgsAndFlags)
	assert.False(t, info.ProcessTemplates)
	assert.False(t, info.ProcessFunctions)
}

func TestOperationsKeepExecutionErrors(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		command  string
		err      error
		skipInit bool
	}{
		{name: "init prevents plan", command: "init", err: errUtils.ErrComponentExecutionFailed},
		{name: "plan error", command: "plan", err: errUtils.ErrComponentExecutionFailed, skipInit: true},
		{name: "detailed plan exit", command: "plan", err: errUtils.ErrPlanHasDiff, skipInit: true},
		{name: "show error", command: "show", err: errUtils.ErrComponentExecutionFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			host := NewMockOperationServices(gomock.NewController(t))
			operations := NewOperations(host)
			info := schema.ConfigAndStacksInfo{SubCommand: "plan-diff", SkipInit: test.skipInit}
			var err error
			if test.command == "show" {
				host.EXPECT().ExecuteTerraform(gomock.Any(), gomock.Any()).Return(test.err)
				_, err = operations.runTerraformShow(&info, "current.plan")
			} else {
				host.EXPECT().ExecuteTerraform(gomock.Any()).DoAndReturn(func(actual schema.ConfigAndStacksInfo, _ ...component.ExecutionOption) error {
					assert.Equal(t, test.command, actual.SubCommand)
					return test.err
				})
				_, err = operations.GenerateNewPlanFile(&schema.AtmosConfiguration{}, &info, t.TempDir(), t.TempDir())
			}
			require.ErrorIs(t, err, test.err)
		})
	}
}

func TestOperationsGenerateArtifactsInOrder(t *testing.T) {
	t.Parallel()

	host := NewMockOperationServices(gomock.NewController(t))
	operations := NewOperations(host)
	config, info := operationArtifactConfig()
	workingDir := t.TempDir()
	host.EXPECT().GenerateFilesForComponent(config, info, workingDir).DoAndReturn(func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, directory string) error {
		require.FileExists(t, filepath.Join(directory, "backend.tf.json"), "backend must precede custom generation")
		require.NoFileExists(t, filepath.Join(directory, "providers_override.tf.json"))
		require.NoFileExists(t, filepath.Join(directory, "terraform_override.tf.json"))
		return os.WriteFile(filepath.Join(directory, "custom.tf"), []byte("# generated by host\n"), 0o600)
	})
	require.NoError(t, operations.GenerateConfigFiles(config, info, workingDir))
	backend, err := os.ReadFile(filepath.Join(workingDir, "backend.tf.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"terraform":{"backend":{"local":{"path":"component.tfstate"}}}}`, string(backend))
	providers, err := os.ReadFile(filepath.Join(workingDir, "providers_override.tf.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"provider":{"aws":{"region":"us-east-1"}}}`, string(providers))
	versions, err := os.ReadFile(filepath.Join(workingDir, "terraform_override.tf.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"terraform":{"required_version":">= 1.0.0"}}`, string(versions))
	require.FileExists(t, filepath.Join(workingDir, "custom.tf"))
	require.NoDirExists(t, filepath.Join(workingDir, ".terraform"), "generation must not initialize the tool")
}

func TestOperationsArtifactFailureStopsLaterStages(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{"backend", "custom", "providers", "required providers"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			host := NewMockOperationServices(gomock.NewController(t))
			operations := NewOperations(host)
			config, info := operationArtifactConfig()
			workingDir := t.TempDir()
			blockedFile := map[string]string{
				"backend": "backend.tf.json", "providers": "providers_override.tf.json", "required providers": "terraform_override.tf.json",
			}[stage]
			if blockedFile != "" {
				require.NoError(t, os.Mkdir(filepath.Join(workingDir, blockedFile), 0o700))
			}
			if stage != "backend" {
				host.EXPECT().GenerateFilesForComponent(config, info, workingDir).DoAndReturn(func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, directory string) error {
					require.FileExists(t, filepath.Join(directory, "backend.tf.json"))
					if stage == "custom" {
						return errUtils.ErrComponentArtifactGeneration
					}
					return nil
				})
			}
			err := operations.GenerateConfigFiles(config, info, workingDir)
			require.Error(t, err)
			if stage == "custom" {
				require.ErrorIs(t, err, errUtils.ErrComponentArtifactGeneration)
			} else {
				var pathError *os.PathError
				require.True(t, errors.As(err, &pathError), "filesystem failure must retain its cause: %v", err)
			}
			if stage == "backend" || stage == "custom" {
				require.NoFileExists(t, filepath.Join(workingDir, "providers_override.tf.json"))
			}
			if stage != "required providers" {
				require.NoFileExists(t, filepath.Join(workingDir, "terraform_override.tf.json"))
			} else {
				require.FileExists(t, filepath.Join(workingDir, "providers_override.tf.json"), "provider output must precede required-provider generation")
			}
		})
	}
}

func operationArtifactConfig() (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	return config, &schema.ConfigAndStacksInfo{
		ComponentBackendType: "local", ComponentBackendSection: map[string]any{"path": "component.tfstate"},
		ComponentProvidersSection: map[string]any{"aws": map[string]any{"region": "us-east-1"}},
		RequiredVersion:           ">= 1.0.0",
	}
}
