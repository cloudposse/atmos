package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// reviewedPlanProject is an isolated workdir-enabled component whose module can change after review.
type reviewedPlanProject struct {
	root, module, workdir, state string
	config                       *schema.AtmosConfiguration
	provider                     *MockReviewedPlanRuntime
}

func TestMissingPlanDigestStopsBeforeAfterHooks(t *testing.T) {
	project := newReviewedPlanProject(t)
	info := project.info(t, "plan")
	project.provider.EXPECT().Prepare(gomock.Any(), info).Return(project.prepared(), nil)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, info, false)
	require.NoError(t, err)
	info.ComponentExecution = &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{execHook("after", "plan")}}
	planPath := filepath.Join(execution.workingDir, "missing.plan")
	snapshotter := &planSnapshotter{ctx: t.Context(), config: project.config, info: info, execCtx: execution, pathFrom: func([]string) string { return planPath }}
	phase := &executionPhase{command: "plan", args: []string{"plan", "-out=" + planPath}, insertAt: 1, componentPath: execution.workingDir}
	shell := newShellFixture(t, nil)
	err = hookExecutor(shell).runExecutionPhase(project.config, info, phase, nil, func(_ *schema.ConfigAndStacksInfo, args []string, _ []component.ExecutionOption) error {
		return snapshotter.around(args, func() error { return nil }, failedOnError)()
	})
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, snapshotter.err, errUtils.ErrPlanVerificationFailed)
	require.Nil(t, snapshotter.result)
	require.Empty(t, shell.calls, "after hooks must not run when the plan cannot be fingerprinted")

	commandErr := errors.New("terraform failed")
	err = snapshotter.around(phase.args, func() error { return commandErr }, failedOnError)()
	require.ErrorIs(t, err, commandErr, "an ordinary command failure must not be masked by a missing plan")

	err = hookExecutor(shell).runExecutionPhase(project.config, info, phase, nil, func(_ *schema.ConfigAndStacksInfo, args []string, _ []component.ExecutionOption) error {
		return snapshotter.around(args, func() error { return os.WriteFile(planPath, []byte("valid plan"), 0o600) }, failedOnError)()
	})
	require.NoError(t, err)
	require.Len(t, shell.calls, 1, "a successful fingerprint permits the after hook")
}

// failedOnError classifies every error as a failed command.
func failedOnError(err error) bool { return err != nil }

// The snapshotter uses the phase's classification of the result: a "changes present" status the
// phase accepts (for example -detailed-exitcode from extra_arguments' TF_CLI_ARGS_plan) still binds
// the plan, and a run that kept no snapshot is never replaced by a later observation.
func TestPlanSnapshotUsesThePhaseClassification(t *testing.T) {
	project := newReviewedPlanProject(t)
	info := project.info(t, "plan")
	project.provider.EXPECT().Prepare(gomock.Any(), info).Return(project.prepared(), nil)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, info, false)
	require.NoError(t, err)
	planPath := filepath.Join(execution.workingDir, "reviewed.plan")
	snapshotter := &planSnapshotter{ctx: t.Context(), config: project.config, info: info, execCtx: execution, pathFrom: func([]string) string { return planPath }}
	execution.planSnapshots = snapshotter
	changes := errUtils.ExitCodeError{Code: 2}
	acceptsChanges := func(err error) bool { return err != nil && !errors.Is(err, changes) }
	write := func() error {
		require.NoError(t, os.WriteFile(planPath, []byte("plan with changes"), 0o600))
		return changes
	}
	err = snapshotter.around([]string{"plan"}, write, acceptsChanges)()
	require.ErrorIs(t, err, changes, "the status still reaches the caller")
	require.NoError(t, snapshotter.err)
	require.NotNil(t, snapshotter.result, "a status the phase accepts binds the plan")
	require.NotEmpty(t, snapshotter.result.planDigest)

	// A genuine failure keeps no snapshot, and the binding does not fall back to a later observation.
	err = snapshotter.around([]string{"plan"}, write, failedOnError)()
	require.ErrorIs(t, err, changes)
	require.Nil(t, snapshotter.result)
	_, err = execution.reviewedSnapshot(t.Context(), project.config, info, planPath)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
}

