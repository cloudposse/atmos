package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHintHandler_Execute(t *testing.T) {
	initToastTestIO(t)

	handler, ok := Get("hint")
	require.True(t, ok)

	step := &schema.WorkflowStep{
		Name:    "test",
		Type:    "hint",
		Content: "Run `atmos dev shell`.",
	}
	vars := NewVariables()
	ctx := context.Background()

	result, err := handler.Execute(ctx, step, vars)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Run `atmos dev shell`.", result.Value)
}

func TestHintHandler_ExecuteWithTemplate(t *testing.T) {
	initToastTestIO(t)

	handler, ok := Get("hint")
	require.True(t, ok)

	vars := NewVariables()
	vars.Set("command", NewStepResult("atmos dev shell"))

	step := &schema.WorkflowStep{
		Name:    "test",
		Type:    "hint",
		Content: "Run {{ .steps.command.value }}.",
	}

	result, err := handler.Execute(context.Background(), step, vars)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Run atmos dev shell.", result.Value)
}

func TestHintHandler_ValidateRequiresContent(t *testing.T) {
	handler, ok := Get("hint")
	require.True(t, ok)

	err := handler.Validate(&schema.WorkflowStep{Name: "missing-content"})
	require.Error(t, err)

	err = handler.Validate(&schema.WorkflowStep{Name: "with-content", Content: "hello"})
	require.NoError(t, err)
}

func TestHintHandler_ExecutePropagatesTemplateResolutionError(t *testing.T) {
	initToastTestIO(t)

	handler, ok := Get("hint")
	require.True(t, ok)

	step := &schema.WorkflowStep{
		Name:    "bad-hint",
		Type:    "hint",
		Content: "{{ range .steps }}",
	}
	vars := NewVariables()

	result, err := handler.Execute(context.Background(), step, vars)
	require.Error(t, err)
	assert.Nil(t, result)
	stepName, ok := errUtils.GetContext(err, "step")
	require.True(t, ok)
	assert.Equal(t, "bad-hint", stepName)
}
