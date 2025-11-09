package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestPositionalArgsBuilder_SingleRequired(t *testing.T) {
	builder := NewPositionalArgsBuilder()
	builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Description: "Component name",
		Required:    true,
		TargetField: "Component",
	})

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 1)
	assert.Equal(t, "component", specs[0].Name)
	assert.Equal(t, "Component", specs[0].TargetField)
	assert.True(t, specs[0].Required)

	// Check usage string
	assert.Equal(t, "<component>", usage)

	// Check validator accepts exactly 1 arg
	err := validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	// Check validator rejects 0 args
	err = validator(nil, []string{})
	assert.Error(t, err)

	// Check validator rejects 2 args
	err = validator(nil, []string{"vpc", "dev"})
	assert.Error(t, err)
}

func TestPositionalArgsBuilder_SingleOptional(t *testing.T) {
	builder := NewPositionalArgsBuilder()
	builder.AddArg(&PositionalArgSpec{
		Name:        "workflow",
		Description: "Workflow name",
		Required:    false,
		TargetField: "WorkflowName",
	})

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 1)
	assert.Equal(t, "workflow", specs[0].Name)
	assert.Equal(t, "WorkflowName", specs[0].TargetField)
	assert.False(t, specs[0].Required)

	// Check usage string
	assert.Equal(t, "[workflow]", usage)

	// Check validator accepts 0 args (optional)
	err := validator(nil, []string{})
	assert.NoError(t, err)

	// Check validator accepts 1 arg
	err = validator(nil, []string{"deploy"})
	assert.NoError(t, err)

	// Check validator rejects 2 args (max 1)
	err = validator(nil, []string{"deploy", "test"})
	assert.Error(t, err)
}

func TestPositionalArgsBuilder_MultipleArgs(t *testing.T) {
	builder := NewPositionalArgsBuilder()
	builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Description: "Component name",
		Required:    true,
		TargetField: "Component",
	})
	builder.AddArg(&PositionalArgSpec{
		Name:        "stack",
		Description: "Stack name",
		Required:    false,
		TargetField: "Stack",
	})

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 2)

	// Check usage string
	assert.Equal(t, "<component> [stack]", usage)

	// Check validator accepts 1 arg (min required)
	err := validator(nil, []string{"vpc"})
	assert.NoError(t, err)

	// Check validator accepts 2 args (max total)
	err = validator(nil, []string{"vpc", "dev"})
	assert.NoError(t, err)

	// Check validator rejects 0 args (min required is 1)
	err = validator(nil, []string{})
	assert.Error(t, err)

	// Check validator rejects 3 args (max total is 2)
	err = validator(nil, []string{"vpc", "dev", "extra"})
	assert.Error(t, err)
}

func TestPositionalArgsBuilder_Empty(t *testing.T) {
	builder := NewPositionalArgsBuilder()
	specs, validator, usage := builder.Build()

	// Check empty specs
	assert.Len(t, specs, 0)

	// Check empty usage
	assert.Equal(t, "", usage)

	// Check validator accepts arbitrary args
	err := validator(nil, []string{})
	assert.NoError(t, err)
	err = validator(nil, []string{"arg1", "arg2", "arg3"})
	assert.NoError(t, err)
}

func TestPositionalArgsBuilder_AllRequired(t *testing.T) {
	builder := NewPositionalArgsBuilder()
	builder.AddArg(&PositionalArgSpec{
		Name:        "arg1",
		Required:    true,
		TargetField: "Arg1",
	})
	builder.AddArg(&PositionalArgSpec{
		Name:        "arg2",
		Required:    true,
		TargetField: "Arg2",
	})

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 2)

	// Check usage
	assert.Equal(t, "<arg1> <arg2>", usage)

	// Check validator requires exactly 2 args
	err := validator(nil, []string{"val1", "val2"})
	assert.NoError(t, err)

	err = validator(nil, []string{"val1"})
	assert.Error(t, err)

	err = validator(nil, []string{"val1", "val2", "val3"})
	assert.Error(t, err)
}

func TestPositionalArgsBuilder_AllOptional(t *testing.T) {
	builder := NewPositionalArgsBuilder()
	builder.AddArg(&PositionalArgSpec{
		Name:        "arg1",
		Required:    false,
		TargetField: "Arg1",
	})
	builder.AddArg(&PositionalArgSpec{
		Name:        "arg2",
		Required:    false,
		TargetField: "Arg2",
	})

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 2)

	// Check usage
	assert.Equal(t, "[arg1] [arg2]", usage)

	// Check validator accepts 0-2 args
	err := validator(nil, []string{})
	assert.NoError(t, err)

	err = validator(nil, []string{"val1"})
	assert.NoError(t, err)

	err = validator(nil, []string{"val1", "val2"})
	assert.NoError(t, err)

	// Check validator rejects 3 args (max is 2)
	err = validator(nil, []string{"val1", "val2", "val3"})
	assert.Error(t, err)
}

func TestPositionalArgsBuilder_Integration(t *testing.T) {
	// Test that builder output can be used with Cobra command
	builder := NewPositionalArgsBuilder()
	builder.AddArg(&PositionalArgSpec{
		Name:        "component",
		Required:    true,
		TargetField: "Component",
	})

	specs, validator, usage := builder.Build()

	// Check specs
	assert.Len(t, specs, 1)
	assert.Equal(t, "Component", specs[0].TargetField)

	// Create a Cobra command with the generated values
	cmd := &cobra.Command{
		Use:  "deploy " + usage,
		Args: validator,
	}

	assert.Equal(t, "deploy <component>", cmd.Use)

	// Test that validator works in Cobra context
	err := cmd.Args(cmd, []string{"vpc"})
	assert.NoError(t, err)

	err = cmd.Args(cmd, []string{})
	assert.Error(t, err)
}
