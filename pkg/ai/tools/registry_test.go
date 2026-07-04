package tools

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// mockTool implements the Tool interface for testing.
type mockTool struct {
	name               string
	description        string
	params             []Parameter
	requiresPermission bool
	isRestricted       bool
	executeFunc        func(ctx context.Context, params map[string]interface{}) (*Result, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Parameters() []Parameter {
	return m.params
}

func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}) (*Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, params)
	}
	return &Result{Success: true, Output: "mock output"}, nil
}

func (m *mockTool) RequiresPermission() bool {
	return m.requiresPermission
}

func (m *mockTool) IsRestricted() bool {
	return m.isRestricted
}

// Helper to create a test tool.
func createTestTool(name, description string) *mockTool {
	return &mockTool{
		name:        name,
		description: description,
		params: []Parameter{
			{Name: "param1", Type: ParamTypeString, Description: "Test param", Required: true},
		},
	}
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.tools)
	assert.Equal(t, 0, registry.Count())
}

func TestRegistry_Register_Success(t *testing.T) {
	registry := NewRegistry()
	tool := createTestTool("test-tool", "A test tool")

	err := registry.Register(tool)

	assert.NoError(t, err)
	assert.Equal(t, 1, registry.Count())
}

func TestRegistry_Register_EmptyName(t *testing.T) {
	registry := NewRegistry()
	tool := &mockTool{name: "", description: "Empty name tool"}

	err := registry.Register(tool)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIToolNameEmpty))
	assert.Equal(t, 0, registry.Count())
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	registry := NewRegistry()
	tool1 := createTestTool("duplicate-tool", "First tool")
	tool2 := createTestTool("duplicate-tool", "Second tool")

	err := registry.Register(tool1)
	require.NoError(t, err)

	err = registry.Register(tool2)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIToolAlreadyRegistered))
	assert.Contains(t, err.Error(), "duplicate-tool")
	assert.Equal(t, 1, registry.Count())
}

func TestRegistry_Register_MultipleTools(t *testing.T) {
	registry := NewRegistry()

	tools := []*mockTool{
		createTestTool("tool-a", "Tool A"),
		createTestTool("tool-b", "Tool B"),
		createTestTool("tool-c", "Tool C"),
	}

	for _, tool := range tools {
		err := registry.Register(tool)
		require.NoError(t, err)
	}

	assert.Equal(t, 3, registry.Count())
}

func TestRegistry_Get_ExistingTool(t *testing.T) {
	registry := NewRegistry()
	expectedTool := createTestTool("my-tool", "My tool description")
	err := registry.Register(expectedTool)
	require.NoError(t, err)

	tool, err := registry.Get("my-tool")

	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "my-tool", tool.Name())
	assert.Equal(t, "My tool description", tool.Description())
}

func TestRegistry_Get_NonExistentTool(t *testing.T) {
	registry := NewRegistry()

	tool, err := registry.Get("nonexistent")

	assert.Nil(t, tool)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIToolNotFound))
}

func TestRegistry_Get_CaseSensitive(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(createTestTool("MyTool", "Mixed case tool"))
	require.NoError(t, err)

	// Exact match works.
	tool, err := registry.Get("MyTool")
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Different case fails.
	tool, err = registry.Get("mytool")
	assert.Nil(t, tool)
	assert.Error(t, err)

	tool, err = registry.Get("MYTOOL")
	assert.Nil(t, tool)
	assert.Error(t, err)
}

func TestRegistry_List_Empty(t *testing.T) {
	registry := NewRegistry()

	tools := registry.List()

	assert.NotNil(t, tools)
	assert.Empty(t, tools)
}

func TestRegistry_List_MultipleTools(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(createTestTool("tool-1", "Tool 1"))
	_ = registry.Register(createTestTool("tool-2", "Tool 2"))
	_ = registry.Register(createTestTool("tool-3", "Tool 3"))

	tools := registry.List()

	assert.Len(t, tools, 3)

	// Verify all tools are present.
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	assert.True(t, names["tool-1"])
	assert.True(t, names["tool-2"])
	assert.True(t, names["tool-3"])
}

func TestRegistry_List_ReturnsCopy(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(createTestTool("tool", "A tool"))

	list1 := registry.List()
	list2 := registry.List()

	// Modifying one list shouldn't affect the other.
	list1[0] = nil
	assert.NotNil(t, list2[0])
}

func TestRegistry_ListByCategory(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(createTestTool("atmos_describe", "Describe tool"))
	_ = registry.Register(createTestTool("file_read", "Read tool"))

	// Current implementation returns all tools.
	// TODO: When Category() method is added to Tool interface, update this test.
	tools := registry.ListByCategory(CategoryAtmos)

	assert.Len(t, tools, 2)
}

func TestRegistry_Unregister_ExistingTool(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(createTestTool("removable", "A removable tool"))
	require.Equal(t, 1, registry.Count())

	err := registry.Unregister("removable")

	assert.NoError(t, err)
	assert.Equal(t, 0, registry.Count())

	// Verify tool is no longer accessible.
	tool, err := registry.Get("removable")
	assert.Nil(t, tool)
	assert.Error(t, err)
}

