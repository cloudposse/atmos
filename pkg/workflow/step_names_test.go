package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCheckAndGenerateWorkflowStepNamesAssignsMissingNames(t *testing.T) {
	def := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Command: "echo a"},
			{Command: "echo b"},
		},
	}

	CheckAndGenerateWorkflowStepNames(def)

	assert.Equal(t, "step1", def.Steps[0].Name)
	assert.Equal(t, "step2", def.Steps[1].Name)
}

func TestCheckAndGenerateWorkflowStepNamesAvoidsCollisionWithExplicitSibling(t *testing.T) {
	def := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step2", Command: "echo explicit"},
			{Command: "echo unnamed"},
		},
	}

	CheckAndGenerateWorkflowStepNames(def)

	// The unnamed step at index 1 would naturally generate "step2", which
	// collides with the explicit sibling name at index 0. It must get a
	// distinct name instead, since step results are keyed by name.
	assert.Equal(t, "step2", def.Steps[0].Name)
	assert.NotEqual(t, "step2", def.Steps[1].Name)
	assert.Equal(t, "step2_", def.Steps[1].Name)
}

func TestCheckAndGenerateWorkflowStepNamesNestedStepsUseParentPrefix(t *testing.T) {
	def := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name: "fanout",
				Type: schema.TaskTypeParallel,
				Steps: []schema.WorkflowStep{
					{Command: "echo a"},
					{Name: "fanout_step2", Command: "echo explicit"},
					{Command: "echo c"},
				},
			},
		},
	}

	CheckAndGenerateWorkflowStepNames(def)

	nested := def.Steps[0].Steps
	assert.Equal(t, "fanout_step1", nested[0].Name)
	assert.Equal(t, "fanout_step2", nested[1].Name)
	// index+1 for the third step would also generate "fanout_step3", which is
	// not taken, so it should be assigned as-is.
	assert.Equal(t, "fanout_step3", nested[2].Name)
}
