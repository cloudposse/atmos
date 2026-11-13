package terraform

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner"
)

// Compile-time sentinel for the record field these tests depend on.
var _ = configurationPlan{Modules: true}

var (
	errModuleDownload    = errors.New("module source unreachable")
	errUnexpectedCommand = errors.New("unexpected command")
)

// reviewWithModules is reviewInDefaultWorkdir for a unit whose `init` downloaded a child module.
func (p *reviewedPlanProject) reviewWithModules(t *testing.T, module string) string {
	t.Helper()
	p.explicitWorkdir = false
	info := p.info(t, "plan")
	p.provider.EXPECT().Prepare(gomock.Any(), info).Return(p.prepared(), nil)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, p.config, info, false)
	require.NoError(t, err)
	p.workdir = execution.workingDir
	writeModule(t, p.workdir, module)
	planPath := filepath.Join(p.workdir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(planPath, []byte("reviewed plan"), 0o600))
	require.NoError(t, recordConfigurationPlan(t.Context(), p.config, info, execution, []string{"-out=" + planPath}))
	return planPath
}

func writeModule(t *testing.T, workdir, content string) {
	t.Helper()
	module := filepath.Join(workdir, downloadedModulesDir, "vpc", "main.tf")
	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.WriteFile(module, []byte(content), 0o600))
}

func readPlanRecord(t *testing.T, planPath string) configurationPlan {
	t.Helper()
	data, err := os.ReadFile(planPath + configurationPlanSuffix)
	require.NoError(t, err)
	var record configurationPlan
	require.NoError(t, json.Unmarshal(data, &record))
	return record
}

// moduleGetter is a fake shell that stands in for `terraform get`: it installs the given module content into
// the directory it runs in, or fails.
func moduleGetter(content string, failure error) *fakeShell {
	return &fakeShell{run: func(call *shellCall) error {
		if !slices.Equal(call.args, []string{"get"}) {
			return errUnexpectedCommand
		}
		if failure != nil {
			return failure
		}
		module := filepath.Join(call.dir, downloadedModulesDir, "vpc", "main.tf")
		if err := os.MkdirAll(filepath.Dir(module), 0o755); err != nil {
			return err
		}
		return os.WriteFile(module, []byte(content), 0o600)
	}}
}

// callDirs lists the directory of every call.
func callDirs(shell *fakeShell) []string {
	dirs := make([]string, 0, len(shell.calls))
	for i := range shell.calls {
		dirs = append(dirs, shell.calls[i].dir)
	}
	return dirs
}

// A pinned module downloads the same bytes on a fresh runner, so the rebuilt workdir verifies.
func TestApplyOfRestoredPlanDownloadsPinnedModulesBeforeVerification(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewWithModules(t, "# vpc v1.2.3")
	require.True(t, readPlanRecord(t, planPath).Modules, "the record notes the downloaded modules")
	project.freshRunner(t, planPath)
	getter := moduleGetter("# vpc v1.2.3", nil)
	testExecutor(t).services.ExecuteShellCommand = getter.execute

	_, execution, err := project.freshApply(t, planPath, project.prepared())

	require.NoError(t, err)
	require.Equal(t, []string{project.workdir}, callDirs(getter), "modules download once, into the rebuilt workdir")
	require.Equal(t, project.workdir, execution.componentPath)
}

// A floating source that now resolves to different code rejects the plan: the modules stay in the digest.
func TestApplyOfRestoredPlanRejectsChangedModules(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewWithModules(t, "# vpc reviewed")
	project.freshRunner(t, planPath)
	testExecutor(t).services.ExecuteShellCommand = moduleGetter("# vpc moved branch", nil).execute

	_, _, err := project.freshApply(t, planPath, project.prepared())

	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
}

// A download failure rejects the plan rather than skipping the module check.
func TestApplyOfRestoredPlanRejectsUnreachableModules(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewWithModules(t, "# vpc reviewed")
	project.freshRunner(t, planPath)
	testExecutor(t).services.ExecuteShellCommand = moduleGetter("", errModuleDownload).execute

	_, _, err := project.freshApply(t, planPath, project.prepared())

	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, errModuleDownload)
}

// A plan made without downloaded modules never downloads anything on a fresh runner.
func TestApplyOfRestoredPlanWithoutModulesDoesNotDownload(t *testing.T) {
	project := newReviewedPlanProject(t)
	planPath := project.reviewInDefaultWorkdir(t)
	require.False(t, readPlanRecord(t, planPath).Modules)
	project.freshRunner(t, planPath)
	getter := moduleGetter("", nil)
	testExecutor(t).services.ExecuteShellCommand = getter.execute

	_, _, err := project.freshApply(t, planPath, project.prepared())

	require.NoError(t, err)
	require.Empty(t, getter.calls)
}
