package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	registry := NewRegistry()

	// Create a test function.
	fn := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "test-env",
			FunctionAliases: []string{"test-e"},
			FunctionPhase:   PreMerge,
		},
	}

	// Register the function.
	err := registry.Register(fn)
	require.NoError(t, err)

	// Get by primary name.
	got, err := registry.Get("test-env")
	require.NoError(t, err)
	assert.Equal(t, "test-env", got.Name())

	// Get by alias.
	got, err = registry.Get("test-e")
	require.NoError(t, err)
	assert.Equal(t, "test-env", got.Name())

	// Get non-existent function.
	_, err = registry.Get("non-existent")
	assert.ErrorIs(t, err, ErrFunctionNotFound)
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
	registry := NewRegistry()

	fn1 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "dup-test",
			FunctionPhase: PreMerge,
		},
	}

	fn2 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "dup-test",
			FunctionPhase: PreMerge,
		},
	}

	// First registration should succeed.
	err := registry.Register(fn1)
	require.NoError(t, err)

	// Duplicate registration should fail.
	err = registry.Register(fn2)
	assert.ErrorIs(t, err, ErrFunctionAlreadyRegistered)
}

func TestRegistry_GetByPhase(t *testing.T) {
	registry := NewRegistry()

	// Register PreMerge function.
	preMergeFn := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "pre-merge-fn",
			FunctionPhase: PreMerge,
		},
	}
	require.NoError(t, registry.Register(preMergeFn))

	// Register PostMerge function.
	postMergeFn := &StoreFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "post-merge-fn",
			FunctionPhase: PostMerge,
		},
	}
	require.NoError(t, registry.Register(postMergeFn))

	// Get PreMerge functions.
	preMergeFns := registry.GetByPhase(PreMerge)
	assert.Len(t, preMergeFns, 1)
	assert.Equal(t, "pre-merge-fn", preMergeFns[0].Name())

	// Get PostMerge functions.
	postMergeFns := registry.GetByPhase(PostMerge)
	assert.Len(t, postMergeFns, 1)
	assert.Equal(t, "post-merge-fn", postMergeFns[0].Name())
}

func TestRegistry_Has(t *testing.T) {
	registry := NewRegistry()

	fn := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "has-test",
			FunctionAliases: []string{"has-alias"},
			FunctionPhase:   PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn))

	assert.True(t, registry.Has("has-test"))
	assert.True(t, registry.Has("has-alias"))
	assert.False(t, registry.Has("non-existent"))
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry()

	fn := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "unreg-test",
			FunctionAliases: []string{"unreg-alias"},
			FunctionPhase:   PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn))

	// Verify it exists.
	assert.True(t, registry.Has("unreg-test"))
	assert.True(t, registry.Has("unreg-alias"))

	// Unregister.
	registry.Unregister("unreg-test")

	// Verify it's gone.
	assert.False(t, registry.Has("unreg-test"))
	assert.False(t, registry.Has("unreg-alias"))
}

func TestDefaultRegistry_HasAllDefaults(t *testing.T) {
	// Force re-initialization.
	registry := DefaultRegistry()

	// Check that default functions are registered.
	expectedFunctions := []string{
		TagEnv,
		TagExec,
		TagRandom,
		TagTemplate,
		TagRepoRoot,
		TagInclude,
		TagIncludeRaw,
		TagStore,
		TagStoreGet,
		TagTerraformOutput,
		TagTerraformState,
		TagAwsAccountID,
		TagAwsCallerIdentityArn,
		TagAwsCallerIdentityUserID,
		TagAwsRegion,
	}

	for _, name := range expectedFunctions {
		assert.True(t, registry.Has(name), "expected function %s to be registered", name)
	}
}

func TestEnvFunction_Execute(t *testing.T) {
	fn := NewEnvFunction()

	// Set up test environment variable.
	t.Setenv("TEST_VAR", "test_value")

	// Test basic env lookup.
	result, err := fn.Execute(context.Background(), "TEST_VAR", nil)
	require.NoError(t, err)
	assert.Equal(t, "test_value", result)

	// Test with default value for missing variable.
	result, err = fn.Execute(context.Background(), "MISSING_VAR default_value", nil)
	require.NoError(t, err)
	assert.Equal(t, "default_value", result)

	// Test missing variable without default.
	result, err = fn.Execute(context.Background(), "MISSING_VAR", nil)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestRandomFunction_Execute(t *testing.T) {
	fn := NewRandomFunction()

	// Test with no arguments (default range).
	result, err := fn.Execute(context.Background(), "", nil)
	require.NoError(t, err)
	val, ok := result.(int)
	require.True(t, ok)
	assert.GreaterOrEqual(t, val, 0)
	assert.LessOrEqual(t, val, 65535)

	// Test with max only.
	result, err = fn.Execute(context.Background(), "100", nil)
	require.NoError(t, err)
	val, ok = result.(int)
	require.True(t, ok)
	assert.GreaterOrEqual(t, val, 0)
	assert.LessOrEqual(t, val, 100)

	// Test with min and max.
	result, err = fn.Execute(context.Background(), "10 20", nil)
	require.NoError(t, err)
	val, ok = result.(int)
	require.True(t, ok)
	assert.GreaterOrEqual(t, val, 10)
	assert.LessOrEqual(t, val, 20)
}

func TestTemplateFunction_Execute(t *testing.T) {
	fn := NewTemplateFunction()

	// Test JSON object.
	result, err := fn.Execute(context.Background(), `{"key": "value"}`, nil)
	require.NoError(t, err)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", m["key"])

	// Test JSON array.
	result, err = fn.Execute(context.Background(), `[1, 2, 3]`, nil)
	require.NoError(t, err)
	arr, ok := result.([]any)
	require.True(t, ok)
	assert.Len(t, arr, 3)

	// Test non-JSON string.
	result, err = fn.Execute(context.Background(), "not json", nil)
	require.NoError(t, err)
	assert.Equal(t, "not json", result)
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	// Empty registry.
	names := registry.List()
	assert.Empty(t, names)

	// Register some functions.
	fn1 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "func1",
			FunctionPhase: PreMerge,
		},
	}
	fn2 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "func2",
			FunctionPhase: PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn1))
	require.NoError(t, registry.Register(fn2))

	names = registry.List()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "func1")
	assert.Contains(t, names, "func2")
}

