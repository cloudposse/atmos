package terraform

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=../provider.go -destination=mock_component_provider_test.go -package=terraform

import (
	bytes "bytes"
	context "context"
	errors "errors"
	os "os"
	filepath "path/filepath"
	testing "testing"

	errUtils "github.com/cloudposse/atmos/errors"
	component "github.com/cloudposse/atmos/pkg/component"
	log "github.com/cloudposse/atmos/pkg/logger"
	provisioner "github.com/cloudposse/atmos/pkg/provisioner"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	schema "github.com/cloudposse/atmos/pkg/schema"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func TestProviderInventory(t *testing.T) {
	p := New(nil)
	assert.Equal(t, "terraform", p.GetType())
	assert.Equal(t, "Infrastructure as Code", p.GetGroup())
	assert.Equal(t, "components/terraform", p.GetBasePath(nil))
	config := &schema.AtmosConfiguration{}
	assert.Equal(t, "components/terraform", p.GetBasePath(config))
	config.Components.Terraform.BasePath = "infra/modules"
	assert.Equal(t, "infra/modules", p.GetBasePath(config))
	commands := p.GetAvailableCommands()
	assert.Contains(t, commands, "plan")
	assert.Contains(t, commands, "workspace")
	commands[0] = "modified"
	assert.Equal(t, "apply", p.GetAvailableCommands()[0])
	for _, stack := range []map[string]any{nil, {}, {"components": "invalid"}, {"components": map[string]any{"helm": map[string]any{"release": nil}}}} {
		names, err := p.ListComponents(t.Context(), "dev", stack)
		require.NoError(t, err)
		assert.Empty(t, names)
	}
	names, err := p.ListComponents(t.Context(), "dev", map[string]any{"components": map[string]any{"terraform": map[string]any{"network": nil, "app": nil}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"app", "network"}, names)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = p.ListComponents(ctx, "dev", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestValidateBackendType(t *testing.T) {
	p := New(nil)
	for _, config := range []map[string]any{nil, {}, {"backend_type": nil}, {"backend_type": "s3"}} {
		require.NoError(t, p.ValidateComponent(config))
	}
	require.ErrorIs(t, p.ValidateComponent(map[string]any{"backend_type": 42}), errUtils.ErrComponentConfigInvalid)
}

func TestProviderExecution(t *testing.T) {
	runtime := NewMockRuntime(gomock.NewController(t))
	p := New(runtime)
	request := &component.ExecutionContext{Context: t.Context(), Component: "app", Stack: "dev", SubCommand: "plan"}
	// Detailed-exit-code and cancellation sentinels must retain their identity.
	for _, result := range []error{nil, errUtils.ErrPlanHasDiff, context.Canceled} {
		runtime.EXPECT().Execute(gomock.Eq(request)).Return(result)
		require.ErrorIs(t, p.Execute(request), result)
	}
	request.AtmosConfig, request.WorkingDir = &schema.AtmosConfiguration{}, t.TempDir()
	runtime.EXPECT().GenerateArtifacts(gomock.Eq(request)).Return(nil)
	require.NoError(t, p.GenerateArtifacts(request))
	runtime.EXPECT().GenerateArtifacts(gomock.Eq(request)).Return(errUtils.ErrCreateDirectory)
	require.ErrorIs(t, p.GenerateArtifacts(request), errUtils.ErrCreateDirectory)
}

func TestProviderRejectsIncompleteRequests(t *testing.T) {
	runtime := NewMockRuntime(gomock.NewController(t)) // No calls are allowed on rejected requests.
	p := New(runtime)
	for _, operation := range []func(*component.ExecutionContext) error{p.Execute, p.GenerateArtifacts} {
		require.ErrorIs(t, operation(nil), errUtils.ErrComponentConfigInvalid)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, operation(&component.ExecutionContext{Context: ctx}), context.Canceled)
	}
	require.ErrorIs(t, New(nil).Execute(&component.ExecutionContext{}), errUtils.ErrComponentExecutionFailed)
	require.ErrorIs(t, p.GenerateArtifacts(&component.ExecutionContext{}), errUtils.ErrComponentConfigInvalid)
	require.ErrorIs(t, p.GenerateArtifacts(&component.ExecutionContext{AtmosConfig: &schema.AtmosConfiguration{}}), errUtils.ErrComponentConfigInvalid)
}

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

func TestAutoGenerationPreservesDirectoryAndFailureContract(t *testing.T) {
	for _, mode := range []string{"disabled", "dry run", "no generation", "generated", "generator failure", "directory failure"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "generated")
			config := &schema.AtmosConfiguration{}
			config.Components.Terraform.AutoGenerateFiles = mode != "disabled"
			info := &schema.ConfigAndStacksInfo{DryRun: mode == "dry run", ComponentSection: map[string]any{"generate": map[string]any{"main.tf": "content"}}}
			if mode == "no generation" {
				info.ComponentSection = nil
			}
			if mode == "directory failure" {
				require.NoError(t, os.WriteFile(path, []byte("obstacle"), 0o600))
			}
			sentinel := errors.New("generation failed")
			executor := NewExecutor(Services{GenerateFilesForComponent: func(actual *schema.AtmosConfiguration, actualInfo *schema.ConfigAndStacksInfo, actualPath string) error {
				require.Same(t, config, actual)
				require.Same(t, info, actualInfo)
				require.Equal(t, path, actualPath)
				require.DirExists(t, path)
				if mode == "generator failure" {
					return sentinel
				}
				return os.WriteFile(filepath.Join(actualPath, "main.tf"), []byte("generated"), 0o600)
			}})
			err := executor.autoGenerateComponentFiles(config, info, path)
			switch mode {
			case "generator failure":
				require.ErrorIs(t, err, sentinel)
			case "directory failure":
				require.ErrorIs(t, err, errUtils.ErrCreateDirectory)
				var pathErr *os.PathError
				require.ErrorAs(t, err, &pathErr)
			default:
				require.NoError(t, err)
			}
			if mode == "generated" {
				content, readErr := os.ReadFile(filepath.Join(path, "main.tf"))
				require.NoError(t, readErr)
				require.Equal(t, "generated", string(content))
			} else {
				require.NoFileExists(t, filepath.Join(path, "main.tf"))
			}
		})
	}
}

