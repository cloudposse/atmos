package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildSpinnerMessage(t *testing.T) {
	assert.Equal(t, "Building image alpine:latest", buildSpinnerMessage("Building image", "alpine:latest"))
	assert.Equal(t, "Building image", buildSpinnerMessage("Building image", "")) // tagless bake build
}

func TestValidateBuildAction(t *testing.T) {
	// Valid: plain build, no engine.
	require.NoError(t, validateBuildAction(&schema.WorkflowStep{
		Build: &schema.ContainerBuildStep{Context: ".", Tags: []string{"app:local"}},
	}))

	tests := []struct {
		name string
		step *schema.WorkflowStep
	}{
		{"bad runtime", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Runtime: "containerd"}}},
		{"bad engine", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Engine: "kaniko"}}},
		{"buildx requires docker", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Engine: "buildx", Runtime: "podman"}}},
		{"bake requires docker", &schema.WorkflowStep{Build: &schema.ContainerBuildStep{Runtime: "podman", Bake: &schema.ContainerBuildBakeStep{File: "x"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, validateBuildAction(tt.step))
		})
	}
}

func TestResolveBuildBake(t *testing.T) {
	vars := NewVariables()

	got, err := resolveBuildBake(vars, nil, "step")
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = resolveBuildBake(vars, &schema.ContainerBuildBakeStep{
		File:    "docker-bake.hcl",
		Target:  "app",
		Targets: []string{"app", "test"},
		Set:     []string{"app.tags=x"},
		Vars:    map[string]string{"TAG": "v1"},
		Load:    true,
		Push:    true,
	}, "step")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "docker-bake.hcl", got.File)
	assert.Equal(t, "app", got.Target)
	assert.Equal(t, []string{"app", "test"}, got.Targets)
	assert.Equal(t, "v1", got.Vars["TAG"])
	assert.True(t, got.Load)
	assert.True(t, got.Push)
}

func TestDefaultMountType(t *testing.T) {
	assert.Equal(t, "bind", defaultMountType(""))
	assert.Equal(t, "volume", defaultMountType("volume"))
}

func TestEffectiveBuildStep(t *testing.T) {
	assert.Equal(t, schema.ContainerBuildStep{}, effectiveBuildStep(&schema.WorkflowStep{}))
	build := &schema.ContainerBuildStep{Context: ".", Tags: []string{"app:local"}}
	assert.Equal(t, *build, effectiveBuildStep(&schema.WorkflowStep{Build: build}))
}

func TestValidateInspectAction(t *testing.T) {
	h := &ContainerHandler{}

	// Valid via the inspect block and via the flat image shorthand.
	require.NoError(t, h.validateInspectAction(&schema.WorkflowStep{Inspect: &schema.ContainerInspectStep{Image: "alpine"}}))
	require.NoError(t, h.validateInspectAction(&schema.WorkflowStep{Image: "alpine"}))

	// Missing image.
	require.Error(t, h.validateInspectAction(&schema.WorkflowStep{}))

	// Bad runtime.
	require.Error(t, h.validateInspectAction(&schema.WorkflowStep{
		Inspect: &schema.ContainerInspectStep{Image: "alpine", Runtime: "containerd"},
	}))
}

func TestExecuteInspect_DryRun(t *testing.T) {
	h := &ContainerHandler{}
	vars := NewVariables()

	res, err := h.executeInspect(context.Background(), &schema.WorkflowStep{
		Name:    "inspect",
		Inspect: &schema.ContainerInspectStep{Image: "alpine:latest"},
		DryRun:  true,
	}, vars)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "alpine:latest", res.Metadata["image"])
}

func TestResolveWorkDir(t *testing.T) {
	vars := NewVariables()

	// Explicit working_directory is resolved and made absolute.
	got, err := resolveWorkDir(vars, &schema.WorkflowStep{WorkingDirectory: "subdir"})
	require.NoError(t, err)
	assert.True(t, len(got) > 0)

	// Empty falls back to the current working directory (absolute).
	got, err = resolveWorkDir(vars, &schema.WorkflowStep{})
	require.NoError(t, err)
	assert.NotEmpty(t, got)
}

func TestEffectiveRunStepMergesShorthand(t *testing.T) {
	// Flat shorthand fields are folded into the run config.
	run := effectiveRunStep(&schema.WorkflowStep{
		Image:   "alpine",
		Command: "echo hi",
		Shell:   "/bin/bash",
		Mounts:  []schema.ContainerMount{{Source: "/h", Target: "/c"}},
	})
	assert.Equal(t, "alpine", run.Image)
	assert.Equal(t, "echo hi", run.Command)
	assert.Equal(t, "/bin/bash", run.Shell)
	require.Len(t, run.Mounts, 1)

	// An explicit run block wins over the shorthand.
	run = effectiveRunStep(&schema.WorkflowStep{
		Image: "flat",
		Run:   &schema.ContainerRunStep{Image: "explicit"},
	})
	assert.Equal(t, "explicit", run.Image)
}

func TestConvertContainerPorts(t *testing.T) {
	ports := convertContainerPorts([]schema.ContainerPort{
		{Host: 8080, Container: 80},
		{Host: 53, Container: 53, Protocol: "udp"},
	})
	require.Len(t, ports, 2)
	assert.Equal(t, "tcp", ports[0].Protocol)
	assert.Equal(t, "udp", ports[1].Protocol)
}

func TestEffectiveInspectStepRuntimeShorthand(t *testing.T) {
	got := effectiveInspectStep(&schema.WorkflowStep{Image: "alpine", Runtime: "podman", RuntimeAutoStart: true})
	assert.Equal(t, "alpine", got.Image)
	assert.Equal(t, "podman", got.Runtime)
	assert.True(t, got.RuntimeAutoStart)

	// Ensure the BakeConfig type is referenced so a field rename fails the build.
	_ = container.BakeConfig{}
}
