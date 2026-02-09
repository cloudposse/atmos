package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ChooseHandler registration and basic validation are tested in interactive_handlers_test.go.
// This file tests helper methods.

//nolint:dupl // Similar test patterns for different handler methods.
func TestChooseHandler_ResolveOptions(t *testing.T) {
	handler, ok := Get("choose")
	require.True(t, ok)
	chooseHandler := handler.(*ChooseHandler)

	t.Run("static options", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Options: []string{"option1", "option2", "option3"},
		}
		vars := NewVariables()

		options, err := chooseHandler.resolveOptions(step, vars)
		require.NoError(t, err)
		assert.Equal(t, []string{"option1", "option2", "option3"}, options)
	})

	t.Run("template options", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Options: []string{"{{ .steps.env.value }}", "staging", "development"},
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))

		options, err := chooseHandler.resolveOptions(step, vars)
		require.NoError(t, err)
		assert.Equal(t, []string{"production", "staging", "development"}, options)
	})

	t.Run("empty options", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Options: []string{},
		}
		vars := NewVariables()

		options, err := chooseHandler.resolveOptions(step, vars)
		require.NoError(t, err)
		assert.Empty(t, options)
	})

	t.Run("invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Options: []string{"valid", "{{ .steps.invalid.value"},
		}
		vars := NewVariables()

		_, err := chooseHandler.resolveOptions(step, vars)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve option 1")
	})
}

func TestChooseHandler_ResolveDefault(t *testing.T) {
	handler, ok := Get("choose")
	require.True(t, ok)
	chooseHandler := handler.(*ChooseHandler)

	t.Run("no default", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Default: "",
		}
		vars := NewVariables()

		defaultVal, err := chooseHandler.resolveDefault(step, vars)
		require.NoError(t, err)
		assert.Empty(t, defaultVal)
	})

	t.Run("static default", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Default: "production",
		}
		vars := NewVariables()

		defaultVal, err := chooseHandler.resolveDefault(step, vars)
		require.NoError(t, err)
		assert.Equal(t, "production", defaultVal)
	})

	t.Run("template default", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Default: "{{ .steps.env.value }}",
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("staging"))

		defaultVal, err := chooseHandler.resolveDefault(step, vars)
		require.NoError(t, err)
		assert.Equal(t, "staging", defaultVal)
	})

	t.Run("invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Default: "{{ .steps.invalid.value",
		}
		vars := NewVariables()

		_, err := chooseHandler.resolveDefault(step, vars)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve default")
	})
}

func TestChooseHandler_CreateChooseKeyMap(t *testing.T) {
	handler, ok := Get("choose")
	require.True(t, ok)
	chooseHandler := handler.(*ChooseHandler)

	t.Run("creates keymap", func(t *testing.T) {
		keyMap := chooseHandler.createChooseKeyMap()
		assert.NotNil(t, keyMap)
		assert.NotNil(t, keyMap.Quit)
	})
}