func TestRegistry_Unregister_NonExistentTool(t *testing.T) {
	registry := NewRegistry()

	err := registry.Unregister("nonexistent")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIToolNotFound))
}

func TestRegistry_Unregister_ThenRegisterSameName(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(createTestTool("reusable", "First version"))

	err := registry.Unregister("reusable")
	require.NoError(t, err)

	err = registry.Register(createTestTool("reusable", "Second version"))
	assert.NoError(t, err)

	tool, err := registry.Get("reusable")
	require.NoError(t, err)
	assert.Equal(t, "Second version", tool.Description())
}

func TestRegistry_Count_Empty(t *testing.T) {
	registry := NewRegistry()

	assert.Equal(t, 0, registry.Count())
}

func TestRegistry_Count_AfterOperations(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Register(createTestTool("tool-1", "Tool 1"))
	assert.Equal(t, 1, registry.Count())

	_ = registry.Register(createTestTool("tool-2", "Tool 2"))
	assert.Equal(t, 2, registry.Count())

	_ = registry.Unregister("tool-1")
	assert.Equal(t, 1, registry.Count())

	_ = registry.Unregister("tool-2")
	assert.Equal(t, 0, registry.Count())
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewRegistry()

	const numGoroutines = 100
	var wg sync.WaitGroup

	// Concurrent registrations.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tool := createTestTool("tool-"+string(rune('a'+id%26)), "Tool")
			_ = registry.Register(tool) // Ignore duplicates
		}(i)
	}

	// Concurrent reads.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = registry.List()
			_ = registry.Count()
			_, _ = registry.Get("tool-a")
		}()
	}

	wg.Wait()

	// Verify no panics occurred and registry is in a valid state.
	count := registry.Count()
	assert.True(t, count > 0 && count <= 26)
}

func TestRegistry_ConcurrentRegisterUnregister(t *testing.T) {
	registry := NewRegistry()

	const numIterations = 50
	var wg sync.WaitGroup

	// Pre-register some tools.
	for i := 0; i < 10; i++ {
		_ = registry.Register(createTestTool("persistent-"+string(rune('0'+i)), "Persistent tool"))
	}

	// Concurrent register and unregister.
	for i := 0; i < numIterations; i++ {
		wg.Add(2)

		// Register goroutine.
		go func(id int) {
			defer wg.Done()
			tool := createTestTool("temp-"+string(rune('a'+id%26)), "Temp tool")
			_ = registry.Register(tool)
		}(i)

		// Unregister goroutine.
		go func(id int) {
			defer wg.Done()
			_ = registry.Unregister("temp-" + string(rune('a'+id%26)))
		}(i)
	}

	wg.Wait()

	// Verify persistent tools are still there.
	count := registry.Count()
	assert.True(t, count >= 10, "Expected at least 10 persistent tools, got %d", count)
}

func TestMockTool_Execute(t *testing.T) {
	tool := &mockTool{
		name:        "exec-test",
		description: "Execution test tool",
		executeFunc: func(_ context.Context, params map[string]interface{}) (*Result, error) {
			value, ok := params["input"].(string)
			if !ok {
				return &Result{Success: false, Error: errors.New("missing input")}, nil
			}
			return &Result{
				Success: true,
				Output:  "Processed: " + value,
				Data:    map[string]interface{}{"processed": true},
			}, nil
		},
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{"input": "test"})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "Processed: test", result.Output)
	assert.True(t, result.Data["processed"].(bool))
}

func TestMockTool_ExecuteError(t *testing.T) {
	tool := &mockTool{
		name:        "error-test",
		description: "Error test tool",
		executeFunc: func(_ context.Context, _ map[string]interface{}) (*Result, error) {
			return nil, errors.New("execution failed")
		},
	}

	result, err := tool.Execute(context.Background(), nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "execution failed", err.Error())
}

func TestTool_Interface(t *testing.T) {
	// Verify that mockTool implements the Tool interface.
	var _ Tool = (*mockTool)(nil)
}

func TestRegistry_ToolWithParameters(t *testing.T) {
	registry := NewRegistry()
	tool := &mockTool{
		name:        "parameterized-tool",
		description: "Tool with complex parameters",
		params: []Parameter{
			{Name: "component", Type: ParamTypeString, Description: "Component name", Required: true},
			{Name: "stack", Type: ParamTypeString, Description: "Stack name", Required: true},
			{Name: "verbose", Type: ParamTypeBool, Description: "Verbose output", Required: false, Default: false},
			{Name: "limit", Type: ParamTypeInt, Description: "Result limit", Required: false, Default: 10},
		},
	}

	err := registry.Register(tool)
	require.NoError(t, err)

	retrieved, err := registry.Get("parameterized-tool")
	require.NoError(t, err)

	params := retrieved.Parameters()
	assert.Len(t, params, 4)
	assert.Equal(t, "component", params[0].Name)
	assert.Equal(t, ParamTypeString, params[0].Type)
	assert.True(t, params[0].Required)
}