func TestResolveProvisionedComponentPreservesFailures(t *testing.T) {
	for _, mode := range []string{"provision failure", "generation failure", "missing component", "nil context"} {
		t.Run(mode, func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			config.Components.Terraform.BasePath = "components"
			config.Components.Terraform.AutoGenerateFiles = mode == "generation failure"
			info := &schema.ConfigAndStacksInfo{ComponentFromArg: "alias", FinalComponent: "module", ComponentSection: map[string]any{"generate": map[string]any{"main.tf": "content"}}}
			sentinel := errors.New("provision or generate failed")
			original := provisionAndResolveTerraformComponentPath
			t.Cleanup(func() { provisionAndResolveTerraformComponentPath = original })
			var expectedPath string
			provisionAndResolveTerraformComponentPath = func(ctx context.Context, writers provisioner.OutputWriters, ac *schema.AtmosConfiguration, actual *schema.ConfigAndStacksInfo, kind, path string) (string, bool, error) {
				require.NotNil(t, ctx)
				_, hasDeadline := ctx.Deadline()
				require.True(t, hasDeadline)
				require.Same(t, config, ac)
				require.Same(t, info, actual)
				require.Equal(t, "terraform", kind)
				expectedPath = path
				if mode == "provision failure" {
					return "", false, sentinel
				}
				return path, mode != "missing component", nil
			}
			executor := NewExecutor(Services{GenerateFilesForComponent: func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string) error { return sentinel }})
			path, err := executor.resolveAndProvisionComponentPath(nil, provisioner.OutputWriters{}, config, info) //nolint:staticcheck // Verify the nil-context fallback creates a bounded provisioning context.
			switch mode {
			case "nil context":
				require.NoError(t, err)
				require.Equal(t, expectedPath, path)
			case "missing component":
				require.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponent)
				require.Contains(t, err.Error(), "alias")
				require.Contains(t, err.Error(), "module")
				require.Empty(t, path)
			default:
				require.ErrorIs(t, err, sentinel)
				require.Empty(t, path)
			}
		})
	}
}

func TestValidationServicePreservesResultAndError(t *testing.T) {
	for _, outcome := range []string{"accepted", "rejected", "error"} {
		t.Run(outcome, func(t *testing.T) {
			ac := &schema.AtmosConfiguration{}
			info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", ComponentSection: map[string]any{"vars": map[string]any{"stage": "dev"}}}
			sentinel := errors.New("schema unavailable")
			executor := NewExecutor(Services{ValidateComponent: func(config *schema.AtmosConfiguration, name string, section map[string]any, path, kind string, modules []string, timeout int) (bool, error) {
				require.Same(t, ac, config)
				require.Equal(t, info.ComponentFromArg, name)
				require.Equal(t, info.ComponentSection, section)
				require.Empty(t, path)
				require.Empty(t, kind)
				require.Nil(t, modules)
				require.Zero(t, timeout)
				if outcome == "error" {
					return false, sentinel
				}
				return outcome == "accepted", nil
			}})
			err := executor.validateTerraformComponent(ac, info)
			switch outcome {
			case "accepted":
				require.NoError(t, err)
			case "rejected":
				require.ErrorIs(t, err, errUtils.ErrInvalidComponent)
				require.Contains(t, err.Error(), "app")
			case "error":
				require.ErrorIs(t, err, sentinel)
			}
		})
	}
}