func newReviewedPlanProject(t *testing.T) *reviewedPlanProject {
	t.Helper()
	root := t.TempDir()
	project := &reviewedPlanProject{root: root, module: filepath.Join(root, "module"), workdir: filepath.Join(root, "chosen"), state: filepath.Join(root, "state", "terraform.tfstate")}
	require.NoError(t, os.MkdirAll(project.module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project.module, "main.tf"), []byte("# reviewed module"), 0o644))
	project.config = &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
	project.config.Components.Terraform.BasePath = "components"
	// The provider is a generated runtime that can prepare, bind plans, and resolve state.
	project.provider = NewMockReviewedPlanRuntime(gomock.NewController(t))
	state := &schema.ComponentConfigurationState{BackendType: "local", Backend: map[string]any{"path": project.state}, Workspace: "default"}
	project.provider.EXPECT().PlanIdentity(gomock.Any(), gomock.Any(), gomock.Any()).Return("provider-source-options", nil).AnyTimes()
	project.provider.EXPECT().ResolveState(gomock.Any(), gomock.Any()).Return(state, nil).AnyTimes()
	project.config.ComponentConfigurations = project.provider
	return project
}

func (p *reviewedPlanProject) info(t *testing.T, subCommand string) *schema.ConfigAndStacksInfo {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	return &schema.ConfigAndStacksInfo{
		Stack: "dev", ComponentFromArg: "app", Component: "app", FinalComponent: "app", Command: binary, SubCommand: subCommand,
		ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvSection: map[string]any{},
		ComponentSection: map[string]any{
			"component": "app", "atmos_component": "app", "atmos_stack": "dev", "source": map[string]any{"uri": p.module},
			"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
			"metadata":  map[string]any{"working_directory": p.workdir},
		},
	}
}

func (p *reviewedPlanProject) prepared() *schema.PreparedComponentConfiguration {
	return &schema.PreparedComponentConfiguration{Engine: "terraform", Source: p.module, Inputs: map[string]any{"value": "reviewed"}, BackendType: "local", Backend: map[string]any{"path": p.state}, Workspace: "default"}
}

// review runs plan and records its binding, returning the plan file path.
func (p *reviewedPlanProject) review(t *testing.T) string {
	t.Helper()
	info := p.info(t, "plan")
	p.provider.EXPECT().Prepare(gomock.Any(), info).Return(p.prepared(), nil)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, p.config, info, false)
	require.NoError(t, err)
	require.Equal(t, p.workdir, execution.workingDir)
	planPath := filepath.Join(execution.workingDir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(planPath, []byte("reviewed plan"), 0o600))
	require.NoError(t, recordConfigurationPlan(t.Context(), p.config, info, execution, []string{"-out=" + planPath}))
	return planPath
}

// reviewedApply returns an apply invocation of the reviewed plan, as a new process would build it:
// nothing has been provisioned, so the retained workdir has to be located from configuration.
func (p *reviewedPlanProject) reviewedApply(t *testing.T, planPath string) *schema.ConfigAndStacksInfo {
	t.Helper()
	apply := p.info(t, "apply")
	apply.UseTerraformPlan, apply.PlanFile = true, planPath
	return apply
}

