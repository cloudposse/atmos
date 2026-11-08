package terraform

import (
	"context"
	"os"
	"path/filepath"
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
	// explicitWorkdir pins the workdir with metadata.working_directory. Units use the default location.
	explicitWorkdir bool
	config          *schema.AtmosConfiguration
	provider        *MockComponentConfigurationRuntime
}

func newReviewedPlanProject(t *testing.T) *reviewedPlanProject {
	t.Helper()
	root := t.TempDir()
	project := &reviewedPlanProject{root: root, module: filepath.Join(root, "module"), workdir: filepath.Join(root, "chosen"), state: filepath.Join(root, "state", "terraform.tfstate"), explicitWorkdir: true}
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
	metadata := map[string]any{}
	if p.explicitWorkdir {
		metadata["working_directory"] = p.workdir
	}
	return &schema.ConfigAndStacksInfo{
		Stack: "dev", ComponentFromArg: "app", Component: "app", FinalComponent: "app", Command: binary, SubCommand: subCommand,
		ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvSection: map[string]any{},
		ComponentSection: map[string]any{
			"component": "app", "atmos_component": "app", "atmos_stack": "dev", "source": map[string]any{"uri": p.module},
			"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
			"metadata":  metadata,
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

// reviewInDefaultWorkdir is review for a unit: the workdir is wherever provisioning places it.
func (p *reviewedPlanProject) reviewInDefaultWorkdir(t *testing.T) string {
	t.Helper()
	p.explicitWorkdir = false
	info := p.info(t, "plan")
	p.provider.EXPECT().Prepare(gomock.Any(), info).Return(p.prepared(), nil)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, p.config, info, false)
	require.NoError(t, err)
	p.workdir = execution.workingDir
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

// freshRunner models a new runner that checked out the same repository at the same path: nothing
// of the earlier run remains except the artifacts the CI plan store restored. It returns the plan
// file and its record bytes after wiping the workdir, then recreates the workdir with only the
// restored artifacts.
func (p *reviewedPlanProject) freshRunner(t *testing.T, planPath string, restore ...string) {
	t.Helper()
	artifacts := map[string][]byte{}
	for _, name := range append([]string{planPath, planPath + configurationPlanSuffix}, restore...) {
		data, err := os.ReadFile(name)
		require.NoError(t, err)
		artifacts[name] = data
	}
	require.NoError(t, os.RemoveAll(p.workdir))
	require.NoError(t, os.MkdirAll(p.workdir, 0o755))
	for name, data := range artifacts {
		require.NoError(t, os.WriteFile(name, data, 0o600))
	}
}

// freshApply prepares an apply of the restored plan; the provider must re-evaluate exactly once.
func (p *reviewedPlanProject) freshApply(t *testing.T, planPath string, prepared *schema.PreparedComponentConfiguration) (*schema.ConfigAndStacksInfo, *componentExecContext, error) {
	t.Helper()
	apply := p.reviewedApply(t, planPath)
	cleared := *apply
	cleared.UseTerraformPlan, cleared.PlanFile = false, ""
	p.provider.EXPECT().Prepare(gomock.Any(), gomock.Cond(func(info *schema.ConfigAndStacksInfo) bool {
		return !info.UseTerraformPlan && info.PlanFile == "" && info.SubCommand == "apply"
	})).Return(prepared, nil).MaxTimes(1)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, p.config, apply, false)
	return apply, execution, err
}

func TestApplyOfRestoredPlanRebuildsWorkdirOnAFreshRunner(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewInDefaultWorkdir(t)
	project.freshRunner(t, planPath)

	apply, execution, err := project.freshApply(t, planPath, project.prepared())
	require.NoError(t, err)

	require.Equal(t, project.workdir, execution.componentPath)
	rebuilt, err := os.ReadFile(filepath.Join(project.workdir, "main.tf"))
	require.NoError(t, err)
	require.Equal(t, "# reviewed module", string(rebuilt), "the module is re-provisioned from the checkout")
	plan, err := os.ReadFile(planPath)
	require.NoError(t, err)
	require.Equal(t, "reviewed plan", string(plan), "the restored plan is kept in place for apply")
	require.FileExists(t, planPath+configurationPlanSuffix)
	require.Empty(t, execution.preparedConfiguration.Inputs, "the plan file carries the inputs")
	require.True(t, apply.UseTerraformPlan)
	require.Equal(t, planPath, apply.PlanFile)
}

// A verified rebuild whose init or apply then failed leaves the rebuilt workdir behind. A retry uses it as
// it is, because it still equals the reviewed rebuild; a change to it after the rebuild is still rejected.
func TestApplyOfRestoredPlanRetriesInAVerifiedRebuild(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewInDefaultWorkdir(t)
	project.freshRunner(t, planPath)
	_, _, err := project.freshApply(t, planPath, project.prepared())
	require.NoError(t, err, "the first attempt rebuilds the workdir")
	// The failed apply left init products behind.
	require.NoError(t, os.MkdirAll(filepath.Join(project.workdir, ".terraform", "providers"), 0o755))

	apply, execution, err := project.freshApply(t, planPath, project.prepared())
	require.NoError(t, err, "the retry accepts the verified rebuild")
	require.NotNil(t, execution)
	require.True(t, apply.UseTerraformPlan)

	require.NoError(t, os.WriteFile(filepath.Join(project.workdir, "main.tf"), []byte("# changed after the rebuild"), 0o644))
	_, execution, err = project.freshApply(t, planPath, project.prepared())
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.Nil(t, execution)
}

func TestApplyOfRestoredPlanRejectsWhatCannotBeReproduced(t *testing.T) {
	tests := []struct {
		name   string
		want   error
		change func(t *testing.T, project *reviewedPlanProject, planPath string)
	}{
		{"module source revision changed", errUtils.ErrPlanVerificationFailed, func(t *testing.T, project *reviewedPlanProject, _ string) {
			require.NoError(t, os.WriteFile(filepath.Join(project.module, "main.tf"), []byte("# another revision"), 0o644))
		}},
		{"extra file in the module", errUtils.ErrPlanVerificationFailed, func(t *testing.T, project *reviewedPlanProject, _ string) {
			require.NoError(t, os.WriteFile(filepath.Join(project.module, "extra.tf"), []byte("# added"), 0o644))
		}},
		{"plan bytes changed", errUtils.ErrPlanVerificationFailed, func(t *testing.T, _ *reviewedPlanProject, planPath string) {
			require.NoError(t, os.WriteFile(planPath, []byte("unreviewed plan"), 0o600))
		}},
		{"binding missing", errUtils.ErrSavedPlanNotFound, func(t *testing.T, _ *reviewedPlanProject, planPath string) {
			require.NoError(t, os.Remove(planPath+configurationPlanSuffix))
		}},
		{"workdir holds unverifiable content", errUtils.ErrPlanVerificationFailed, func(t *testing.T, project *reviewedPlanProject, _ string) {
			require.NoError(t, os.WriteFile(filepath.Join(project.workdir, "main.tf"), []byte("# stale"), 0o644))
		}},
		{"lock file differs from the reviewed one", errUtils.ErrPlanVerificationFailed, func(t *testing.T, project *reviewedPlanProject, _ string) {
			require.NoError(t, os.WriteFile(filepath.Join(project.workdir, ".terraform.lock.hcl"), []byte("# unreviewed providers"), 0o644))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := newReviewedPlanProject(t)
			planPath := project.reviewInDefaultWorkdir(t)
			project.freshRunner(t, planPath)
			test.change(t, project, planPath)

			_, execution, err := project.freshApply(t, planPath, project.prepared())
			require.ErrorIs(t, err, test.want)
			require.Nil(t, execution)
		})
	}
}

func TestApplyOfRestoredPlanVerifiesTheRestoredLockFile(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewInDefaultWorkdir(t)
	lock := filepath.Join(project.workdir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lock, []byte("# reviewed providers"), 0o644))
	// The record is made after init has written the lock file, so re-record with it present.
	info := project.info(t, "plan")
	execution := &componentExecContext{preparedConfiguration: project.prepared(), componentPath: project.workdir, workingDir: project.workdir}
	require.NoError(t, recordConfigurationPlan(t.Context(), project.config, info, execution, []string{"-out=" + planPath}))
	project.freshRunner(t, planPath, lock)

	_, execution, err := project.freshApply(t, planPath, project.prepared())
	require.NoError(t, err)
	restored, err := os.ReadFile(lock)
	require.NoError(t, err)
	require.Equal(t, "# reviewed providers", string(restored))
	require.NotNil(t, execution)
}

func TestApplyOfRestoredPlanRejectsChangedBackendAndFailedPreparation(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewInDefaultWorkdir(t)
	project.freshRunner(t, planPath)

	moved := project.prepared()
	moved.Backend = map[string]any{"path": filepath.Join(project.root, "elsewhere")}
	apply := project.reviewedApply(t, planPath)
	project.provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(moved, nil)
	_, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed, "a backend other than the reviewed one must not apply the plan")

	project.freshRunner(t, planPath)
	failing := project.reviewedApply(t, planPath)
	project.provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(nil, context.Canceled)
	_, err = testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, failing, false)
	require.ErrorIs(t, err, context.Canceled)
}

