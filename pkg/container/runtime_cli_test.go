package container

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installFakeRuntime(t *testing.T, name string, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime scripts are POSIX shell based")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func fakeRuntimeScript(runtimeName string) string {
	return `#!/bin/sh
if [ "$1" != "info" ] && [ "$1" != "version" ]; then
  if [ "$ATMOS_FAKE_AUTH" != "present" ]; then
    echo "missing forwarded env" >&2
    exit 9
  fi
fi
case "$1" in
  info)
    exit 0
    ;;
  version)
    echo "1.2.3"
    ;;
  tag)
    echo "tagged"
    ;;
  push)
    echo "latest: digest: sha256:abcdef1234567890 size: 1234"
    ;;
  image)
    if [ "$2" = "inspect" ]; then
      printf '%s\n' '{"Id":"sha256:abcdef","RepoTags":["app:local"],"RepoDigests":["app@sha256:abcdef"],"Size":2048,"Created":"2026-06-19T00:00:00Z","Architecture":"arm64","Os":"linux","Config":{"Labels":{"app":"test"}},"RootFS":{"Layers":["l1","l2"]}}'
      exit 0
    fi
    echo "unknown image command" >&2
    exit 4
    ;;
  create)
    echo "pull progress"
    echo "` + runtimeName + `-container-id"
    ;;
  start|stop|rm|pull)
    exit 0
    ;;
  inspect)
    printf '%s\n' '{"Id":"container-id","Name":"/box","Image":"sha256:img","State":{"Status":"running"},"Config":{"Labels":{"app":"test"}},"Created":"2026-06-19T00:00:00Z"}'
    ;;
  ps)
    if [ "` + runtimeName + `" = "podman" ]; then
      printf '%s\n' '[{"Id":"podman-id","Names":["podman-box"],"Image":"alpine","State":"running","Labels":{"app":"test"}}]'
    else
      printf '%s\n' '{"ID":"docker-id","Names":"/docker-box","Image":"alpine","State":"running","Labels":"app=test"}'
      printf '%s\n' 'not-json'
    fi
    ;;
  exec)
    echo "exec stdout"
    ;;
  logs)
    echo "log stdout"
    ;;
  *)
    echo "unknown command: $*" >&2
    exit 4
    ;;
esac
`
}

func TestDockerRuntimeCLIMethodsUseConfiguredEnv(t *testing.T) {
	installFakeRuntime(t, dockerCmd, fakeRuntimeScript("docker"))
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
	installFakeRuntime(t, podmanCmd, fakeRuntimeScript("podman"))
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
	script := `#!/bin/sh
case "$1" in
  info) exit 0 ;;
  create) exit 0 ;;
  *) exit 0 ;;
esac
`
	installFakeRuntime(t, dockerCmd, script)
	installFakeRuntime(t, podmanCmd, script)

	_, dockerErr := NewDockerRuntime().Create(context.Background(), &CreateConfig{Name: "box", Image: "alpine"})
	require.Error(t, dockerErr)
	assert.Contains(t, dockerErr.Error(), "returned no container ID")

	_, podmanErr := NewPodmanRuntime().Create(context.Background(), &CreateConfig{Name: "box", Image: "alpine"})
	require.Error(t, podmanErr)
	assert.Contains(t, podmanErr.Error(), "returned no container ID")
}

func TestRuntimePushReturnsPartialResultOnError(t *testing.T) {
	script := `#!/bin/sh
case "$1" in
  info) exit 0 ;;
  push)
    echo "error digest: sha256:feedface"
    exit 7
    ;;
  *) exit 0 ;;
esac
`
	installFakeRuntime(t, dockerCmd, script)
	installFakeRuntime(t, podmanCmd, strings.ReplaceAll(script, "error digest", "error\\ndigest"))

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
