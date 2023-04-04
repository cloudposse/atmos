package workflow

import (
	"github.com/stretchr/testify/assert"
	"testing"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkflowCommand(t *testing.T) {
	cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	workflow := "test-1"
	workflowPath := "stacks/workflows/workflow1.yaml"

	workflowDefinition := schema.WorkflowDefinition{
		Description: "Test workflow 1",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Type:    "shell",
				Command: "echo 1",
			},
			{
				Name:    "step2",
				Type:    "shell",
				Command: "echo 2",
			},
			{
				Name:    "",
				Type:    "shell",
				Command: "echo 3",
			},
			{
				Name:    "",
				Type:    "shell",
				Command: "echo 4",
			},
		},
	}

	err = e.ExecuteWorkflow(
		cliConfig,
		workflow,
		workflowPath,
		&workflowDefinition,
		false,
		"",
		// `step3` name is not defined in the workflow, so we auto-generate a friendly name consisting of
		// a prefix of `step` and followed by the index of the step (the index starts with 1, so the first generated step name would be `step1`)
		"step3",
	)
	assert.Nil(t, err)

	err = e.ExecuteWorkflow(
		cliConfig,
		workflow,
		workflowPath,
		&workflowDefinition,
		false,
		"",
		// The workflow does not have 5 steps, we we should get an error
		"step5",
	)
	assert.Error(t, err)
}