func TestApplyOfReviewedPlanReusesReviewedPreparationAndWorkdir(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.review(t)

	// Prepare is expected exactly once (for the plan); applying must restore the reviewed preparation instead.
	apply := project.reviewedApply(t, planPath)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.NoError(t, err)

	require.Equal(t, project.workdir, execution.componentPath, "a reviewed plan runs in the retained directory")
	require.Equal(t, execution.workingDir, execution.componentPath)
	require.Empty(t, execution.preparedConfiguration.Inputs, "the record stores no inputs; the plan file carries them")
	retained, err := os.ReadFile(filepath.Join(project.workdir, "main.tf"))
	require.NoError(t, err)
	require.Equal(t, "# reviewed module", string(retained))
	require.Equal(t, planPath, apply.PlanFile)
	require.True(t, apply.UseTerraformPlan)

	// Run the prepared invocation through the command pipeline and inspect the
	// actual subprocess arguments, rather than only the restored metadata.
	argsFile := filepath.Join(project.root, "apply-args")
	apply.SkipInit = true
	apply.ComponentBackendType = "http" // Avoid a separate workspace subprocess.
	apply.ComponentEnvList = append(apply.ComponentEnvList, "_ATMOS_TEST_ARGS_FILE="+argsFile)
	require.NoError(t, testExecutor(t).executeCommandPipeline(project.config, apply, execution))
	args, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	require.Contains(t, strings.Split(string(args), "\n"), planPath)
}

func TestSavedPlanApplySkipsSecondComponentValidation(t *testing.T) {
	project := newReviewedPlanProject(t)
	executor := testExecutor(t)
	validations := 0
	executor.services.ValidateComponent = func(_ *schema.AtmosConfiguration, _ string, section map[string]any, _, _ string, _ []string, _ int) (bool, error) {
		validations++
		vars, _ := section["vars"].(map[string]any)
		return vars["value"] == "reviewed", nil
	}
	planPath := project.review(t)
	require.Equal(t, 1, validations, "plan creation validates the component with its values")

	apply := project.reviewedApply(t, planPath)
	_, err := executor.prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.NoError(t, err, "a verified saved plan must not be rejected for its intentionally empty vars")
	require.Equal(t, 1, validations, "saved-plan apply must not revalidate the redacted component")
}

func TestDirectPluginApplyStillValidatesComponent(t *testing.T) {
	project := newReviewedPlanProject(t)
	apply := project.info(t, "apply")
	project.provider.EXPECT().Prepare(gomock.Any(), apply).Return(project.prepared(), nil)
	executor := testExecutor(t)
	validations := 0
	executor.services.ValidateComponent = func(_ *schema.AtmosConfiguration, _ string, _ map[string]any, _, _ string, _ []string, _ int) (bool, error) {
		validations++
		return false, nil
	}
	_, err := executor.prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.ErrorIs(t, err, errUtils.ErrInvalidComponent)
	require.Equal(t, 1, validations, "a direct apply has no reviewed plan to replace validation")
}

// Terragrunt evaluates the configuration again for apply, so a hook enabled only for apply is
// absent from the plan-time preparation. Applying the reviewed plan runs the apply-time hooks
// recorded with it, and not the plan-time ones.
func TestApplyOfReviewedPlanRunsApplyTimeHooks(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	hook := func(name string) schema.ComponentHook {
		return schema.ComponentHook{Name: name, Commands: []string{"apply"}, Execute: []string{exe, name}}
	}
	project := newReviewedPlanProject(t)
	reviewed := project.prepared()
	reviewed.Execution = &schema.ComponentExecution{
		AfterHooks: []schema.ComponentHook{hook("plan-time")},
		Apply:      &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{hook("notify"), hook("always")}},
	}
	info := project.info(t, "plan")
	project.provider.EXPECT().Prepare(gomock.Any(), info).Return(reviewed, nil)
	planned, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, info, false)
	require.NoError(t, err)
	planPath := filepath.Join(planned.workingDir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(planPath, []byte("reviewed plan"), 0o600))
	require.NoError(t, recordConfigurationPlan(t.Context(), project.config, info, planned, []string{"-out=" + planPath}))

	apply := project.reviewedApply(t, planPath)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.NoError(t, err)
	require.Equal(t, &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{hook("notify"), hook("always")}}, apply.ComponentExecution)

	// Terraform and the hooks are all the test binary, which records every invocation.
	log := filepath.Join(t.TempDir(), "hooks.jsonl")
	apply.SkipInit = true
	apply.ComponentBackendType = "http" // Avoid a separate workspace subprocess.
	apply.ComponentEnvList = append(apply.ComponentEnvList, testEnvHookChild+"=1", testEnvHookLog+"="+log)
	require.NoError(t, testExecutor(t).executeCommandPipeline(project.config, apply, execution))
	var ran []string
	for _, record := range readHookLog(t, log) {
		ran = append(ran, record.Arguments[0])
	}
	require.Equal(t, []string{"apply", "notify", "always"}, ran)
}

