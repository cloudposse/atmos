package container

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestRunEphemeralContainer_PullAlwaysBeforeCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)

	gomock.InOrder(
		runtime.EXPECT().Pull(ctx, "alpine:latest").Return(nil),
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("container-id", nil),
		runtime.EXPECT().Start(ctx, "container-id").Return(nil),
		runtime.EXPECT().Exec(ctx, "container-id", []string{"true"}, gomock.Any()).Return(nil),
		runtime.EXPECT().Remove(ctx, "container-id", true).Return(nil),
	)

	_, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:       "test",
		Image:      "alpine:latest",
		Command:    []string{"true"},
		PullPolicy: PullAlways,
	})

	require.NoError(t, err)
}

func TestRunEphemeralContainer_PullAlwaysErrorStopsBeforeCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	pullErr := errors.New("pull failed")

	runtime.EXPECT().Pull(ctx, "alpine:latest").Return(pullErr)

	_, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:       "test",
		Image:      "alpine:latest",
		Command:    []string{"true"},
		PullPolicy: PullAlways,
	})

	require.ErrorIs(t, err, pullErr)
	require.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
}

func TestRunEphemeralContainer_PullMissingCreateAndPullErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	createErr := errors.New("create failed")
	pullErr := errors.New("pull failed")

	gomock.InOrder(
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("", createErr),
		runtime.EXPECT().Pull(ctx, "alpine:latest").Return(pullErr),
	)

	_, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:    "test",
		Image:   "alpine:latest",
		Command: []string{"true"},
	})

	require.ErrorIs(t, err, createErr)
	require.ErrorIs(t, err, pullErr)
	require.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
}

// TestRunEphemeralContainer_PullNeverDoesNotRetryCreate is the negative-path
// counterpart to TestRunEphemeralContainer_PullMissingRetriesCreate: with
// PullNever, a create failure must surface immediately without a pull/retry.
func TestRunEphemeralContainer_PullNeverDoesNotRetryCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	createErr := errors.New("create failed")

	// Exactly one Create and no Pull: the test fails if recovery runs unexpectedly.
	runtime.EXPECT().Create(ctx, gomock.Any()).Return("", createErr)

	_, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:       "test",
		Image:      "alpine:latest",
		Command:    []string{"echo", "ok"},
		PullPolicy: PullNever,
	})

	require.ErrorIs(t, err, createErr)
	require.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
}

func TestRunEphemeralContainer_StartErrorCleanupAlways(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)
	startErr := errors.New("start failed")

	gomock.InOrder(
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("container-id", nil),
		runtime.EXPECT().Start(ctx, "container-id").Return(startErr),
		runtime.EXPECT().Remove(ctx, "container-id", true).Return(nil),
	)

	result, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:          "test",
		Image:         "alpine:latest",
		Command:       []string{"true"},
		CleanupPolicy: CleanupAlways,
	})

	require.ErrorIs(t, err, startErr)
	require.NotNil(t, result)
	assert.Equal(t, "container-id", result.ContainerID)
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

func TestRunEphemeralContainer_CleanupNeverLeavesSuccessfulContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	runtime := NewMockRuntime(ctrl)

	gomock.InOrder(
		runtime.EXPECT().Create(ctx, gomock.Any()).Return("container-id", nil),
		runtime.EXPECT().Start(ctx, "container-id").Return(nil),
		runtime.EXPECT().Exec(ctx, "container-id", []string{"true"}, gomock.Any()).Return(nil),
	)

	result, err := RunEphemeralContainer(ctx, runtime, &EphemeralConfig{
		Name:          "test",
		Image:         "alpine:latest",
		Command:       []string{"true"},
		CleanupPolicy: CleanupNever,
	})

	require.NoError(t, err)
	assert.Equal(t, "container-id", result.ContainerID)
}

func TestRunEphemeralContainer_NilInputs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	runtime := NewMockRuntime(ctrl)

	_, err := RunEphemeralContainer(context.Background(), nil, &EphemeralConfig{})
	require.ErrorIs(t, err, errUtils.ErrNilParam)

	_, err = RunEphemeralContainer(context.Background(), runtime, nil)
	require.ErrorIs(t, err, errUtils.ErrNilParam)
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

func TestBuildEphemeralPreviewAllOptions(t *testing.T) {
	preview := BuildEphemeralPreview("", &EphemeralConfig{
		Image:             "alpine:latest",
		Command:           []string{"/bin/sh", "-lc", "echo ok"},
		WorkspaceHostPath: "/repo",
		WorkspaceReadOnly: true,
		WorkspaceFolder:   "/work",
		CleanupPolicy:     CleanupNever,
		TTY:               true,
		Interactive:       true,
		User:              "1000:1000",
		Env:               []string{"FOO=bar"},
		Mounts: []Mount{{
			Type:     "volume",
			Source:   "cache",
			Target:   "/cache",
			ReadOnly: true,
		}},
		Ports:   []PortBinding{{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}},
		RunArgs: []string{"--network=host"},
	})

	assert.Contains(t, preview, "docker|podman run -t -i --user 1000:1000 -w /work")
	assert.NotContains(t, preview, "--rm")
	assert.Contains(t, preview, "-e FOO=bar")
	assert.Contains(t, preview, "--mount type=volume,source=cache,target=/cache,readonly")
	assert.Contains(t, preview, "--mount type=bind,source=/repo,target=/work,readonly")
	assert.Contains(t, preview, "-p 8080:80/tcp")
	assert.Contains(t, preview, "--network=host alpine:latest /bin/sh -lc echo ok")
}

func TestBuildImagePreviews(t *testing.T) {
	buildPreview := BuildImageBuildPreview("", &BuildConfig{
		Dockerfile: "Dockerfile",
		Context:    ".",
		Engine:     "buildx",
		Tags:       []string{"app:test"},
		Target:     "runtime",
		NoCache:    true,
		Pull:       true,
	})
	assert.Contains(t, buildPreview, "docker buildx build")
	assert.Contains(t, buildPreview, "--no-cache --pull --target runtime")
	assert.Contains(t, buildPreview, "-t app:test -f Dockerfile .")

	bakePreview := BuildImageBuildPreview("", &BuildConfig{
		Bake: &BakeConfig{
			File:    "docker-bake.hcl",
			Targets: []string{"default"},
			Push:    true,
		},
	})
	assert.Equal(t, "docker buildx bake --file docker-bake.hcl --push default", bakePreview)

	assert.Equal(t, "docker|podman tag app:test registry.example.com/app:test", BuildImageTagPreview("", "app:test", "registry.example.com/app:test"))
	assert.Equal(t, "podman push registry.example.com/app:test", BuildImagePushPreview("podman", "registry.example.com/app:test"))
}
