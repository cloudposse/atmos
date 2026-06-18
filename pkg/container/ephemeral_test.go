package container

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRunEphemeralContainer_SuccessCleanupAlways(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)

	gomock.InOrder(
		runtime.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, cfg *CreateConfig) (string, error) {
			assert.Equal(t, "alpine:latest", cfg.Image)
			assert.Equal(t, "/workspace", cfg.WorkspaceFolder)
			assert.True(t, cfg.OverrideCommand)
			require.Len(t, cfg.Mounts, 1)
			assert.Equal(t, "/host", cfg.Mounts[0].Source)
			assert.Equal(t, "/workspace", cfg.Mounts[0].Target)
			return "container-id", nil
		}),
		runtime.EXPECT().Start(ctx, "container-id").Return(nil),
		runtime.EXPECT().Exec(ctx, "container-id", []string{"/bin/sh", "-lc", "echo ok"}, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ []string, opts *ExecOptions) error {
				_, _ = io.WriteString(opts.Stdout, "ok\n")
				_, _ = io.WriteString(opts.Stderr, "warn\n")
				assert.Equal(t, []string{"FOO=bar"}, opts.Env)
				return nil
			}),
		runtime.EXPECT().Remove(ctx, "container-id", true).Return(nil),
	)

	result, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:              "test",
		Image:             "alpine:latest",
		Command:           []string{"/bin/sh", "-lc", "echo ok"},
		WorkspaceHostPath: "/host",
		Env:               []string{"FOO=bar"},
	})

	require.NoError(t, err)
	assert.Equal(t, "container-id", result.ContainerID)
	assert.Equal(t, "ok\n", result.Stdout)
	assert.Equal(t, "warn\n", result.Stderr)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunEphemeralContainer_PullMissingRetriesCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	createErr := errors.New("image not found")

	gomock.InOrder(
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("", createErr),
		runtime.EXPECT().Pull(ctx, "alpine:latest").Return(nil),
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("container-id", nil),
		runtime.EXPECT().Start(ctx, "container-id").Return(nil),
		runtime.EXPECT().Exec(ctx, "container-id", []string{"true"}, gomock.Any()).Return(nil),
		runtime.EXPECT().Remove(ctx, "container-id", true).Return(nil),
	)

	_, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:    "test",
		Image:   "alpine:latest",
		Command: []string{"true"},
	})

	require.NoError(t, err)
}

func TestRunEphemeralContainer_CleanupOnSuccessLeavesFailedContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	execErr := errors.New("failed")

	gomock.InOrder(
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("container-id", nil),
		runtime.EXPECT().Start(ctx, "container-id").Return(nil),
		runtime.EXPECT().Exec(ctx, "container-id", []string{"false"}, gomock.Any()).Return(execErr),
	)

	result, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:          "test",
		Image:         "alpine:latest",
		Command:       []string{"false"},
		CleanupPolicy: CleanupOnSuccess,
	})

	require.ErrorIs(t, err, execErr)
	assert.Equal(t, 1, result.ExitCode)
}

func TestBuildEphemeralPreview(t *testing.T) {
	preview := BuildEphemeralPreview("docker", &EphemeralConfig{
		Image:             "alpine:latest",
		Command:           []string{"/bin/sh", "-lc", "echo ok"},
		WorkspaceHostPath: "/repo",
		Env:               []string{"FOO=bar"},
	})

	assert.Contains(t, preview, "docker run --rm")
	assert.Contains(t, preview, "-e FOO=bar")
	assert.Contains(t, preview, "--mount type=bind,source=/repo,target=/workspace")
	assert.Contains(t, preview, "alpine:latest /bin/sh -lc echo ok")
}
