package workflow

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

type fakeContainer struct {
	id      string
	name    string
	command []string
	opts    *container.ExecOptions
}

func (f *fakeContainer) ID() string {
	return f.id
}

func (f *fakeContainer) Name() string {
	return f.name
}

func (f *fakeContainer) Exec(_ context.Context, command []string, opts *container.ExecOptions) error {
	f.command = command
	f.opts = opts
	if opts.Stdout != nil {
		_, _ = io.WriteString(opts.Stdout, "ok\n")
	}
	if opts.Stderr != nil {
		_, _ = io.WriteString(opts.Stderr, "warning\n")
	}
	return nil
}

func (f *fakeContainer) Cleanup(bool) error {
	return nil
}

func TestContainerSessionExecShellCapturesOutput(t *testing.T) {
	fake := &fakeContainer{id: "container-id", name: "sandbox"}
	session := &ContainerSession{
		backend:       fake,
		config:        &schema.WorkflowContainer{Workspace: "/workspace"},
		hostWorkspace: "/repo",
	}
	var stdout, stderr bytes.Buffer

	err := session.ExecShell(context.Background(), &ContainerStepParams{
		Step:          &schema.WorkflowStep{Name: "capture", Type: "shell", Command: "echo ok"},
		WorkflowDef:   &schema.WorkflowDefinition{Output: "none"},
		HostWorkDir:   "/repo",
		Command:       "echo ok",
		StdoutCapture: &stdout,
		StderrCapture: &stderr,
	})

	require.NoError(t, err)
	assert.Equal(t, "ok\n", stdout.String())
	assert.Equal(t, "warning\n", stderr.String())
}

func TestContainerSessionExecShellTerminalSessionDoesNotCaptureOutput(t *testing.T) {
	for _, tc := range []struct {
		name        string
		tty         bool
		interactive bool
	}{
		{name: "tty", tty: true},
		{name: "interactive", interactive: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeContainer{id: "container-id", name: "sandbox"}
			session := &ContainerSession{
				backend:       fake,
				config:        &schema.WorkflowContainer{Workspace: "/workspace"},
				hostWorkspace: "/repo",
			}
			var stdout, stderr bytes.Buffer

			err := session.ExecShell(context.Background(), &ContainerStepParams{
				Step: &schema.WorkflowStep{
					Name:        "session",
					Type:        "shell",
					Command:     "interactive",
					Tty:         tc.tty,
					Interactive: tc.interactive,
				},
				WorkflowDef:   &schema.WorkflowDefinition{Output: "none"},
				HostWorkDir:   "/repo",
				Command:       "interactive",
				StdoutCapture: &stdout,
				StderrCapture: &stderr,
			})

			require.NoError(t, err)
			require.NotNil(t, fake.opts)
			assert.Equal(t, tc.tty, fake.opts.Tty)
			assert.Equal(t, tc.interactive, fake.opts.AttachStdin)
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestContainerSessionExecShellPassesStepEnvAndMappedWorkingDirectory(t *testing.T) {
	fake := &fakeContainer{id: "container-id", name: "sandbox"}
	session := &ContainerSession{
		backend: fake,
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

	err := session.ExecShell(context.Background(), &ContainerStepParams{
		Step:        step,
		WorkflowDef: workflowDef,
		HostWorkDir: "/repo/services/api",
		Command:     "env",
		StepEnv:     stepEnv,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"/bin/bash", "-lc", "env"}, fake.command)
	assert.Equal(t, "/workspace/services/api", fake.opts.WorkingDir)
	assert.Contains(t, fake.opts.Env, "CONTAINER_ONLY=yes")
	assert.Contains(t, fake.opts.Env, "SHARED=step")
	assert.Contains(t, fake.opts.Env, "STEP=1")
	assert.NotContains(t, fake.opts.Env, "SHARED=container")
}

func TestContainerSessionExecShellRejectsWorkingDirectoryOutsideWorkspace(t *testing.T) {
	fake := &fakeContainer{id: "container-id", name: "sandbox"}
	session := &ContainerSession{
		backend:       fake,
		config:        &schema.WorkflowContainer{Workspace: "/workspace"},
		hostWorkspace: "/repo",
	}
	step := &schema.WorkflowStep{Name: "test", Type: "shell", Command: "pwd"}
	workflowDef := &schema.WorkflowDefinition{Output: "none"}

	err := session.ExecShell(context.Background(), &ContainerStepParams{
		Step:        step,
		WorkflowDef: workflowDef,
		HostWorkDir: "/tmp/outside",
		Command:     "pwd",
	})

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
