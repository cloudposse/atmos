package step

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

type pushRuntime struct {
	tagCalls    [][2]string
	pushCalls   []string
	tagErr      error
	pushResults map[string]*container.PushResult
	pushErrs    map[string]error
}

func (r *pushRuntime) Build(context.Context, *container.BuildConfig) error { return nil }
func (r *pushRuntime) Create(context.Context, *container.CreateConfig) (string, error) {
	return "", nil
}
func (r *pushRuntime) Start(context.Context, string) error { return nil }
func (r *pushRuntime) Stop(context.Context, string, time.Duration) error {
	return nil
}
func (r *pushRuntime) Remove(context.Context, string, bool) error { return nil }
func (r *pushRuntime) Inspect(context.Context, string) (*container.Info, error) {
	return nil, nil
}

func (r *pushRuntime) List(context.Context, map[string]string) ([]container.Info, error) {
	return nil, nil
}

func (r *pushRuntime) Exec(context.Context, string, []string, *container.ExecOptions) error {
	return nil
}

func (r *pushRuntime) Shell(context.Context, string, *container.ShellOptions) error {
	// Push never opens a shell; fail loudly if it is unexpectedly invoked.
	return errors.New("unexpected Shell call")
}

func (r *pushRuntime) Attach(context.Context, string, *container.AttachOptions) error {
	return nil
}
func (r *pushRuntime) Pull(context.Context, string) error { return nil }
func (r *pushRuntime) Tag(_ context.Context, source, target string) error {
	r.tagCalls = append(r.tagCalls, [2]string{source, target})
	return r.tagErr
}

func (r *pushRuntime) Push(_ context.Context, image string) (*container.PushResult, error) {
	r.pushCalls = append(r.pushCalls, image)
	return r.pushResults[image], r.pushErrs[image]
}

func (r *pushRuntime) ImageInspect(context.Context, string) (*container.ImageInfo, error) {
	return nil, nil
}

func (r *pushRuntime) Logs(context.Context, string, bool, string, io.Writer, io.Writer) error {
	return nil
}
func (r *pushRuntime) Info(context.Context) (*container.RuntimeInfo, error) { return nil, nil }

func TestRunPushImagesTagsAndPushesResolvedTags(t *testing.T) {
	runtime := &pushRuntime{
		pushResults: map[string]*container.PushResult{
			"registry.example.com/app:v1": {
				Image:  "registry.example.com/app:v1",
				Digest: "sha256:111",
				Output: "pushed v1",
			},
			"registry.example.com/app:latest": {
				Image:  "registry.example.com/app:latest",
				Digest: "sha256:222",
				Output: "pushed latest",
			},
		},
		pushErrs: map[string]error{},
	}

	result, err := runPushImages(context.Background(), runtime, &resolvedPushConfig{Image: "app:local"}, []string{
		"registry.example.com/app:v1",
		"registry.example.com/app:latest",
	})

	require.NoError(t, err)
	assert.Equal(t, [][2]string{
		{"app:local", "registry.example.com/app:v1"},
		{"app:local", "registry.example.com/app:latest"},
	}, runtime.tagCalls)
	assert.Equal(t, []string{"registry.example.com/app:v1", "registry.example.com/app:latest"}, runtime.pushCalls)
	assert.Equal(t, "registry.example.com/app:latest", result.Value)
	assert.Equal(t, "sha256:222", result.Metadata["digest"])
	assert.Equal(t, "pushed v1\npushed latest", result.Metadata["stdout"])
	assert.Equal(t, 0, result.Metadata[exitCodeMetadata])
}

func TestRunPushImagesNoTagsPushesSourceImage(t *testing.T) {
	runtime := &pushRuntime{
		pushResults: map[string]*container.PushResult{
			"app:local": {
				Image:  "app:local",
				Digest: "sha256:source",
				Output: "pushed source",
			},
		},
		pushErrs: map[string]error{},
	}

	result, err := runPushImages(context.Background(), runtime, &resolvedPushConfig{Image: "app:local"}, nil)

	require.NoError(t, err)
	assert.Empty(t, runtime.tagCalls)
	assert.Equal(t, []string{"app:local"}, runtime.pushCalls)
	assert.Equal(t, "app:local", result.Metadata["image"])
	assert.Equal(t, "sha256:source", result.Metadata["digest"])
	assert.Equal(t, "pushed source", result.Metadata["stdout"])
}