func TestStoredPlanIsAComparisonReferenceNotAPlanToApply(t *testing.T) {
	project := newReviewedPlanProject(t)
	deploy := project.info(t, "deploy")
	deploy.StoredPlanFile = filepath.Join(project.workdir, "stored.plan.tfplan")
	require.False(t, usesConfigurationPlan(deploy), "a downloaded plan used for drift verification must not trigger a saved-plan restore")
	project.provider.EXPECT().Prepare(gomock.Any(), deploy).Return(project.prepared(), nil)
	prepared, err := prepareConfiguration(t.Context(), project.config, deploy)
	require.NoError(t, err)
	require.NotEmpty(t, prepared.Inputs, "an ordinary preparation evaluates the unit again")
}

func TestReviewedRebuildAcceptsOnlyTheRestoredArtifacts(t *testing.T) {
	dir := t.TempDir()
	workdir := filepath.Join(dir, "work")
	planPath := filepath.Join(workdir, "reviewed.planfile")
	rebuild := &reviewedRebuild{plan: &configurationPlan{}, planPath: planPath, workdir: workdir}

	only, err := rebuild.holdsOnlyTransport()
	require.NoError(t, err)
	require.True(t, only, "a workdir that does not exist yet holds nothing unverifiable")

	require.NoError(t, os.MkdirAll(workdir, 0o755))
	for _, name := range []string{"reviewed.planfile", "reviewed.planfile" + configurationPlanSuffix, terraformLockFile} {
		require.NoError(t, os.WriteFile(filepath.Join(workdir, name), []byte(name), 0o600))
	}
	only, err = rebuild.holdsOnlyTransport()
	require.NoError(t, err)
	require.True(t, only)

	require.NoError(t, os.WriteFile(filepath.Join(workdir, "other.planfile"), nil, 0o600))
	only, err = rebuild.holdsOnlyTransport()
	require.NoError(t, err)
	require.False(t, only, "another plan file is not part of the restored artifact set")

	// A plan kept outside the workdir does not excuse files inside it.
	external := &reviewedRebuild{plan: &configurationPlan{}, planPath: filepath.Join(dir, "elsewhere.planfile"), workdir: workdir}
	only, err = external.holdsOnlyTransport()
	require.NoError(t, err)
	require.False(t, only)
}

