package container

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestStartSandbox_RemovesStoppedMatchingOrphansAndUsesUniqueName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	config := NewWorkflowSandboxConfig("deploy", "workflows/deploy.yaml", "/repo")
	config.Image = "alpine:latest"

	gomock.InOrder(
		runtime.EXPECT().
			List(ctx, map[string]string{"label": "com.atmos.type=workflow-sandbox"}).
			Return([]Info{
				{
					ID:     "old-stopped",
					Status: "exited",
					Labels: map[string]string{
						SandboxLabelType:         SandboxTypeWorkflow,
						SandboxLabelWorkflow:     "deploy",
						SandboxLabelWorkflowPath: "workflows/deploy.yaml",
						SandboxLabelWorkspace:    "/repo",
					},
				},
				{
					ID:     "old-running",
					Status: "running",
					Labels: map[string]string{
						SandboxLabelType:         SandboxTypeWorkflow,
						SandboxLabelWorkflow:     "deploy",
						SandboxLabelWorkflowPath: "workflows/deploy.yaml",
						SandboxLabelWorkspace:    "/repo",
					},
				},
				{
					ID:     "other",
					Status: "exited",
					Labels: map[string]string{
						SandboxLabelType:      SandboxTypeWorkflow,
						SandboxLabelWorkflow:  "other",
						SandboxLabelWorkspace: "/repo",
					},
				},
			}, nil),
		runtime.EXPECT().Remove(ctx, "old-stopped", true).Return(nil),
		runtime.EXPECT().Create(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, create *CreateConfig) (string, error) {
				assert.NotEqual(t, "old-stopped", create.Name)
				assert.Contains(t, create.Name, "atmos-workflow-deploy")
				assert.Equal(t, "alpine:latest", create.Image)
				assert.Equal(t, SandboxTypeWorkflow, create.Labels[SandboxLabelType])
				assert.Equal(t, "deploy", create.Labels[SandboxLabelWorkflow])
				return "new-container", nil
			}),
		runtime.EXPECT().Start(ctx, "new-container").Return(nil),
	)

	sandbox, err := startSandboxWithRuntime(ctx, runtime, &config)
	require.NoError(t, err)
	assert.Equal(t, "new-container", sandbox.ID())
}

func TestSandboxCleanupPolicies(t *testing.T) {
	tests := []struct {
		name          string
		policy        string
		success       bool
		expectCleanup bool
	}{
		{name: "always success", policy: CleanupAlways, success: true, expectCleanup: true},
		{name: "always failure", policy: CleanupAlways, success: false, expectCleanup: true},
		{name: "on success success", policy: CleanupOnSuccess, success: true, expectCleanup: true},
		{name: "on success failure", policy: CleanupOnSuccess, success: false, expectCleanup: false},
		{name: "never", policy: CleanupNever, success: true, expectCleanup: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			runtime := NewMockRuntime(ctrl)
			sandbox := &Sandbox{
				config: SandboxConfig{
					CleanupPolicy: tt.policy,
				},
				runtime:     runtime,
				containerID: "container-id",
			}
			if tt.expectCleanup {
				// Cleanup force-removes directly (no graceful Stop) to avoid the
				// PID-1-ignores-SIGTERM grace-period stall.
				runtime.EXPECT().Remove(gomock.Any(), "container-id", true).Return(nil)
			}

			require.NoError(t, sandbox.Cleanup(tt.success))
		})
	}
}

func TestCreateSandboxContainer_PullsOnImageMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	config := &SandboxConfig{Image: "alpine:latest", PullPolicy: PullMissing}

	// Image-missing create error is recoverable: pull, then recreate succeeds.
	gomock.InOrder(
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("", errors.New("no such image: alpine:latest")),
		runtime.EXPECT().Pull(ctx, "alpine:latest").Return(nil),
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("cid", nil),
	)

	id, err := createSandboxContainer(ctx, runtime, config)
	require.NoError(t, err)
	assert.Equal(t, "cid", id)
}

func TestCreateSandboxContainer_NonImageErrorDoesNotPull(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	config := &SandboxConfig{Image: "alpine:latest", PullPolicy: PullMissing}
	createErr := errors.New("statfs /run/agent.sock: operation not supported")

	// Exactly one Create and no Pull: a non-image-missing failure surfaces as-is.
	runtime.EXPECT().Create(ctx, gomock.Any()).Return("", createErr)

	_, err := createSandboxContainer(ctx, runtime, config)
	require.ErrorIs(t, err, createErr)
	require.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
}

func TestNewWorkflowSandboxConfig(t *testing.T) {
	cfg := NewWorkflowSandboxConfig("deploy", "workflows/deploy.yaml", "/repo")
	assert.Contains(t, cfg.Name, "atmos-workflow-")
	assert.Equal(t, "deploy", cfg.Workflow)
	assert.Equal(t, "workflows/deploy.yaml", cfg.WorkflowPath)
	assert.Equal(t, "/repo", cfg.WorkspaceHostPath)
	assert.Equal(t, "/workspace", cfg.WorkspaceFolder)
	assert.Equal(t, PullMissing, cfg.PullPolicy)
	assert.Equal(t, CleanupAlways, cfg.CleanupPolicy)
	assert.Equal(t, SandboxTypeWorkflow, cfg.Labels[SandboxLabelType])
	assert.Equal(t, "deploy", cfg.Labels[SandboxLabelWorkflow])
	assert.NotEmpty(t, cfg.RunID)
}