func TestRunPushImagesTagError(t *testing.T) {
	tagErr := errors.New("tag failed")
	runtime := &pushRuntime{
		tagErr:      tagErr,
		pushResults: map[string]*container.PushResult{},
		pushErrs:    map[string]error{},
	}

	result, err := runPushImages(context.Background(), runtime, &resolvedPushConfig{Image: "app:local"}, []string{"registry.example.com/app:v1"})

	require.ErrorIs(t, err, tagErr)
	assert.Equal(t, "registry.example.com/app:v1", result.Value)
	assert.Equal(t, 1, result.Metadata[exitCodeMetadata])
	assert.Equal(t, "tag failed", result.Error)
	assert.Empty(t, runtime.pushCalls)
}

func TestRunPushImagesPushErrorKeepsPartialResultMetadata(t *testing.T) {
	pushErr := errors.New("push failed")
	runtime := &pushRuntime{
		pushResults: map[string]*container.PushResult{
			"app:local": {
				Image:  "app:local",
				Digest: "sha256:partial",
				Output: "partial output",
			},
		},
		pushErrs: map[string]error{"app:local": pushErr},
	}

	result, err := runPushImages(context.Background(), runtime, &resolvedPushConfig{Image: "app:local"}, nil)

	require.ErrorIs(t, err, pushErr)
	assert.Equal(t, "app:local", result.Metadata["image"])
	assert.Equal(t, "sha256:partial", result.Metadata["digest"])
	assert.Equal(t, "partial output", result.Metadata["stdout"])
	assert.Equal(t, 1, result.Metadata[exitCodeMetadata])
	assert.Equal(t, "push failed", result.Error)
}

func TestPreviewPush(t *testing.T) {
	result := previewPush("", &resolvedPushConfig{Image: "app:local"}, []string{"registry.example.com/app:v1"})

	assert.Equal(t, "app:local", result.Value)
	assert.Equal(t, "app:local", result.Metadata["image"])
	assert.Equal(t, 0, result.Metadata[exitCodeMetadata])

	noTags := previewPush("podman", &resolvedPushConfig{Image: "app:local"}, nil)
	assert.Equal(t, "app:local", noTags.Value)
	assert.Equal(t, "app:local", noTags.Metadata["image"])
}

func TestContainerHandlerExecuteDryRunActions(t *testing.T) {
	handler := &ContainerHandler{}
	vars := NewVariables()

	runResult, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name:    "run",
		Type:    "container",
		DryRun:  true,
		Image:   "alpine:latest",
		Command: "echo ok",
	}, vars)
	require.NoError(t, err)
	assert.Contains(t, runResult.Value, "docker|podman run --rm")
	assert.Equal(t, 0, runResult.Metadata[exitCodeMetadata])

	buildResult, err := handler.ExecuteWithWorkflow(context.Background(), &schema.WorkflowStep{
		Name:   "build",
		Type:   "container",
		Action: "build",
		DryRun: true,
		Build: &schema.ContainerBuildStep{
			Context:    ".",
			Dockerfile: "Dockerfile",
			Tags:       []string{"app:test"},
		},
	}, vars, nil)
	require.NoError(t, err)
	assert.Equal(t, "app:test", buildResult.Value)
	assert.Equal(t, "app:test", buildResult.Metadata["image"])
	assert.Equal(t, 0, buildResult.Metadata[exitCodeMetadata])

	pushResult, err := handler.ExecuteWithWorkflow(context.Background(), &schema.WorkflowStep{
		Name:   "push",
		Type:   "container",
		Action: "push",
		DryRun: true,
		Push: &schema.ContainerPushStep{
			Image: "app:test",
			Tags:  []string{"registry.example.com/app:test"},
		},
	}, vars, nil)
	require.NoError(t, err)
	assert.Equal(t, "app:test", pushResult.Value)
	assert.Equal(t, "app:test", pushResult.Metadata["image"])
	assert.Equal(t, 0, pushResult.Metadata[exitCodeMetadata])
}
