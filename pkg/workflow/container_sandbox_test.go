package workflow

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

type fakeSandbox struct {
	id      string
	name    string
	command []string
	opts    *container.ExecOptions
}

func (f *fakeSandbox) ID() string {
	return f.id
}

func (f *fakeSandbox) Name() string {
	return f.name
}

func (f *fakeSandbox) Exec(_ context.Context, command []string, opts *container.ExecOptions) error {
	f.command = command
	f.opts = opts
	if opts.Stdout != nil {
		_, _ = io.WriteString(opts.Stdout, "ok\n")
	}
	return nil
}

func (f *fakeSandbox) Cleanup(bool) error {
	return nil
}

func TestWorkflowSandboxExecShellPassesStepEnvAndMappedWorkingDirectory(t *testing.T) {
	fake := &fakeSandbox{id: "container-id", name: "sandbox"}
	sandbox := &WorkflowSandbox{
		sandbox: fake,
		config: &schema.WorkflowContainer{
			Shell:     "/bin/bash",
			Workspace: "/workspace",
			Env: map[string]string{
				"CONTAINER_ONLY": "yes",
				"SHARED":         "container",
			},
		},
		hostWorkspace: "/repo",
	}
	workflowDef := &schema.WorkflowDefinition{
		Output: "none",
		Env: map[string]string{
			"SHARED": "workflow",
		},
	}
	step := &schema.WorkflowStep{
		Name:    "test",
		Type:    "shell",
		Command: "env",
		Env: map[string]string{
			"SHARED": "step",
			"STEP":   "1",
		},
	}
	stepEnv := []string{"SHARED=step", "STEP=1"}

	err := sandbox.ExecShell(context.Background(), step, workflowDef, "/repo/services/api", "env", stepEnv)

	require.NoError(t, err)
	assert.Equal(t, []string{"/bin/bash", "-lc", "env"}, fake.command)
	assert.Equal(t, "/workspace/services/api", fake.opts.WorkingDir)
	assert.Contains(t, fake.opts.Env, "CONTAINER_ONLY=yes")
	assert.Contains(t, fake.opts.Env, "SHARED=step")
	assert.Contains(t, fake.opts.Env, "STEP=1")
	assert.NotContains(t, fake.opts.Env, "SHARED=container")
}

func TestWorkflowSandboxExecShellRejectsWorkingDirectoryOutsideWorkspace(t *testing.T) {
	fake := &fakeSandbox{id: "container-id", name: "sandbox"}
	sandbox := &WorkflowSandbox{
		sandbox:       fake,
		config:        &schema.WorkflowContainer{Workspace: "/workspace"},
		hostWorkspace: "/repo",
	}
	step := &schema.WorkflowStep{Name: "test", Type: "shell", Command: "pwd"}
	workflowDef := &schema.WorkflowDefinition{Output: "none"}

	err := sandbox.ExecShell(context.Background(), step, workflowDef, "/tmp/outside", "pwd", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside workflow container workspace")
}

func TestMergeWorkflowAndStepContainerEnvOverridePolicy(t *testing.T) {
	base := &schema.WorkflowContainer{
		Image: "alpine",
		Env: map[string]string{
			"SHARED": "workflow",
		},
	}
	override := &schema.WorkflowContainer{
		Image: "node",
		Env: map[string]string{
			"SHARED": "step",
		},
	}

	merged := mergeWorkflowContainer(base, override)

	assert.Equal(t, "node", merged.Image)
	assert.Equal(t, map[string]string{"SHARED": "step"}, merged.Env)
}