func TestNormalizeSandboxConfig(t *testing.T) {
	cfg := &SandboxConfig{RunID: "rid", Workflow: "w", WorkflowPath: "p", WorkspaceHostPath: "/repo"}
	normalizeSandboxConfig(cfg)
	assert.Equal(t, "atmos-workflow-rid", cfg.Name)
	assert.Equal(t, "/workspace", cfg.WorkspaceFolder)
	assert.Equal(t, PullMissing, cfg.PullPolicy)
	assert.Equal(t, CleanupAlways, cfg.CleanupPolicy)
	assert.Equal(t, SandboxTypeWorkflow, cfg.Labels[SandboxLabelType])
	assert.Equal(t, "w", cfg.Labels[SandboxLabelWorkflow])
	assert.Equal(t, "p", cfg.Labels[SandboxLabelWorkflowPath])
	assert.Equal(t, "rid", cfg.Labels[SandboxLabelRunID])
	assert.Equal(t, "/repo", cfg.Labels[SandboxLabelWorkspace])

	// Existing values are preserved (no clobbering).
	custom := &SandboxConfig{Name: "n", WorkspaceFolder: "/w", PullPolicy: PullAlways, CleanupPolicy: CleanupNever}
	normalizeSandboxConfig(custom)
	assert.Equal(t, "n", custom.Name)
	assert.Equal(t, "/w", custom.WorkspaceFolder)
	assert.Equal(t, PullAlways, custom.PullPolicy)
	assert.Equal(t, CleanupNever, custom.CleanupPolicy)
}

func TestBuildSandboxCreateConfig(t *testing.T) {
	cfg := &SandboxConfig{
		Name: "n", Image: "alpine", WorkspaceFolder: "/workspace", WorkspaceHostPath: "/repo",
		WorkspaceReadOnly: true,
		Mounts:            []Mount{{Type: "bind", Source: "/x", Target: "/y"}},
	}
	got := buildSandboxCreateConfig(cfg)
	assert.Equal(t, "n", got.Name)
	assert.Equal(t, "alpine", got.Image)
	assert.True(t, got.OverrideCommand)
	require.Len(t, got.Mounts, 2) // user mount + appended workspace mount
	ws := got.Mounts[1]
	assert.Equal(t, "/repo", ws.Source)
	assert.Equal(t, "/workspace", ws.Target)
	assert.True(t, ws.ReadOnly)

	// No workspace host path -> no workspace mount appended.
	got2 := buildSandboxCreateConfig(&SandboxConfig{Name: "n", Image: "alpine"})
	assert.Empty(t, got2.Mounts)
}

func TestMatchesSandboxLabels(t *testing.T) {
	cfg := &SandboxConfig{Workflow: "deploy", WorkflowPath: "p", WorkspaceHostPath: "/repo"}
	full := map[string]string{
		SandboxLabelType: SandboxTypeWorkflow, SandboxLabelWorkflow: "deploy",
		SandboxLabelWorkflowPath: "p", SandboxLabelWorkspace: "/repo",
	}
	assert.True(t, matchesSandboxLabels(full, cfg))
	assert.False(t, matchesSandboxLabels(nil, cfg))
	assert.False(t, matchesSandboxLabels(map[string]string{SandboxLabelType: "other"}, cfg))
	assert.False(t, matchesSandboxLabels(
		map[string]string{SandboxLabelType: SandboxTypeWorkflow, SandboxLabelWorkflow: "other"}, cfg,
	))
}

func TestIsContainerRunning(t *testing.T) {
	assert.True(t, isContainerRunning("Running"))
	assert.True(t, isContainerRunning("Up 3 minutes"))
	assert.False(t, isContainerRunning("Exited (0)"))
	assert.False(t, isContainerRunning(""))
}

func TestSanitizeSandboxName(t *testing.T) {
	assert.Equal(t, "deploy-prod", sanitizeSandboxName("deploy/prod"))
	assert.Equal(t, "workflow", sanitizeSandboxName("///"))
	assert.Equal(t, "abc", sanitizeSandboxName("abc"))
	assert.Len(t, sanitizeSandboxName(strings.Repeat("a", 60)), maxSandboxNameLength)
}

func TestSandboxIDAndName(t *testing.T) {
	var nilSandbox *Sandbox
	assert.Equal(t, "", nilSandbox.ID())
	assert.Equal(t, "", nilSandbox.Name())

	dryRun := &Sandbox{config: SandboxConfig{Name: "gen-name"}}
	assert.Equal(t, "gen-name", dryRun.ID()) // no containerID -> falls back to name
	assert.Equal(t, "gen-name", dryRun.Name())

	started := &Sandbox{config: SandboxConfig{Name: "gen-name"}, containerID: "cid"}
	assert.Equal(t, "cid", started.ID())
}

func TestSandboxExec_DryRunAndNil(t *testing.T) {
	var nilSandbox *Sandbox
	assert.ErrorIs(t, nilSandbox.Exec(context.Background(), []string{"true"}, &ExecOptions{}), errUtils.ErrNilParam)

	dryRun := &Sandbox{config: SandboxConfig{DryRun: true}}
	assert.NoError(t, dryRun.Exec(context.Background(), []string{"true"}, &ExecOptions{}))
}
