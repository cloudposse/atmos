package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
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
