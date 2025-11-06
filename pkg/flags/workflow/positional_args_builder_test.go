package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkflowPositionalArgsBuilder_WithWorkflowName_Required(t *testing.T) {
	builder := NewWorkflowPositionalArgsBuilder()
	builder.WithWorkflowName(true)

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 1)
	assert.Equal(t, "name", specs[0].Name)
	assert.Equal(t, "WorkflowName", specs[0].TargetField)
	assert.True(t, specs[0].Required)
	assert.Equal(t, "Workflow name", specs[0].Description)

	// Check usage string
	assert.Equal(t, "<name>", usage)

	// Check validator requires exactly 1 arg
	err := validator(nil, []string{"deploy"})
	assert.NoError(t, err)

	err = validator(nil, []string{})
	assert.Error(t, err)

	err = validator(nil, []string{"deploy", "test"})
	assert.Error(t, err)
}

func TestWorkflowPositionalArgsBuilder_WithWorkflowName_Optional(t *testing.T) {
	builder := NewWorkflowPositionalArgsBuilder()
	builder.WithWorkflowName(false)

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 1)
	assert.Equal(t, "name", specs[0].Name)
	assert.Equal(t, "WorkflowName", specs[0].TargetField)
	assert.False(t, specs[0].Required)

	// Check usage string
	assert.Equal(t, "[name]", usage)

	// Check validator accepts 0 or 1 args
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"deploy"})
	assert.NoError(t, err)

	err = validator(nil, []string{"deploy", "test"})
	assert.Error(t, err)
}

func TestWorkflowPositionalArgsBuilder_FluentInterface(t *testing.T) {
	// Test that methods return builder for chaining
	builder := NewWorkflowPositionalArgsBuilder()
	result := builder.WithWorkflowName(true)

	assert.Equal(t, builder, result, "WithWorkflowName should return builder for chaining")
}

func TestWorkflowPositionalArgsBuilder_RealWorldUsage(t *testing.T) {
	// Simulate real usage in workflow command
	_, workflowValidator, workflowUsage := NewWorkflowPositionalArgsBuilder().
		WithWorkflowName(true).
		Build()

	// Check usage matches expected pattern for workflow command
	assert.Equal(t, "<name>", workflowUsage)

	// Test validator with real workflow scenarios
	err := workflowValidator(nil, []string{"deploy"})
	assert.NoError(t, err, "should accept: atmos workflow deploy")

	err = workflowValidator(nil, []string{})
	assert.Error(t, err, "should reject: atmos workflow (missing workflow name)")
}