// Plan runs with the provider's module source, so its directory follows source target rules,
// including an explicit working_directory. A new apply process whose discovered section carries
// no source must locate the same retained directory without provisioning anything.
func TestApplyOfReviewedPlanFindsRetainedWorkdirWithoutDiscoveredSource(t *testing.T) {
	project := newReviewedPlanProject(t)
	plan := project.info(t, "plan")
	delete(plan.ComponentSection, "source")
	project.provider.EXPECT().Prepare(gomock.Any(), plan).Return(project.prepared(), nil)
	planned, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, plan, false)
	require.NoError(t, err)
	require.Equal(t, project.workdir, planned.workingDir)
	planPath := filepath.Join(planned.workingDir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(planPath, []byte("reviewed plan"), 0o600))
	require.NoError(t, recordConfigurationPlan(t.Context(), project.config, plan, planned, []string{"-out=" + planPath}))

	apply := project.reviewedApply(t, planPath)
	delete(apply.ComponentSection, "source")
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.NoError(t, err)
	require.Equal(t, project.workdir, execution.componentPath)
}

func TestApplyOfReviewedPlanRejectsChangedWorkdir(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.review(t)
	require.NoError(t, os.WriteFile(filepath.Join(project.workdir, "main.tf"), []byte("# edited after review"), 0o644))

	apply := project.reviewedApply(t, planPath)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.Nil(t, execution)
}

func TestPrepareConfigurationRestoresReviewedPlanWithoutPreparing(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.review(t)
	apply := project.reviewedApply(t, planPath)

	prepared, err := prepareConfiguration(t.Context(), project.config, apply)
	require.NoError(t, err)
	// The record keeps no inputs: the plan file carries the values Terraform applies.
	require.Empty(t, apply.ComponentVarsSection)
	require.NotNil(t, apply.ComponentVarsSection)
	require.Equal(t, prepared.Inputs, apply.ComponentSection["vars"])

	// A tampered plan is rejected before any of the reviewed state is applied to the invocation.
	require.NoError(t, os.WriteFile(planPath, []byte("unreviewed plan"), 0o600))
	rejected := project.reviewedApply(t, planPath)
	result, err := prepareConfiguration(t.Context(), project.config, rejected)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.Nil(t, result)
	require.Nil(t, rejected.ComponentVarsSection)
}

func TestPrepareInitExecutionKeepsReviewedPlanWorkspace(t *testing.T) {
	for _, test := range []struct {
		name         string
		owned        bool
		savedPlan    bool
		wantCleaned  bool
		wantSameRoot bool
	}{
		{"provider-owned saved plan", true, true, false, true},
		{"provider-owned without a saved plan", true, false, true, true},
		{"ordinary component with a saved plan", false, true, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			environment := filepath.Join(directory, ".terraform", "environment")
			require.NoError(t, os.MkdirAll(filepath.Dir(environment), 0o755))
			require.NoError(t, os.WriteFile(environment, []byte("workspace"), 0o644))
			info := schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}, UseTerraformPlan: test.savedPlan}
			if test.owned {
				info.ComponentSettingsSection = map[string]any{"configuration": true}
			}
			result, err := prepareInitExecution(t.Context(), provisioner.OutputWriters{}, &schema.AtmosConfiguration{}, &info, directory)
			require.NoError(t, err)
			require.Equal(t, directory, result)
			if test.wantCleaned {
				require.NoFileExists(t, environment)
			} else {
				require.FileExists(t, environment, "the reviewed plan's workspace selection must be retained")
			}
		})
	}
}

