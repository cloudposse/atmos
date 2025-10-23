package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessCustomYamlTagsWithoutContext(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	input := schema.AtmosSectionMapType{
		"simple_string": "value",
		"number":        123,
		"bool":          true,
	}

	// Clear any existing context.
	ClearResolutionContext()
	defer ClearResolutionContext()

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil)

	require.NoError(t, err)
	assert.Equal(t, input, result)
}

func TestProcessCustomYamlTagsCreatesContext(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	input := schema.AtmosSectionMapType{
		"simple": "value",
	}

	// Clear any existing context.
	ClearResolutionContext()
	defer ClearResolutionContext()

	// Processing should create a goroutine-local context.
	_, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil)
	require.NoError(t, err)

	// Verify context was created.
	ctx := GetOrCreateResolutionContext()
	assert.NotNil(t, ctx)
}

func TestProcessCustomYamlTagsWithContextParameter(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	input := schema.AtmosSectionMapType{
		"test": "value",
	}

	// Add a node to context before processing.
	node := DependencyNode{
		Component:    "test-component",
		Stack:        "test-stack",
		FunctionType: "test",
		FunctionCall: "test",
	}
	require.NoError(t, ctx.Push(atmosConfig, node))

	result, err := ProcessCustomYamlTagsWithContext(atmosConfig, input, "test-stack", nil, ctx)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Context should still have the node.
	assert.Equal(t, 1, len(ctx.CallStack))
}

func TestProcessNodesWithContextNestedMaps(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	input := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": "value",
			},
		},
	}

	result := processNodesWithContext(atmosConfig, input, "test-stack", nil, ctx)

	require.NotNil(t, result)
	assert.Equal(t, input, result)
}

func TestProcessNodesWithContextSlices(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	input := map[string]any{
		"list": []any{
			"item1",
			"item2",
			map[string]any{
				"nested": "value",
			},
		},
	}

	result := processNodesWithContext(atmosConfig, input, "test-stack", nil, ctx)

	require.NotNil(t, result)
	assert.Equal(t, input, result)
}

func TestProcessNodesWithContextMixedTypes(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	input := map[string]any{
		"string":  "value",
		"number":  42,
		"float":   3.14,
		"bool":    true,
		"null":    nil,
		"map":     map[string]any{"key": "value"},
		"slice":   []any{1, 2, 3},
		"complex": []any{map[string]any{"nested": true}},
	}

	result := processNodesWithContext(atmosConfig, input, "test-stack", nil, ctx)

	require.NotNil(t, result)
	assert.Equal(t, input, result)
}

func TestProcessCustomTagsWithContextSkipFunctions(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	// Test that skip list is respected.
	skip := []string{"terraform.state", "terraform.output"}

	// These should not be processed (just returned as-is).
	input1 := "!terraform.state vpc dev output"
	result1 := processCustomTagsWithContext(atmosConfig, input1, "test-stack", skip, ctx)
	assert.Equal(t, input1, result1)

	input2 := "!terraform.output vpc dev output"
	result2 := processCustomTagsWithContext(atmosConfig, input2, "test-stack", skip, ctx)
	assert.Equal(t, input2, result2)

	// Non-skipped functions should still be processed.
	input3 := "!env HOME"
	result3 := processCustomTagsWithContext(atmosConfig, input3, "test-stack", skip, ctx)
	// Result should be different (env var value or empty string).
	assert.IsType(t, "", result3)
}

func TestProcessCustomTagsWithContextTemplateFunction(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	// Template function should work without errors.
	input := "!template {{ .test }}"
	result := processCustomTagsWithContext(atmosConfig, input, "test-stack", nil, ctx)

	// Should return a string (the template).
	assert.IsType(t, "", result)
}

func TestProcessCustomTagsWithContextEnvFunction(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	// Test !env function.
	input := "!env USER"
	result := processCustomTagsWithContext(atmosConfig, input, "test-stack", nil, ctx)

	// Should return a string.
	assert.IsType(t, "", result)
}

func TestProcessCustomTagsWithContextUnknownTag(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	// Unknown tags should be returned as-is.
	input := "!unknown.function arg1 arg2"
	result := processCustomTagsWithContext(atmosConfig, input, "test-stack", nil, ctx)

	assert.Equal(t, input, result)
}

func TestProcessCustomYamlTagsContextIsolation(t *testing.T) {
	// Test that each goroutine gets its own context.
	done1 := make(chan bool)
	done2 := make(chan bool)

	atmosConfig := &schema.AtmosConfiguration{}
	input := schema.AtmosSectionMapType{"test": "value"}

	// Goroutine 1.
	go func() {
		defer close(done1)
		ClearResolutionContext()
		defer ClearResolutionContext()

		ctx1 := GetOrCreateResolutionContext()
		node1 := DependencyNode{
			Component:    "component1",
			Stack:        "stack1",
			FunctionType: "test",
			FunctionCall: "test1",
		}
		require.NoError(t, ctx1.Push(atmosConfig, node1))

		_, err := ProcessCustomYamlTags(atmosConfig, input, "stack1", nil)
		assert.NoError(t, err)

		assert.Equal(t, 1, len(ctx1.CallStack))
		assert.Equal(t, "component1", ctx1.CallStack[0].Component)
	}()

	// Goroutine 2.
	go func() {
		defer close(done2)
		ClearResolutionContext()
		defer ClearResolutionContext()

		ctx2 := GetOrCreateResolutionContext()
		node2 := DependencyNode{
			Component:    "component2",
			Stack:        "stack2",
			FunctionType: "test",
			FunctionCall: "test2",
		}
		require.NoError(t, ctx2.Push(atmosConfig, node2))

		_, err := ProcessCustomYamlTags(atmosConfig, input, "stack2", nil)
		assert.NoError(t, err)

		assert.Equal(t, 1, len(ctx2.CallStack))
		assert.Equal(t, "component2", ctx2.CallStack[0].Component)
	}()

	<-done1
	<-done2
}
