package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestReviewedPlanKeepsGeneratedErrorPolicyFile(t *testing.T) {
	project := newReviewedPlanProject(t)
	plan := project.info(t, "plan")
	prepared := project.prepared()
	prepared.Files = []schema.ComponentConfigurationFile{{Path: "labels.tf", Content: []byte("# reviewed labels"), IfExists: "error", Mode: 0o600}}
	project.provider.EXPECT().Prepare(gomock.Any(), plan).Return(prepared, nil)
	execution, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, plan, false)
	require.NoError(t, err)
	path := filepath.Join(execution.workingDir, "reviewed.planfile")
	require.NoError(t, os.WriteFile(path, []byte("reviewed plan"), 0o600))
	observeFixturePlan(t, project.config, plan, execution, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), project.config, plan, execution, []string{"-out=" + path}))
	apply := project.reviewedApply(t, path)
	_, err = testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
	require.NoError(t, err)
	contents, err := os.ReadFile(filepath.Join(project.workdir, "labels.tf"))
	require.NoError(t, err)
	require.Equal(t, "# reviewed labels", string(contents))
}

func TestReviewedApplyRejectsBeforeHookChanges(t *testing.T) {
	for _, target := range []string{"plan", "workdir"} {
		t.Run(target, func(t *testing.T) {
			project := newReviewedPlanProject(t)
			path := project.review(t)
			apply := project.reviewedApply(t, path)
			_, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, apply, false)
			require.NoError(t, err)
			apply.ComponentExecution = &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{execHook("change", "apply")}}
			shell := newShellFixture(t, func(call *shellCall) error {
				require.Equal(t, "hook-change", call.command, "apply subprocess must not start")
				changed := path
				if target == "workdir" {
					changed = filepath.Join(project.workdir, "main.tf")
				}
				return os.WriteFile(changed, []byte("changed by before hook"), 0o600)
			})
			err = hookExecutor(shell).runMainShellCommand(project.config, apply, []string{"apply", path}, project.workdir, nil)
			require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
			require.Len(t, shell.calls, 1)
		})
	}
}

func TestPolicySnapshotRejectsPlanRewrittenBeforePublication(t *testing.T) {
	for _, rewrite := range []bool{false, true} {
		t.Run(fmt.Sprint(rewrite), func(t *testing.T) {
			config, info, execution, _, published := planFixture(t)
			private := filepath.Join(t.TempDir(), "policy.plan")
			execution.planPolicy = &planfile.Guard{}
			ctx := execution.observePlanSnapshots(t.Context(), config, info, []string{"plan", "-out=" + published})
			snapshot := ctx.Value(planSnapshotKey{}).(*planSnapshotter)
			run := snapshot.around([]string{"plan", "-out=" + private}, func() error { return os.WriteFile(private, []byte("engine plan"), 0o600) }, failedOnError)
			require.NoError(t, run())
			digest, err := fileDigest(private)
			require.NoError(t, err)
			require.Equal(t, digest, snapshot.result.planDigest)
			require.Equal(t, published, snapshot.result.path)
			if rewrite {
				require.NoError(t, os.WriteFile(private, []byte("after hook plan"), 0o600))
			}
			bytes, err := os.ReadFile(private)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(published, bytes, 0o600))
			err = recordConfigurationPlan(t.Context(), config, info, execution, []string{"plan", "-out=" + published})
			if rewrite {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPlanBindingUsesPostInitDirectory(t *testing.T) {
	config, info, execution, _, _ := planFixture(t)
	original := execution.componentPath
	relocated := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(relocated, "main.tf"), []byte("relocated module"), 0o600))
	shell := newShellFixture(t, func(call *shellCall) error {
		require.Equal(t, relocated, call.dir)
		return os.WriteFile(filepath.Join(call.dir, "moved.plan"), []byte("actual plan"), 0o600)
	})
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.mock.Execute
	err := executor.runMainCommandPhase(config, info, execution, []string{"plan", "-out=moved.plan"}, relocated, false)
	require.NoError(t, err)
	binding, err := loadConfigurationPlan(filepath.Join(relocated, "moved.plan"), info)
	require.NoError(t, err)
	require.Equal(t, relocated, binding.WorkingDirectory)
	require.NoFileExists(t, filepath.Join(original, "moved.plan"+configurationPlanSuffix))
}
