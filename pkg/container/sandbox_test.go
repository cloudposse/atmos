package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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

	sandbox, err := startSandboxWithRuntime(ctx, runtime, config)
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
				runtime.EXPECT().Stop(gomock.Any(), "container-id", gomock.Any()).Return(nil)
				runtime.EXPECT().Remove(gomock.Any(), "container-id", true).Return(nil)
			}

			require.NoError(t, sandbox.Cleanup(tt.success))
		})
	}
}