func TestRegistry_Len(t *testing.T) {
	registry := NewRegistry()

	// Empty registry.
	assert.Equal(t, 0, registry.Len())

	// Register a function.
	fn := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "len-test",
			FunctionPhase: PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn))
	assert.Equal(t, 1, registry.Len())
}

func TestRegistry_Clear(t *testing.T) {
	registry := NewRegistry()

	// Register some functions.
	fn1 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "clear-test1",
			FunctionAliases: []string{"alias1"},
			FunctionPhase:   PreMerge,
		},
	}
	fn2 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "clear-test2",
			FunctionPhase: PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn1))
	require.NoError(t, registry.Register(fn2))
	assert.Equal(t, 2, registry.Len())

	// Clear the registry.
	registry.Clear()
	assert.Equal(t, 0, registry.Len())
	assert.False(t, registry.Has("clear-test1"))
	assert.False(t, registry.Has("alias1"))
	assert.False(t, registry.Has("clear-test2"))
}

func TestRegistry_AliasConflicts(t *testing.T) {
	registry := NewRegistry()

	// Register a function with alias.
	fn1 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "alias-conflict-1",
			FunctionAliases: []string{"shared-alias"},
			FunctionPhase:   PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn1))

	// Try to register another function with same alias.
	fn2 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "alias-conflict-2",
			FunctionAliases: []string{"shared-alias"},
			FunctionPhase:   PreMerge,
		},
	}
	err := registry.Register(fn2)
	assert.ErrorIs(t, err, ErrFunctionAlreadyRegistered)
}

func TestRegistry_NameConflictsWithAlias(t *testing.T) {
	registry := NewRegistry()

	// Register a function with alias.
	fn1 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "name-alias-conflict",
			FunctionAliases: []string{"will-conflict"},
			FunctionPhase:   PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn1))

	// Try to register function where name matches existing alias.
	fn2 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "will-conflict",
			FunctionPhase: PreMerge,
		},
	}
	err := registry.Register(fn2)
	assert.ErrorIs(t, err, ErrFunctionAlreadyRegistered)
}

func TestRegistry_AliasConflictsWithName(t *testing.T) {
	registry := NewRegistry()

	// Register a function.
	fn1 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:  "existing-name",
			FunctionPhase: PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn1))

	// Try to register function where alias matches existing name.
	fn2 := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "new-func",
			FunctionAliases: []string{"existing-name"},
			FunctionPhase:   PreMerge,
		},
	}
	err := registry.Register(fn2)
	assert.ErrorIs(t, err, ErrFunctionAlreadyRegistered)
}

func TestRegistry_Unregister_NonExistent(t *testing.T) {
	registry := NewRegistry()

	// Unregister non-existent function should not panic.
	registry.Unregister("non-existent")
	assert.Equal(t, 0, registry.Len())
}

func TestRegistry_CaseInsensitive(t *testing.T) {
	registry := NewRegistry()

	fn := &EnvFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "CaseSensitive",
			FunctionAliases: []string{"ALIAS"},
			FunctionPhase:   PreMerge,
		},
	}
	require.NoError(t, registry.Register(fn))

	// Test case-insensitive lookup.
	assert.True(t, registry.Has("casesensitive"))
	assert.True(t, registry.Has("CASESENSITIVE"))
	assert.True(t, registry.Has("alias"))
	assert.True(t, registry.Has("ALIAS"))

	// Get returns the function with original name case.
	got, err := registry.Get("CASESENSITIVE")
	require.NoError(t, err)
	assert.Equal(t, "CaseSensitive", got.Name())
}

func TestRegistry_GetByPhase_Empty(t *testing.T) {
	registry := NewRegistry()

	// Empty registry should return empty slice.
	preMerge := registry.GetByPhase(PreMerge)
	assert.Empty(t, preMerge)

	postMerge := registry.GetByPhase(PostMerge)
	assert.Empty(t, postMerge)
}
