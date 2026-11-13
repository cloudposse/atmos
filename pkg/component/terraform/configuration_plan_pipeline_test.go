package terraform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// reviewedPlanRuntime is a configuration provider that can prepare, bind plans, and resolve state.
type reviewedPlanRuntime struct {
	*MockComponentConfigurationRuntime
	state *schema.ComponentConfigurationState
}

func (reviewedPlanRuntime) PlanIdentity(context.Context, *schema.ConfigAndStacksInfo, []schema.ComponentConfigurationSource) (string, error) {
	return "provider-source-options", nil
}

func (r reviewedPlanRuntime) ResolveState(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	return r.state, nil
}

// reviewedPlanProject is an isolated workdir-enabled component whose module can change after review.
type reviewedPlanProject struct {
	root, module, workdir, state string
	config                       *schema.AtmosConfiguration
	provider                     *MockComponentConfigurationRuntime
}

func newReviewedPlanProject(t *testing.T) *reviewedPlanProject {
	t.Helper()
	root := t.TempDir()
	project := &reviewedPlanProject{root: root, module: filepath.Join(root, "module"), workdir: filepath.Join(root, "chosen"), state: filepath.Join(root, "state", "terraform.tfstate")}
	require.NoError(t, os.MkdirAll(project.module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project.module, "main.tf"), []byte("# reviewed module"), 0o644))
	project.config = &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
	project.config.Components.Terraform.BasePath = "components"
	project.provider = NewMockComponentConfigurationRuntime(gomock.NewController(t))
	state := &schema.ComponentConfigurationState{BackendType: "local", Backend: map[string]any{"path": project.state}, Workspace: "default"}
	project.config.ComponentConfigurations = reviewedPlanRuntime{MockComponentConfigurationRuntime: project.provider, state: state}
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
	require.Equal(t, map[string]any{"value": "reviewed"}, execution.preparedConfiguration.Inputs)
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
	require.Equal(t, map[string]any{"value": "reviewed"}, apply.ComponentVarsSection)
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