// TestPlanSnapshotterFingerprintsThePlanBeforeAfterHooksRun verifies that the plan Terraform wrote is
// fingerprinted when the subprocess returns, so an after hook that rewrites it cannot be bound.
func TestPlanSnapshotterFingerprintsThePlanBeforeAfterHooksRun(t *testing.T) {
	for _, test := range []struct {
		name       string
		afterHook  func(t *testing.T, planPath string)
		wantRecord bool
	}{
		{name: "plan unchanged by after hooks", afterHook: func(*testing.T, string) {}, wantRecord: true},
		{name: "plan rewritten by an after hook", afterHook: func(t *testing.T, planPath string) {
			require.NoError(t, os.WriteFile(planPath, []byte("rewritten by an after hook"), 0o600))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := newReviewedPlanProject(t)
			info := project.info(t, "plan")
			project.provider.EXPECT().Prepare(gomock.Any(), info).Return(project.prepared(), nil)
			execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, info, false)
			require.NoError(t, err)
			planPath := filepath.Join(execution.workingDir, "reviewed.planfile")
			snapshotter := &planSnapshotter{ctx: t.Context(), config: project.config, info: info, execCtx: execution, pathFrom: func([]string) string { return planPath }}
			execution.planSnapshots = snapshotter

			// Terraform writes the plan.
			terraform := snapshotter.around(nil, func() error { return os.WriteFile(planPath, []byte("reviewed plan"), 0o600) }, failedOnError)
			require.NoError(t, terraform())
			written, err := fileDigest(planPath)
			require.NoError(t, err)
			require.NotNil(t, snapshotter.result)
			require.Equal(t, written, snapshotter.result.planDigest, "the digest is taken when the subprocess returns")

			test.afterHook(t, planPath)
			err = recordConfigurationPlan(t.Context(), project.config, info, execution, []string{"-out=" + planPath})

			if test.wantRecord {
				require.NoError(t, err)
				require.FileExists(t, planPath+configurationPlanSuffix)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
			require.NoFileExists(t, planPath+configurationPlanSuffix, "a rewritten plan is never bound")
		})
	}
}

// TestPlanSnapshotterBindsTheEngineThatRanTerraform verifies that the engine digest is captured around
// the subprocess, so an engine replaced before the binding is written is rejected.
func TestPlanSnapshotterBindsTheEngineThatRanTerraform(t *testing.T) {
	for _, test := range []struct {
		name    string
		replace bool
	}{
		{name: "engine unchanged binds the digest of the engine that ran"},
		{name: "engine replaced after Terraform finished is rejected", replace: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := newReviewedPlanProject(t)
			info := project.info(t, "plan")
			original, err := os.ReadFile(info.Command)
			require.NoError(t, err)
			// The copy keeps the original's extension, because Windows runs only files that have one.
			engine := filepath.Join(t.TempDir(), "engine"+filepath.Ext(info.Command))
			require.NoError(t, os.WriteFile(engine, original, 0o700))
			info.Command = engine
			ran, err := fileDigest(engine)
			require.NoError(t, err)

			project.provider.EXPECT().Prepare(gomock.Any(), info).Return(project.prepared(), nil)
			execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, info, false)
			require.NoError(t, err)
			planPath := filepath.Join(execution.workingDir, "reviewed.planfile")
			snapshotter := &planSnapshotter{ctx: t.Context(), config: project.config, info: info, execCtx: execution, pathFrom: func([]string) string { return planPath }}
			execution.planSnapshots = snapshotter

			terraform := snapshotter.around(nil, func() error { return os.WriteFile(planPath, []byte("reviewed plan"), 0o600) }, failedOnError)
			require.NoError(t, terraform())
			require.NotNil(t, snapshotter.result)
			require.Equal(t, ran, snapshotter.result.engineDigest, "the engine digest is taken around the subprocess")

			if test.replace {
				require.NoError(t, os.WriteFile(engine, append(original, []byte("replaced by an after hook")...), 0o700))
			}
			err = recordConfigurationPlan(t.Context(), project.config, info, execution, []string{"-out=" + planPath})

			if test.replace {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
				require.NoFileExists(t, planPath+configurationPlanSuffix, "a replaced engine is never bound")
				return
			}
			require.NoError(t, err)
			data, err := os.ReadFile(planPath + configurationPlanSuffix)
			require.NoError(t, err)
			require.Contains(t, string(data), ran)
		})
	}
}

