package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
)

func TestWorkflowOptions_GetGlobalFlags(t *testing.T) {
	opts := &WorkflowOptions{
		StandardOptions: flags.StandardOptions{
			Flags: global.Flags{
				BasePath: "/test/path",
			},
		},
		WorkflowName: "deploy",
	}

	globalFlags := opts.GetGlobalFlags()

	require.NotNil(t, globalFlags)
	assert.Equal(t, "/test/path", globalFlags.BasePath)
}

func TestWorkflowOptions_GetPositionalArgs(t *testing.T) {
	opts := &WorkflowOptions{
		WorkflowName: "deploy",
	}

	// Set positional args
	opts.SetPositionalArgs([]string{"deploy", "arg1", "arg2"})

	positionalArgs := opts.GetPositionalArgs()

	require.NotNil(t, positionalArgs)
	assert.Len(t, positionalArgs, 3)
	assert.Equal(t, "deploy", positionalArgs[0])
	assert.Equal(t, "arg1", positionalArgs[1])
	assert.Equal(t, "arg2", positionalArgs[2])
}

func TestWorkflowOptions_GetPositionalArgs_Empty(t *testing.T) {
	opts := &WorkflowOptions{
		WorkflowName: "",
	}

	// Initialize with empty slice
	opts.SetPositionalArgs([]string{})

	positionalArgs := opts.GetPositionalArgs()

	assert.NotNil(t, positionalArgs)
	assert.Empty(t, positionalArgs)
}

func TestWorkflowOptions_GetSeparatedArgs(t *testing.T) {
	opts := &WorkflowOptions{
		WorkflowName: "deploy",
	}

	separatedArgs := opts.GetSeparatedArgs()

	assert.NotNil(t, separatedArgs)
	assert.Empty(t, separatedArgs) // Workflow commands don't use separated args
}

func TestWorkflowOptions_EmbeddedFields(t *testing.T) {
	opts := &WorkflowOptions{
		StandardOptions: flags.StandardOptions{
			Stack:  "prod",
			File:   "workflows.yaml",
			DryRun: true,
			Flags: global.Flags{
				BasePath: "/test",
			},
		},
		WorkflowName: "deploy",
		FromStep:     "step2",
	}

	// Test direct access to embedded StandardOptions fields
	assert.Equal(t, "prod", opts.Stack)
	assert.Equal(t, "workflows.yaml", opts.File)
	assert.True(t, opts.DryRun)
	assert.Equal(t, "/test", opts.BasePath)

	// Test workflow-specific fields
	assert.Equal(t, "deploy", opts.WorkflowName)
	assert.Equal(t, "step2", opts.FromStep)
}

func TestWorkflowOptions_DefaultValues(t *testing.T) {
	opts := &WorkflowOptions{}

	// Test default values
	assert.Equal(t, "", opts.WorkflowName)
	assert.Equal(t, "", opts.FromStep)
	assert.Equal(t, "", opts.Stack)
	assert.Equal(t, "", opts.File)
	assert.False(t, opts.DryRun)
}

func TestWorkflowOptions_SetPositionalArgs(t *testing.T) {
	opts := &WorkflowOptions{
		WorkflowName: "deploy",
	}

	args := []string{"deploy", "extra1", "extra2"}
	opts.SetPositionalArgs(args)

	retrievedArgs := opts.GetPositionalArgs()
	assert.Equal(t, args, retrievedArgs)
}
