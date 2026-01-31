package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// FilterHandler registration and basic validation are tested in interactive_handlers_test.go.
// This file tests helper methods.

//nolint:dupl // Similar test patterns for different handler methods.
func TestFilterHandler_ResolveOptions(t *testing.T) {
	handler, ok := Get("filter")
	require.True(t, ok)
	filterHandler := handler.(*FilterHandler)

	t.Run("static options", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Options: []string{"option1", "option2", "option3"},
		}
		vars := NewVariables()

		options, err := filterHandler.resolveOptions(step, vars)
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

		options, err := filterHandler.resolveOptions(step, vars)
		require.NoError(t, err)
		assert.Equal(t, []string{"production", "staging", "development"}, options)
	})

	t.Run("empty options", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Options: []string{},
		}
		vars := NewVariables()

		options, err := filterHandler.resolveOptions(step, vars)
		require.NoError(t, err)
		assert.Empty(t, options)
	})

	t.Run("invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Options: []string{"valid", "{{ .steps.invalid.value"},
		}
		vars := NewVariables()

		_, err := filterHandler.resolveOptions(step, vars)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve option 1")
	})
}

func TestFilterHandler_CreateFilterKeyMap(t *testing.T) {
	handler, ok := Get("filter")
	require.True(t, ok)
	filterHandler := handler.(*FilterHandler)

	t.Run("creates keymap", func(t *testing.T) {
		keyMap := filterHandler.createFilterKeyMap()
		assert.NotNil(t, keyMap)
		assert.NotNil(t, keyMap.Quit)
	})
}