// identityOnlyRuntime returns a generated runtime that can bind plans but cannot resolve backend state.
func identityOnlyRuntime(t *testing.T) *MockPlanIdentityRuntime {
	t.Helper()
	runtime := NewMockPlanIdentityRuntime(gomock.NewController(t))
	runtime.EXPECT().PlanIdentity(gomock.Any(), gomock.Any(), gomock.Any()).Return("provider-source-options", nil).AnyTimes()
	return runtime
}

// stateFailureRuntime returns a generated runtime that accepts the plan identity but fails to resolve the backend.
func stateFailureRuntime(t *testing.T, fixture *planRuntimeFixture, stateErr error) *MockReviewedPlanRuntime {
	t.Helper()
	runtime := NewMockReviewedPlanRuntime(gomock.NewController(t))
	runtime.EXPECT().PlanIdentity(gomock.Any(), gomock.Any(), gomock.Any()).Return(fixture.identity, fixture.err).AnyTimes()
	runtime.EXPECT().ResolveState(gomock.Any(), gomock.Any()).Return(nil, stateErr).AnyTimes()
	return runtime
}

// recordedPlan records a plan companion and prepares the invocation for a direct apply.
func recordedPlan(t *testing.T) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, *componentExecContext, *planRuntimeFixture, string) {
	t.Helper()
	config, info, execCtx, runtime, path := planFixture(t)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out=" + path}))
	info.SubCommand = "apply"
	info.UseTerraformPlan = true
	info.PlanFile = path
	return config, info, execCtx, runtime, path
}

func TestRestoreConfigurationPlanRequiresPlanBindingRuntime(t *testing.T) {
	config, info, _, _, _ := recordedPlan(t)
	config.ComponentConfigurations = nil
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	require.Nil(t, prepared)
}

func TestRestoreConfigurationPlanRequiresStateRuntime(t *testing.T) {
	config, info, _, _, _ := recordedPlan(t)
	config.ComponentConfigurations = identityOnlyRuntime(t)
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension, "a provider that cannot prove the backend is unchanged must not apply a saved plan")
	require.Nil(t, prepared)
}

func TestRestoreConfigurationPlanBackendResolution(t *testing.T) {
	failure := errors.New("backend unreachable")
	t.Run("resolution error is propagated", func(t *testing.T) {
		config, info, _, runtime, _ := recordedPlan(t)
		config.ComponentConfigurations = stateFailureRuntime(t, runtime, failure)
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, failure)
		require.NotErrorIs(t, err, errUtils.ErrPlanVerificationFailed, "an unreachable backend is not a verification mismatch")
	})
	t.Run("missing state is a verification failure", func(t *testing.T) {
		config, info, _, runtime, _ := recordedPlan(t)
		runtime.state = nil
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	})
	t.Run("backend type change is a verification failure", func(t *testing.T) {
		config, info, _, runtime, _ := recordedPlan(t)
		runtime.state.BackendType = "s3"
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	})
}

