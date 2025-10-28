package agents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_List_SortedAlphabetically(t *testing.T) {
	registry := NewRegistry()

	// Register agents in random order.
	agents := []*Agent{
		{Name: "zebra", DisplayName: "Zebra", Description: "Last alphabetically", IsBuiltIn: false},
		{Name: "alpha", DisplayName: "Alpha", Description: "First alphabetically", IsBuiltIn: false},
		{Name: "middle", DisplayName: "Middle", Description: "Middle alphabetically", IsBuiltIn: false},
	}

	for _, agent := range agents {
		err := registry.Register(agent)
		require.NoError(t, err)
	}

	// List should return agents sorted by name.
	list := registry.List()
	require.Len(t, list, 3)

	assert.Equal(t, "alpha", list[0].Name)
	assert.Equal(t, "middle", list[1].Name)
	assert.Equal(t, "zebra", list[2].Name)
}

func TestRegistry_ListBuiltIn_SortedAlphabetically(t *testing.T) {
	registry := NewRegistry()

	// Register mix of built-in and custom agents.
	agents := []*Agent{
		{Name: "zebra", DisplayName: "Zebra", Description: "Built-in", IsBuiltIn: true},
		{Name: "custom", DisplayName: "Custom", Description: "Custom", IsBuiltIn: false},
		{Name: "alpha", DisplayName: "Alpha", Description: "Built-in", IsBuiltIn: true},
		{Name: "middle", DisplayName: "Middle", Description: "Built-in", IsBuiltIn: true},
	}

	for _, agent := range agents {
		err := registry.Register(agent)
		require.NoError(t, err)
	}

	// ListBuiltIn should return only built-in agents sorted by name.
	list := registry.ListBuiltIn()
	require.Len(t, list, 3)

	assert.Equal(t, "alpha", list[0].Name)
	assert.Equal(t, "middle", list[1].Name)
	assert.Equal(t, "zebra", list[2].Name)
}

func TestRegistry_ListCustom_SortedAlphabetically(t *testing.T) {
	registry := NewRegistry()

	// Register mix of built-in and custom agents.
	agents := []*Agent{
		{Name: "zebra", DisplayName: "Zebra", Description: "Custom", IsBuiltIn: false},
		{Name: "builtin", DisplayName: "BuiltIn", Description: "Built-in", IsBuiltIn: true},
		{Name: "alpha", DisplayName: "Alpha", Description: "Custom", IsBuiltIn: false},
		{Name: "middle", DisplayName: "Middle", Description: "Custom", IsBuiltIn: false},
	}

	for _, agent := range agents {
		err := registry.Register(agent)
		require.NoError(t, err)
	}

	// ListCustom should return only custom agents sorted by name.
	list := registry.ListCustom()
	require.Len(t, list, 3)

	assert.Equal(t, "alpha", list[0].Name)
	assert.Equal(t, "middle", list[1].Name)
	assert.Equal(t, "zebra", list[2].Name)
}

func TestRegistry_ListByCategory_SortedAlphabetically(t *testing.T) {
	registry := NewRegistry()

	// Register agents with different categories.
	agents := []*Agent{
		{Name: "zebra", DisplayName: "Zebra", Description: "Analysis", Category: "analysis", IsBuiltIn: false},
		{Name: "other", DisplayName: "Other", Description: "General", Category: "general", IsBuiltIn: false},
		{Name: "alpha", DisplayName: "Alpha", Description: "Analysis", Category: "analysis", IsBuiltIn: false},
		{Name: "middle", DisplayName: "Middle", Description: "Analysis", Category: "analysis", IsBuiltIn: false},
	}

	for _, agent := range agents {
		err := registry.Register(agent)
		require.NoError(t, err)
	}

	// ListByCategory should return only agents in that category sorted by name.
	list := registry.ListByCategory("analysis")
	require.Len(t, list, 3)

	assert.Equal(t, "alpha", list[0].Name)
	assert.Equal(t, "middle", list[1].Name)
	assert.Equal(t, "zebra", list[2].Name)
}

func TestRegistry_BuiltInAgents_ConsistentOrder(t *testing.T) {
	// This test ensures that the built-in agents returned by GetBuiltInAgents()
	// will always appear in the same order when registered and listed.
	registry := NewRegistry()

	// Register all built-in agents.
	for _, agent := range GetBuiltInAgents() {
		err := registry.Register(agent)
		require.NoError(t, err)
	}

	// List multiple times - order should be consistent.
	firstList := registry.List()
	secondList := registry.List()
	thirdList := registry.List()

	require.Len(t, firstList, 5)
	require.Len(t, secondList, 5)
	require.Len(t, thirdList, 5)

	// All three lists should have same order.
	for i := range firstList {
		assert.Equal(t, firstList[i].Name, secondList[i].Name)
		assert.Equal(t, firstList[i].Name, thirdList[i].Name)
	}

	// Verify alphabetical order by name.
	assert.Equal(t, "component-refactor", firstList[0].Name)
	assert.Equal(t, "config-validator", firstList[1].Name)
	assert.Equal(t, "general", firstList[2].Name)
	assert.Equal(t, "security-auditor", firstList[3].Name)
	assert.Equal(t, "stack-analyzer", firstList[4].Name)
}
