package step

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func installStepFakeDocker(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker script is POSIX shell based")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  info) exit 0 ;;
  build|pull|start|rm|tag) exit 0 ;;
  create)
    echo "container-id"
    ;;
  exec)
    echo "run stdout"
    ;;
  image)
    if [ "$2" = "inspect" ]; then
      printf '%s\n' '{"Id":"sha256:built","RepoTags":["app:local"],"RepoDigests":["app@sha256:built"],"Size":1024,"Created":"2026-06-19T00:00:00Z","Architecture":"amd64","Os":"linux","Config":{"Labels":{"app":"test"}},"RootFS":{"Layers":["l1"]}}'
      exit 0
    fi
    exit 4
    ;;
  ps)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestContainerHandlerExecuteRunWithFakeDocker(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}

	res, err := h.executeRun(context.Background(), &schema.WorkflowStep{
		Name: "run",
		Run: &schema.ContainerRunStep{
			Image:    "alpine",
			Command:  "echo hi",
			Provider: string(container.TypeDocker),
		},
	}, NewVariables(), &schema.WorkflowDefinition{Output: "none"})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "run stdout\n", res.Metadata["stdout"])
	assert.Equal(t, 0, res.Metadata[exitCodeMetadata])
	assert.Equal(t, "container-id", res.Metadata["container_id"])
}

func TestContainerHandlerExecuteBuildWithFakeDocker(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}

	res, err := h.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Context:  ".",
			Tags:     []string{"app:local"},
		},
	}, NewVariables())

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "app:local", res.Metadata["image"])
	assert.Equal(t, "sha256:built", res.Metadata["image_id"])
	assert.Equal(t, []string{"app:local"}, res.Metadata["repo_tags"])
}

func TestContainerHandlerExecuteInspectWithFakeDocker(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}

	res, err := h.executeInspect(context.Background(), &schema.WorkflowStep{
		Name: "inspect",
		Inspect: &schema.ContainerInspectStep{
			Image:    "app:local",
			Provider: string(container.TypeDocker),
		},
	}, NewVariables())

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "app:local", res.Metadata["image"])
	assert.Equal(t, "sha256:built", res.Metadata["image_id"])
	assert.Equal(t, int64(1024), res.Metadata["size"])
}