func TestRestoreConfigurationPlanReportsUnreadableArtifacts(t *testing.T) {
	t.Run("missing engine binary", func(t *testing.T) {
		config, info, _, _, _ := recordedPlan(t)
		require.NoError(t, os.Remove(info.Command))
		prepared, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, os.ErrNotExist)
		require.Nil(t, prepared)
	})
	t.Run("symlink added to the reviewed workdir", func(t *testing.T) {
		config, info, execCtx, _, _ := recordedPlan(t)
		if err := os.Symlink(filepath.Join(execCtx.workingDir, "main.tf"), filepath.Join(execCtx.workingDir, "alias.tf")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		prepared, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
		require.Nil(t, prepared)
	})
	t.Run("unchanged inputs are accepted", func(t *testing.T) {
		config, info, execCtx, _, path := recordedPlan(t)
		prepared, err := restoreConfigurationPlan(t.Context(), config, info)
		require.NoError(t, err)
		require.Equal(t, execCtx.preparedConfiguration.Backend, prepared.Backend)
		require.Equal(t, path, info.PlanFile)
	})
}

func TestRecordConfigurationPlanFailsWhenWorkdirCannotBeDigested(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	execCtx.workingDir = filepath.Join(t.TempDir(), "absent")
	err := recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out=" + path})
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoFileExists(t, path+configurationPlanSuffix, "no binding may be written for an unverifiable workdir")
}

func TestRecordConfigurationPlanReplacesBindingAtomically(t *testing.T) {
	config, info, execCtx, runtime, path := planFixture(t)
	args := []string{"plan", "-out=" + path}
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, args))
	first, err := os.ReadFile(path + configurationPlanSuffix)
	require.NoError(t, err)
	runtime.identity = "changed provider"
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, args))
	second, err := os.ReadFile(path + configurationPlanSuffix)
	require.NoError(t, err)
	require.NotEqual(t, first, second, "re-planning must replace the earlier binding")
	require.Contains(t, string(second), "changed provider")
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotContains(t, entry.Name(), ".atmos-plan-", "temporary binding files are cleaned up")
	}
}

func TestConfigurationWorkdirDigestSkipsOnlyNestedMutableDirectories(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.plan")
	write := func(name string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(name), 0o600))
	}
	write("main.tf")
	baseline, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)

	for _, ignored := range []string{".git/HEAD", ".terragrunt-cache/x/main.tf", ".terraform/modules.json", "sub/.terraform/x", ".atmos/metadata.json", ".atmos/metadata.json.lock", "sub/.atmos/source-files.json", ".atmos-plan-temp", "terraform.tfstate.backup", "other.planfile.atmos.json"} {
		write(ignored)
	}
	same, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.Equal(t, baseline, same, "mutable data does not alter the retained-bytes digest")

	for _, retained := range []string{"modules/child/main.tf", "terragrunt.hcl", ".terraform.lock.hcl", "settings.atmos.json", ".terraform/modules/vpc/.atmos/hook.sh", ".terraform/modules/vpc/.terragrunt-cache/main.tf", ".terraform/modules/vpc/.git/hooks/post-checkout"} {
		write(retained)
		changed, err := configurationWorkdirDigest(root, plan)
		require.NoError(t, err)
		require.NotEqual(t, baseline, changed, retained)
		baseline = changed
	}
}

func TestConfigurationWorkdirDigestFailsForUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not enforced on Windows")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked.tf")
	require.NoError(t, os.WriteFile(locked, []byte("secret"), 0o600))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })
	if file, err := os.Open(locked); err == nil {
		_ = file.Close()
		t.Skip("permissions are not enforced for this user")
	}
	_, err := configurationWorkdirDigest(root, filepath.Join(root, "reviewed.plan"))
	require.ErrorIs(t, err, os.ErrPermission)
}
