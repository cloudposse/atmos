package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAddAffectedSpaceliftAdminStack(t *testing.T) {
	// Prepare test data
	atmosConfig := &schema.AtmosConfiguration{}
	stackName := "test-stack"
	componentName := "test-component"
	affectedList := []schema.Affected{
		{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "spacelift",
			Affected:      "foo",
		},
	}
	spaceliftAdminStack := "spacelift-admin-stack"
	spaceliftAdminStacks := map[string]any{
		spaceliftAdminStack: map[string]any{
			"spacelift": map[string]any{
				"admin": true,
			},
		},
	}
	settingsSection := map[string]any{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	// Call the function under test
	affectedListResult, err := addAffectedSpaceliftAdminStack(
		atmosConfig,
		&affectedList,
		&settingsSection,
		&spaceliftAdminStacks,
		stackName,
		componentName,
		configAndStacksInfo,
		false,
	)

	assert.NoError(t, err)

	// Check that the spacelift admin stack was added to the affected list
	found := false
	for _, affected := range *affectedListResult {
		if affected.Component == componentName && affected.ComponentType == "spacelift" {
			found = true
			break
		}
	}
	assert.True(t, found, "Spacelift admin stack should be added to affected list")
}

func TestAddAffectedSpaceliftAdminStack_NoAdminStack(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackName := "test-stack"
	componentName := "test-component"
	affectedList := []schema.Affected{}
	spaceliftAdminStacks := map[string]any{}
	settingsSection := map[string]any{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	// Should not panic or add anything
	_, err := addAffectedSpaceliftAdminStack(
		atmosConfig,
		&affectedList,
		&settingsSection,
		&spaceliftAdminStacks,
		stackName,
		componentName,
		configAndStacksInfo,
		false,
	)

	assert.NoError(t, err)
	assert.Equal(t, 0, len(affectedList), "Affected list should remain empty if no admin stack")
}

func TestAddAffectedSpaceliftAdminStack_DuplicateNotAdded(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackName := "test-stack"
	componentName := "test-component"
	spaceliftAdminStack := "spacelift-admin-stack"
	affectedList := []schema.Affected{
		{
			Component:     spaceliftAdminStack,
			Stack:         stackName,
			ComponentType: "spacelift",
			Affected:      "foo",
		},
	}
	spaceliftAdminStacks := map[string]any{
		spaceliftAdminStack: map[string]any{
			"spacelift": map[string]any{
				"admin": true,
			},
		},
	}
	settingsSection := map[string]any{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	// Call the function under test
	_, err := addAffectedSpaceliftAdminStack(
		atmosConfig,
		&affectedList,
		&settingsSection,
		&spaceliftAdminStacks,
		stackName,
		componentName,
		configAndStacksInfo,
		false,
	)

	assert.NoError(t, err)

	// Should not add a duplicate
	count := 0
	for _, affected := range affectedList {
		if affected.Component == spaceliftAdminStack && affected.ComponentType == "spacelift" {
			count++
		}
	}
	assert.Equal(t, 1, count, "Spacelift admin stack should not be added twice")
}

func TestAppendToAffected(t *testing.T) {
	t.Run("should add new affected component", func(t *testing.T) {
		// Setup
		atmosConfig := &schema.AtmosConfiguration{}
		componentName := "test-component"
		stackName := "test-stack"
		affectedList := []schema.Affected{}
		affected := &schema.Affected{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "terraform",
			Affected:      "test-change",
		}

		componentSection := map[string]any{
			"settings": map[string]any{},
		}

		// Execute
		err := appendToAffected(
			atmosConfig,
			componentName,
			stackName,
			&componentSection,
			&affectedList,
			affected,
			false,
			&map[string]any{},
			true,
		)

		// Verify
		require.NoError(t, err)
		assert.Len(t, affectedList, 1)
		assert.Equal(t, componentName, affectedList[0].Component)
		assert.Equal(t, stackName, affectedList[0].Stack)
		assert.Equal(t, "test-change", affectedList[0].Affected)
		assert.Len(t, affectedList[0].AffectedAll, 1)
	})

	t.Run("should update existing component with new affected reason", func(t *testing.T) {
		// Setup
		atmosConfig := &schema.AtmosConfiguration{}
		componentName := "test-component"
		stackName := "test-stack"
		affectedList := []schema.Affected{
			{
				Component:     componentName,
				Stack:         stackName,
				ComponentType: "terraform",
				Affected:      "initial-change",
				AffectedAll:   []string{"initial-change"},
			},
		}

		affected := &schema.Affected{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "terraform",
			Affected:      "another-change",
		}

		componentSection := map[string]any{
			"settings": map[string]any{},
		}

		// Execute
		err := appendToAffected(
			atmosConfig,
			componentName,
			stackName,
			&componentSection,
			&affectedList,
			affected,
			false,
			&map[string]any{},
			true,
		)

		// Verify
		require.NoError(t, err)
		assert.Len(t, affectedList, 1)
		assert.Equal(t, componentName, affectedList[0].Component)
		assert.Len(t, affectedList[0].AffectedAll, 2)
		assert.Contains(t, affectedList[0].AffectedAll, "initial-change")
		assert.Contains(t, affectedList[0].AffectedAll, "another-change")
	})

	t.Run("should include settings when requested", func(t *testing.T) {
		// Setup
		atmosConfig := &schema.AtmosConfiguration{}
		componentName := "test-component"
		stackName := "test-stack"
		affectedList := []schema.Affected{}
		affected := &schema.Affected{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "terraform",
			Affected:      "test-change",
		}

		settings := map[string]any{
			"setting1": "value1",
			"setting2": 42,
		}

		componentSection := map[string]any{
			"settings": settings,
		}

		// Execute with includeSettings = true
		err := appendToAffected(
			atmosConfig,
			componentName,
			stackName,
			&componentSection,
			&affectedList,
			affected,
			false,
			&map[string]any{},
			true,
		)

		// Verify
		require.NoError(t, err)
		assert.Len(t, affectedList, 1)
		assert.NotNil(t, affectedList[0].Settings)
		assert.Equal(t, "value1", affectedList[0].Settings["setting1"])
	})

	t.Run("should not include settings when not requested", func(t *testing.T) {
		// Setup
		atmosConfig := &schema.AtmosConfiguration{}
		componentName := "test-component"
		stackName := "test-stack"
		affectedList := []schema.Affected{}
		affected := &schema.Affected{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "terraform",
			Affected:      "test-change",
		}

		componentSection := map[string]any{
			"settings": map[string]any{
				"setting1": "value1",
			},
		}

		// Execute with includeSettings = false
		err := appendToAffected(
			atmosConfig,
			componentName,
			stackName,
			&componentSection,
			&affectedList,
			affected,
			false,
			&map[string]any{},
			false,
		)

		// Verify
		require.NoError(t, err)
		assert.Len(t, affectedList, 1)
		assert.Nil(t, affectedList[0].Settings)
	})
}