func TestConflictingEnvironmentWarningNamesVariablesWithoutValues(t *testing.T) {
	t.Setenv("TF_WORKSPACE", "private-workspace-value")
	t.Setenv("TF_VAR_db_password", "secret-password-value")
	t.Setenv("TF_CLI_ARGS_plan", "private-cli-value")
	var output bytes.Buffer
	logger := log.Default()
	originalLevel := logger.GetLevel()
	logger.SetLevel(log.WarnLevel)
	logger.SetOutput(&output)
	t.Cleanup(func() { logger.SetLevel(originalLevel); logger.SetOutput(os.Stderr) })
	warnOnConflictingEnvVars()
	require.Contains(t, output.String(), "TF_WORKSPACE")
	require.Contains(t, output.String(), "TF_VAR_db_password")
	require.Contains(t, output.String(), "TF_CLI_ARGS_plan")
	require.NotContains(t, output.String(), "private-workspace-value")
	require.NotContains(t, output.String(), "secret-password-value")
	require.NotContains(t, output.String(), "private-cli-value")
}

func TestFinalizeVerification(t *testing.T) {
	t.Parallel()

	freshPlan := filepath.Join(t.TempDir(), "fresh.tfplan")

	t.Run("drift + fail returns verification error", func(t *testing.T) {
		t.Parallel()
		info := &schema.ConfigAndStacksInfo{}
		err := finalizeVerification(info, freshPlan, "  ~ resource changed", true, schema.PlanfileVerifyFail)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
		assert.Contains(t, err.Error(), "resource changed")
		// On failure the fresh plan is NOT selected for apply.
		assert.Empty(t, info.PlanFile)
		assert.False(t, info.UseTerraformPlan)
	})

	t.Run("drift + warn proceeds with the fresh plan", func(t *testing.T) {
		t.Parallel()
		info := &schema.ConfigAndStacksInfo{}
		err := finalizeVerification(info, freshPlan, "  ~ resource changed", true, schema.PlanfileVerifyWarn)
		require.NoError(t, err)
		assert.Equal(t, freshPlan, info.PlanFile)
		assert.True(t, info.UseTerraformPlan)
	})

	t.Run("no drift selects the fresh plan", func(t *testing.T) {
		t.Parallel()
		info := &schema.ConfigAndStacksInfo{}
		err := finalizeVerification(info, freshPlan, "", false, schema.PlanfileVerifyFail)
		require.NoError(t, err)
		assert.Equal(t, freshPlan, info.PlanFile)
		assert.True(t, info.UseTerraformPlan)
	})
}

func TestScheduledTerraformUsesRegistryAndCapturesOutput(t *testing.T) {
	provider := replaceTerraformProvider(t)
	var stdout, stderr bytes.Buffer
	provider.EXPECT().Execute(gomock.Any()).DoAndReturn(func(request *component.ExecutionContext) error {
		require.Same(t, t.Context(), request.Context)
		assert.Equal(t, "app", request.ConfigAndStacksInfo.ComponentFromArg)
		var options component.ExecutionOptions
		for _, option := range request.Options {
			option(&options)
		}
		require.Same(t, &stdout, options.Streams.Stdout)
		require.Same(t, &stderr, options.Streams.Stderr)
		_, err := options.StdoutCapture.Write([]byte("plan output"))
		require.NoError(t, err)
		_, err = options.StderrCapture.Write([]byte("diagnostic"))
		require.NoError(t, err)
		return errUtils.ErrPlanHasDiff
	})
	result, err := testExecutor(t).executeTerraformQueryComponent(scheduleradapters.TerraformExecution{
		Context: t.Context(), Info: schema.ConfigAndStacksInfo{ComponentFromArg: "app", SubCommand: "plan"},
		CaptureOutput: true, Stdout: &stdout, Stderr: &stderr,
	})
	require.ErrorIs(t, err, errUtils.ErrPlanHasDiff)
	assert.Equal(t, "plan output", result.Stdout)
	assert.Equal(t, "diagnostic", result.Stderr)
}

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
