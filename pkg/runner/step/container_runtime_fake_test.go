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

func TestContainerHandlerExecuteBuildWritesCISummaryWhenEnabled(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}
	var summaries []string
	prev := writeStepSummaryFn
	writeStepSummaryFn = func(content string) error {
		summaries = append(summaries, content)
		return nil
	}
	defer func() { writeStepSummaryFn = prev }()
	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}})

	_, err := h.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Context:  ".",
			Tags:     []string{"app:local"},
		},
	}, vars)

	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Contains(t, summaries[0], "## 🐳 app:local")
	assert.Contains(t, summaries[0], "| Digest | `sha256:built` |")
}

func TestContainerHandlerExecuteBuildSkipsCISummaryWhenDisabled(t *testing.T) {
	installStepFakeDocker(t)
	h := &ContainerHandler{}
	called := false
	prev := writeStepSummaryFn
	writeStepSummaryFn = func(string) error {
		called = true
		return nil
	}
	defer func() { writeStepSummaryFn = prev }()
	disabled := false
	vars := NewVariables()
	vars.SetAtmosConfig(&schema.AtmosConfiguration{
		CI: schema.CIConfig{
			Enabled: true,
			Summary: schema.CISummaryConfig{
				Enabled: &disabled,
			},
		},
	})

	_, err := h.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Context:  ".",
			Tags:     []string{"app:local"},
		},
	}, vars)

	require.NoError(t, err)
	assert.False(t, called)
}

func TestWriteContainerImageSummarySkipsWhenCIUnavailable(t *testing.T) {
	prev := writeStepSummaryFn
	called := false
	writeStepSummaryFn = func(string) error {
		called = true
		return nil
	}
	defer func() { writeStepSummaryFn = prev }()

	writeContainerImageSummary(nil, &container.ImageInfo{RepoTags: []string{"app:local"}}, container.ImageSummaryOptions{Image: "app:local"})
	writeContainerImageSummary(&schema.AtmosConfiguration{}, &container.ImageInfo{RepoTags: []string{"app:local"}}, container.ImageSummaryOptions{Image: "app:local"})
	writeContainerImageSummary(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, nil, container.ImageSummaryOptions{Image: "app:local"})

	assert.False(t, called)
}

func TestWriteContainerImageSummaryIgnoresWriteError(t *testing.T) {
	prev := writeStepSummaryFn
	writeStepSummaryFn = func(string) error {
		return assert.AnError
	}
	defer func() { writeStepSummaryFn = prev }()

	assert.NotPanics(t, func() {
		writeContainerImageSummary(
			&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}},
			&container.ImageInfo{RepoTags: []string{"app:local"}},
			container.ImageSummaryOptions{Image: "app:local"},
		)
	})
}

func TestWritePushedImageSummariesSkipsInvalidAndInspectFailures(t *testing.T) {
	var summaries []string
	prev := writeStepSummaryFn
	writeStepSummaryFn = func(content string) error {
		summaries = append(summaries, content)
		return nil
	}
	defer func() { writeStepSummaryFn = prev }()
	runtime := &pushRuntime{
		imageInfos: map[string]*container.ImageInfo{
			"registry.example.com/app:ok": {
				ID:       "sha256:img",
				RepoTags: []string{"registry.example.com/app:ok"},
			},
		},
		inspectErrs: map[string]error{
			"registry.example.com/app:missing": assert.AnError,
		},
	}

	writePushedImageSummaries(context.Background(), runtime, &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, []*container.PushResult{
		nil,
		{},
		{Image: "registry.example.com/app:missing", Digest: "sha256:missing"},
		{Image: "registry.example.com/app:ok", Digest: "sha256:ok"},
	})

	require.Len(t, summaries, 1)
	assert.Contains(t, summaries[0], "## 🐳 registry.example.com/app:ok")
	assert.Contains(t, summaries[0], "| Digest | `sha256:ok` |")
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
