package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteWorkflow(t *testing.T) {
	stacksPath := "../../../tests/fixtures/scenarios/workflows"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	workflowsDir := stacksPath + "/stacks/workflows"
	workflowPath := workflowsDir + "/test.yaml"

	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	assert.NoError(t, err, "'InitCliConfig' should execute without error")

	tests := []struct {
		name             string
		workflow         string
		workflowPath     string
		workflowDef      *schema.WorkflowDefinition
		dryRun           bool
		commandLineStack string
		fromStep         string
		wantErr          bool
		errMsg           string
	}{
		{
			name:         "valid workflow execution",
			workflow:     "test-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          false,
		},
		{
			name:         "empty workflow",
			workflow:     "no-steps",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "workflow has no steps defined",
		},
		{
			name:         "invalid step type",
			workflow:     "invalid-step",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "invalid",
						Command: "echo 'Step 1'",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "invalid workflow step type",
		},
		{
			name:         "invalid from-step",
			workflow:     "test-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "nonexistent-step",
			wantErr:          true,
			errMsg:           "invalid from-step flag",
		},
		{
			name:         "failing shell command",
			workflow:     "failing-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "exit 1",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "workflow step execution failed",
		},
		{
			name:         "failing atmos command",
			workflow:     "failing-atmos-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "atmos",
						Command: "invalid-command",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "workflow step execution failed",
		},
		{
			name:         "workflow with stack override",
			workflow:     "stack-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Stack: "prod",
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "dev",
			fromStep:         "",
			wantErr:          false,
		},
		{
			name:         "failing atmos command with stack",
			workflow:     "failing-atmos-with-stack",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Stack: "prod",
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "atmos",
						Command: "terraform plan mock -s idontexist",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "workflow step execution failed",
		},
		{
			name:         "failing atmos command with command line stack override",
			workflow:     "failing-atmos-with-cli-stack",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Stack: "prod",
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "atmos",
						Command: "terraform plan mock -s idontexist",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "dev",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "workflow step execution failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExecuteWorkflow(
				config,
				tt.workflow,
				tt.workflowPath,
				tt.workflowDef,
				tt.dryRun,
				tt.commandLineStack,
				tt.fromStep,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckAndGenerateWorkflowStepNames(t *testing.T) {
	tests := []struct {
		name     string
		input    *schema.WorkflowDefinition
		expected *schema.WorkflowDefinition
	}{
		{
			name: "steps with names",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
			expected: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
		},
		{
			name: "steps without names",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
			expected: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
		},
		{
			name: "mixed steps",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "custom-step",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
			expected: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "custom-step",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkAndGenerateWorkflowStepNames(tt.input)
			assert.Equal(t, tt.expected, tt.input)
		})
	}
}
