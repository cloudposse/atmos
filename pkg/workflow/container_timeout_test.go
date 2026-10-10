package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// blockingContainer runs every command until its context ends, like a long process.
type blockingContainer struct {
	fakeContainer
}

func (b *blockingContainer) Exec(ctx context.Context, _ []string, _ *container.ExecOptions) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestContainerExecShellHonorsStepTimeout(t *testing.T) {
	backend := &blockingContainer{fakeContainer{id: "container-id", name: "sandbox"}}
	session := &ContainerSession{
		backend:       backend,
		config:        &schema.WorkflowContainer{Workspace: "/workspace"},
		hostWorkspace: "/repo",
	}
	step := &schema.WorkflowStep{Name: "slow", Type: "shell", Command: "sleep 30", Timeout: "20ms"}

	// Workflow container steps run ExecShell under the step deadline the same way.
	err := stepPkg.RunWithStepDeadline(context.Background(), step, nil, func(ctx context.Context) error {
		return session.ExecShell(ctx, &ContainerStepParams{
			Step:        step,
			WorkflowDef: &schema.WorkflowDefinition{Output: "none"},
			HostWorkDir: "/repo",
			Command:     step.Command,
		})
	})

	require.ErrorIs(t, err, errUtils.ErrStepTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestContainerExecShellWithoutTimeoutRunsToCompletion(t *testing.T) {
	session := &ContainerSession{
		backend:       &fakeContainer{id: "container-id", name: "sandbox"},
		config:        &schema.WorkflowContainer{Workspace: "/workspace"},
		hostWorkspace: "/repo",
	}
	step := &schema.WorkflowStep{Name: "quick", Type: "shell", Command: "echo ok"}

	err := stepPkg.RunWithStepDeadline(context.Background(), step, nil, func(ctx context.Context) error {
		return session.ExecShell(ctx, &ContainerStepParams{
			Step:        step,
			WorkflowDef: &schema.WorkflowDefinition{Output: "none"},
			HostWorkDir: "/repo",
			Command:     step.Command,
		})
	})

	require.NoError(t, err)
}