func TestReviewedRebuildStagesAndReturnsTheArtifacts(t *testing.T) {
	workdir := filepath.Join(t.TempDir(), "work")
	rebuild := &reviewedRebuild{plan: &configurationPlan{}, planPath: filepath.Join(workdir, "reviewed.planfile"), workdir: workdir}

	restore, err := rebuild.stage()
	require.NoError(t, err)
	require.NoError(t, restore(), "staging a workdir that does not exist is a no-op")

	require.NoError(t, os.MkdirAll(workdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "reviewed.planfile"), []byte("plan"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workdir, terraformLockFile), []byte("restored lock"), 0o600))
	restore, err = rebuild.stage()
	require.NoError(t, err)
	require.NoDirExists(t, workdir, "provisioning gets an empty place")

	// Provisioning writes a lock file of its own; the restored one wins.
	require.NoError(t, os.MkdirAll(workdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdir, terraformLockFile), []byte("copied lock"), 0o600))
	require.NoError(t, restore())
	plan, err := os.ReadFile(filepath.Join(workdir, "reviewed.planfile"))
	require.NoError(t, err)
	require.Equal(t, "plan", string(plan))
	lock, err := os.ReadFile(filepath.Join(workdir, terraformLockFile))
	require.NoError(t, err)
	require.Equal(t, "restored lock", string(lock))
	holding, err := filepath.Glob(filepath.Join(filepath.Dir(workdir), ".atmos-restore-*"))
	require.NoError(t, err)
	require.Empty(t, holding, "the holding directory is removed")
}

func TestNewReviewedRebuildRequiresAnOldEnoughRecord(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "app", ComponentSettingsSection: map[string]any{"configuration": true}}
	_, err := newReviewedRebuild(info, &configurationPlan{WorkingDirectory: t.TempDir()}, "plan")
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed, "a record without a rebuild digest cannot prove a rebuilt workdir")
}
