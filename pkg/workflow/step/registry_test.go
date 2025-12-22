package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Registration tests are in other test files.
// This file tests additional registry functionality.

func TestRegistryGet(t *testing.T) {
	// Test Get for existing handlers.
	existingHandlers := []string{
		"shell", "atmos", "input", "confirm", "choose", "filter",
		"file", "write", "toast", "alert", "title", "clear",
		"linebreak", "markdown", "spin", "table", "pager",
		"format", "join", "style", "log", "env", "exit",
		"stage", "sleep",
	}

	for _, name := range existingHandlers {
		t.Run("get_"+name, func(t *testing.T) {
			handler, ok := Get(name)
			assert.True(t, ok, "handler %s should exist", name)
			assert.NotNil(t, handler)
			assert.Equal(t, name, handler.GetName())
		})
	}

	t.Run("get non-existent handler", func(t *testing.T) {
		handler, ok := Get("non_existent_handler_xyz")
		assert.False(t, ok)
		assert.Nil(t, handler)
	})
}

func TestRegistryList(t *testing.T) {
	handlers := List()

	// Verify the returned map is not nil.
	assert.NotNil(t, handlers)

	// Verify it contains expected handlers.
	assert.Contains(t, handlers, "shell")
	assert.Contains(t, handlers, "atmos")
	assert.Contains(t, handlers, "input")

	// Verify the count matches what Count() returns.
	assert.Equal(t, Count(), len(handlers))
}

func TestRegistryListByCategory(t *testing.T) {
	byCategory := ListByCategory()

	// Verify all categories are present.
	assert.Contains(t, byCategory, CategoryInteractive)
	assert.Contains(t, byCategory, CategoryOutput)
	assert.Contains(t, byCategory, CategoryUI)
	assert.Contains(t, byCategory, CategoryCommand)

	// Verify some expected handlers in each category.
	categoryHas := func(cat StepCategory, name string) bool {
		for _, h := range byCategory[cat] {
			if h.GetName() == name {
				return true
			}
		}
		return false
	}

	assert.True(t, categoryHas(CategoryInteractive, "input"))
	assert.True(t, categoryHas(CategoryInteractive, "confirm"))
	assert.True(t, categoryHas(CategoryOutput, "spin"))
	assert.True(t, categoryHas(CategoryOutput, "table"))
	assert.True(t, categoryHas(CategoryUI, "toast"))
	assert.True(t, categoryHas(CategoryUI, "title"))
	assert.True(t, categoryHas(CategoryCommand, "shell"))
	assert.True(t, categoryHas(CategoryCommand, "atmos"))
}

func TestRegistryCount(t *testing.T) {
	count := Count()

	// Should have at least the core handlers registered.
	assert.GreaterOrEqual(t, count, 20, "should have at least 20 handlers registered")
}

func TestRegistryRegister(t *testing.T) {
	// Count before registration.
	countBefore := Count()

	// Register a duplicate handler (should replace).
	// Get the existing shell handler.
	existingShell, ok := Get("shell")
	require.True(t, ok)

	// Re-register it (should succeed without error).
	Register(existingShell)

	// Count should remain the same (replaced, not added).
	assert.Equal(t, countBefore, Count())
}

func TestStepCategory(t *testing.T) {
	// Verify category constants.
	assert.Equal(t, StepCategory("interactive"), CategoryInteractive)
	assert.Equal(t, StepCategory("output"), CategoryOutput)
	assert.Equal(t, StepCategory("ui"), CategoryUI)
	assert.Equal(t, StepCategory("command"), CategoryCommand)
}

func TestHandlerInterface(t *testing.T) {
	// Verify all handlers implement the interface correctly.
	handlers := List()

	for name, handler := range handlers {
		t.Run("interface_"+name, func(t *testing.T) {
			// GetName should return the handler name.
			assert.Equal(t, name, handler.GetName())

			// GetCategory should return a valid category.
			cat := handler.GetCategory()
			assert.True(t, cat == CategoryInteractive ||
				cat == CategoryOutput ||
				cat == CategoryUI ||
				cat == CategoryCommand,
				"handler %s has invalid category: %s", name, cat)

			// RequiresTTY should be consistent with category.
			// Interactive handlers typically require TTY.
			requiresTTY := handler.RequiresTTY()
			if cat == CategoryInteractive {
				assert.True(t, requiresTTY, "interactive handler %s should require TTY", name)
			}
		})
	}
}
