package step

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The dry-run branches of each action build their config and render a preview
// without ever touching a real container runtime, so they are fully unit-testable.

func TestExecuteRun_DryRun(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()

	res, err := h.executeRun(context.Background(), &schema.WorkflowStep{
		Name:   "run",
		Run:    &schema.ContainerRunStep{Image: "alpine:latest", Command: "echo hi"},
		DryRun: true,
	}, vars, nil)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 0, res.Metadata[exitCodeMetadata])
}

func TestExecuteBuild_DryRun(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()

	res, err := h.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Context: ".",
			Tags:    []string{"app:local"},
		},
		DryRun: true,
	}, vars)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "app:local", res.Metadata["image"])
}

func TestExecutePush_DryRun(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()

	res, err := h.executePush(context.Background(), &schema.WorkflowStep{
		Name: "push",
		Push: &schema.ContainerPushStep{
			Image: "app:local",
			Tags:  []string{"registry.example.com/app:local"},
		},
		DryRun: true,
	}, vars)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "app:local", res.Metadata["image"])
}

func TestValidatePushAction(t *testing.T) {
	h := &ContainerHandler{}

	require.NoError(t, h.validatePushAction(&schema.WorkflowStep{
		Push: &schema.ContainerPushStep{Image: "app:local"},
	}))

	// Missing image.
	require.Error(t, h.validatePushAction(&schema.WorkflowStep{Push: &schema.ContainerPushStep{}}))

	// Bad runtime.
	require.Error(t, h.validatePushAction(&schema.WorkflowStep{
		Push: &schema.ContainerPushStep{Image: "app:local", Provider: "containerd"},
	}))
}

func TestEffectivePushStep(t *testing.T) {
	assert.Equal(t, schema.ContainerPushStep{}, effectivePushStep(&schema.WorkflowStep{}))
	push := &schema.ContainerPushStep{Image: "app:local", Tags: []string{"a"}}
	assert.Equal(t, *push, effectivePushStep(&schema.WorkflowStep{Push: push}))
}

func TestMetadataString(t *testing.T) {
	assert.Equal(t, "", metadataString(nil, "digest"))
	result := &container.PushResult{Image: "app:local", Digest: "sha256:abc"}
	assert.Equal(t, "sha256:abc", metadataString(result, "digest"))
	assert.Equal(t, "app:local", metadataString(result, "image"))
	assert.Equal(t, "", metadataString(result, "unknown"))
}

func TestContainerStepName(t *testing.T) {
	assert.True(t, strings.HasPrefix(containerStepName(""), "atmos-step-step-"))
	assert.True(t, strings.HasPrefix(containerStepName("Build/Scan"), "atmos-step-Build-Scan-"))
}

func TestStepDefaultString(t *testing.T) {
	assert.Equal(t, "fallback", defaultString("", "fallback"))
	assert.Equal(t, "value", defaultString("value", "fallback"))
}

func TestExpandHostPath(t *testing.T) {
	// Home expansion.
	assert.True(t, strings.HasSuffix(filepath.ToSlash(expandHostPath("~/sub")), "/sub"))
	assert.NotContains(t, expandHostPath("~/sub"), "~")

	// Env expansion.
	t.Setenv("ATMOS_TEST_HOSTPATH", "/expanded")
	assert.Equal(t, "/expanded/x", expandHostPath("$ATMOS_TEST_HOSTPATH/x"))

	// Plain path is unchanged.
	assert.Equal(t, "/plain/path", expandHostPath("/plain/path"))
}

func TestResolveRunCommand(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()

	// Explicit run.command is resolved.
	got, err := h.resolveRunCommand(context.Background(), &schema.WorkflowStep{Name: "s"}, vars,
		&schema.ContainerRunStep{Command: "echo hi"})
	require.NoError(t, err)
	assert.Equal(t, "echo hi", got)
}

func TestWriteOutput(t *testing.T) {
	h := &ContainerHandler{}
	// OutputModeNone is a no-op (no stream writes).
	assert.NotPanics(t, func() {
		h.writeOutput(&schema.WorkflowStep{}, &schema.WorkflowDefinition{Output: "none"}, "out", "err")
	})

	// Other modes write the captured streams; initialize the data writer first.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	assert.NotPanics(t, func() {
		h.writeOutput(&schema.WorkflowStep{Name: "s"}, &schema.WorkflowDefinition{Output: "log"}, "out", "err")
	})
}
