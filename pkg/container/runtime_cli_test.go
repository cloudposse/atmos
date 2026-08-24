package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

func TestDockerRuntimeCLIMethodsUseConfiguredEnv(t *testing.T) {
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: dockerCmd,
		Mode: testhelpers.FakeContainerRuntimeFull,
	})
	ctx := context.Background()
	runtime := NewDockerRuntime()
	runtime.SetEnv([]string{"ATMOS_FAKE_AUTH=present"})

	require.NoError(t, runtime.Tag(ctx, "app:local", "registry.example.com/app:local"))

	push, err := runtime.Push(ctx, "registry.example.com/app:local")
	require.NoError(t, err)
	assert.Equal(t, "registry.example.com/app:local", push.Image)
	assert.Equal(t, "sha256:abcdef1234567890", push.Digest)

	image, err := runtime.ImageInspect(ctx, "app:local")
	require.NoError(t, err)
	assert.Equal(t, "sha256:abcdef", image.ID)
	assert.Equal(t, []string{"app:local"}, image.RepoTags)
	assert.Equal(t, int64(2048), image.Size)
	assert.Equal(t, 2, image.Layers)

	containerID, err := runtime.Create(ctx, &CreateConfig{Name: "box", Image: "alpine"})
	require.NoError(t, err)
	assert.Equal(t, "docker-container-id", containerID)

	require.NoError(t, runtime.Start(ctx, containerID))
	require.NoError(t, runtime.Stop(ctx, containerID, time.Second))
	require.NoError(t, runtime.Remove(ctx, containerID, true))

	info, err := runtime.Inspect(ctx, containerID)
	require.NoError(t, err)
	assert.Equal(t, "container-id", info.ID)
	assert.Equal(t, "box", info.Name)
	assert.Equal(t, "running", info.Status)

	list, err := runtime.List(ctx, map[string]string{"label": "app=test"})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "docker-box", list[0].Name)
	assert.Equal(t, map[string]string{"app": "test"}, list[0].Labels)

	require.NoError(t, runtime.Exec(ctx, containerID, []string{"echo", "hi"}, &ExecOptions{}))
}

func TestPodmanRuntimeCLIMethodsUseConfiguredEnv(t *testing.T) {
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: podmanCmd,
		Mode: testhelpers.FakeContainerRuntimeFull,
	})
	ctx := context.Background()
	runtime := NewPodmanRuntime()
	runtime.SetEnv([]string{"ATMOS_FAKE_AUTH=present"})

	require.NoError(t, runtime.Tag(ctx, "app:local", "registry.example.com/app:local"))

	push, err := runtime.Push(ctx, "registry.example.com/app:local")
	require.NoError(t, err)
	assert.Equal(t, "sha256:abcdef1234567890", push.Digest)

	image, err := runtime.ImageInspect(ctx, "app:local")
	require.NoError(t, err)
	assert.Equal(t, "sha256:abcdef", image.ID)
	assert.Equal(t, "linux", image.Os)

	containerID, err := runtime.Create(ctx, &CreateConfig{Name: "box", Image: "alpine"})
	require.NoError(t, err)
	assert.Equal(t, "podman-container-id", containerID)

	require.NoError(t, runtime.Start(ctx, containerID))
	require.NoError(t, runtime.Stop(ctx, containerID, time.Second))
	require.NoError(t, runtime.Remove(ctx, containerID, true))

	list, err := runtime.List(ctx, map[string]string{"label": "app=test"})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "podman-box", list[0].Name)
	assert.Equal(t, map[string]string{"app": "test"}, list[0].Labels)

	info, err := runtime.Inspect(ctx, "podman-box")
	require.NoError(t, err)
	assert.Equal(t, "podman-id", info.ID)

	require.NoError(t, runtime.Exec(ctx, containerID, []string{"echo", "hi"}, &ExecOptions{}))
}

func TestRuntimeCreateErrorsOnEmptyContainerID(t *testing.T) {
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: dockerCmd,
		Mode: testhelpers.FakeContainerRuntimeEmptyCreate,
	})
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: podmanCmd,
		Mode: testhelpers.FakeContainerRuntimeEmptyCreate,
	})

	_, dockerErr := NewDockerRuntime().Create(context.Background(), &CreateConfig{Name: "box", Image: "alpine"})
	require.Error(t, dockerErr)
	assert.Contains(t, dockerErr.Error(), "returned no container ID")

	_, podmanErr := NewPodmanRuntime().Create(context.Background(), &CreateConfig{Name: "box", Image: "alpine"})
	require.Error(t, podmanErr)
	assert.Contains(t, podmanErr.Error(), "returned no container ID")
}

func TestRuntimePushReturnsPartialResultOnError(t *testing.T) {
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: dockerCmd,
		Mode: testhelpers.FakeContainerRuntimePushError,
	})
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: podmanCmd,
		Mode: testhelpers.FakeContainerRuntimePushError,
	})

	dockerResult, dockerErr := NewDockerRuntime().Push(context.Background(), "app:local")
	require.Error(t, dockerErr)
	require.NotNil(t, dockerResult)
	assert.Equal(t, "sha256:feedface", dockerResult.Digest)

	podmanResult, podmanErr := NewPodmanRuntime().Push(context.Background(), "app:local")
	require.Error(t, podmanErr)
	require.NotNil(t, podmanResult)
	assert.Equal(t, "sha256:feedface", podmanResult.Digest)
	assert.Contains(t, podmanResult.Output, "\n")
}
