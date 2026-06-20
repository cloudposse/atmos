package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

func installStepFakeDocker(t *testing.T) {
	t.Helper()
	testhelpers.InstallFakeContainerRuntime(t, testhelpers.FakeContainerRuntimeSpec{
		Name: string(container.TypeDocker),
		Mode: testhelpers.FakeContainerRuntimeStep,
	})
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
