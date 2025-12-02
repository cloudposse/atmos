package function

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFunction is a test implementation of Function.
type mockFunction struct {
	BaseFunction
	executeFunc func(ctx context.Context, args string, execCtx *ExecutionContext) (any, error)
}

func (f *mockFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	if f.executeFunc != nil {
		return f.executeFunc(ctx, args, execCtx)
	}
	return args, nil
}

func newMockFunction(name string, aliases []string, phase Phase) *mockFunction {
	return &mockFunction{
		BaseFunction: BaseFunction{
			FunctionName:    name,
			FunctionAliases: aliases,
			FunctionPhase:   phase,
		},
	}
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	assert.NotNil(t, r)
	assert.Equal(t, 0, r.Len())
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()

	fn := newMockFunction("env", nil, PreMerge)
	err := r.Register(fn)

	require.NoError(t, err)
	assert.Equal(t, 1, r.Len())
	assert.True(t, r.Has("env"))
}

func TestRegistryRegisterWithAliases(t *testing.T) {
	r := NewRegistry()

	fn := newMockFunction("store.get", []string{"store"}, PostMerge)
	err := r.Register(fn)

	require.NoError(t, err)
	assert.True(t, r.Has("store.get"))
	assert.True(t, r.Has("store"))
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	r := NewRegistry()

	fn1 := newMockFunction("env", nil, PreMerge)
	fn2 := newMockFunction("env", nil, PreMerge)

	err := r.Register(fn1)
	require.NoError(t, err)

	err = r.Register(fn2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDuplicateFunction))
}

func TestRegistryRegisterAliasConflict(t *testing.T) {
	r := NewRegistry()

	fn1 := newMockFunction("store.get", []string{"store"}, PostMerge)
	fn2 := newMockFunction("other", []string{"store"}, PostMerge)

	err := r.Register(fn1)
	require.NoError(t, err)

	err = r.Register(fn2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDuplicateFunction))
}

func TestRegistryRegisterNameConflictsWithAlias(t *testing.T) {
	r := NewRegistry()

	fn1 := newMockFunction("primary", []string{"alias"}, PreMerge)
	fn2 := newMockFunction("alias", nil, PreMerge)

	err := r.Register(fn1)
	require.NoError(t, err)

	err = r.Register(fn2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDuplicateFunction))
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()

	fn := newMockFunction("env", nil, PreMerge)
	require.NoError(t, r.Register(fn))

	got, err := r.Get("env")
	require.NoError(t, err)
	assert.Equal(t, fn, got)
}

func TestRegistryGetByAlias(t *testing.T) {
	r := NewRegistry()

	fn := newMockFunction("store.get", []string{"store"}, PostMerge)
	require.NoError(t, r.Register(fn))

	got, err := r.Get("store")
	require.NoError(t, err)
	assert.Equal(t, fn, got)
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrFunctionNotFound))
}

func TestRegistryHas(t *testing.T) {
	r := NewRegistry()

	fn := newMockFunction("env", []string{"environment"}, PreMerge)
	require.NoError(t, r.Register(fn))

	assert.True(t, r.Has("env"))
	assert.True(t, r.Has("environment"))
	assert.False(t, r.Has("nonexistent"))
}

func TestRegistryGetByPhase(t *testing.T) {
	r := NewRegistry()

	fn1 := newMockFunction("env", nil, PreMerge)
	fn2 := newMockFunction("exec", nil, PreMerge)
	fn3 := newMockFunction("terraform.output", nil, PostMerge)
	fn4 := newMockFunction("store.get", nil, PostMerge)

	require.NoError(t, r.Register(fn1))
	require.NoError(t, r.Register(fn2))
	require.NoError(t, r.Register(fn3))
	require.NoError(t, r.Register(fn4))

	preMerge := r.GetByPhase(PreMerge)
	assert.Len(t, preMerge, 2)

	postMerge := r.GetByPhase(PostMerge)
	assert.Len(t, postMerge, 2)
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()

	fn1 := newMockFunction("env", nil, PreMerge)
	fn2 := newMockFunction("exec", nil, PreMerge)
	fn3 := newMockFunction("terraform.output", nil, PostMerge)

	require.NoError(t, r.Register(fn1))
	require.NoError(t, r.Register(fn2))
	require.NoError(t, r.Register(fn3))

	names := r.List()
	sort.Strings(names)

	assert.Equal(t, []string{"env", "exec", "terraform.output"}, names)
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()

	fn := newMockFunction("env", []string{"environment"}, PreMerge)
	require.NoError(t, r.Register(fn))

	err := r.Unregister("env")
	require.NoError(t, err)

	assert.False(t, r.Has("env"))
	assert.False(t, r.Has("environment"))
	assert.Equal(t, 0, r.Len())
}

func TestRegistryUnregisterNotFound(t *testing.T) {
	r := NewRegistry()

	err := r.Unregister("nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrFunctionNotFound))
}

func TestRegistryClear(t *testing.T) {
	r := NewRegistry()

	fn1 := newMockFunction("env", nil, PreMerge)
	fn2 := newMockFunction("exec", nil, PreMerge)
	require.NoError(t, r.Register(fn1))
	require.NoError(t, r.Register(fn2))

	assert.Equal(t, 2, r.Len())

	r.Clear()

	assert.Equal(t, 0, r.Len())
	assert.False(t, r.Has("env"))
	assert.False(t, r.Has("exec"))
}

func TestRegistryConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	done := make(chan bool)

	// Concurrent registration.
	go func() {
		for i := 0; i < 100; i++ {
			fn := newMockFunction("env", nil, PreMerge)
			_ = r.Register(fn)
			_ = r.Unregister("env")
		}
		done <- true
	}()

	// Concurrent reads.
	go func() {
		for i := 0; i < 100; i++ {
			_ = r.Has("env")
			_, _ = r.Get("env")
			_ = r.List()
		}
		done <- true
	}()

	<-done
	<-done
}

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase    Phase
		expected string
	}{
		{PreMerge, "pre-merge"},
		{PostMerge, "post-merge"},
		{Phase(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.phase.String())
		})
	}
}

func TestExecutionContext(t *testing.T) {
	env := map[string]string{
		"HOME": "/home/user",
		"USER": "testuser",
	}

	ctx := NewExecutionContext(env, "/work/dir", "config.yaml")

	assert.Equal(t, "/home/user", ctx.GetEnv("HOME"))
	assert.Equal(t, "testuser", ctx.GetEnv("USER"))
	assert.Equal(t, "", ctx.GetEnv("NONEXISTENT"))

	assert.True(t, ctx.HasEnv("HOME"))
	assert.False(t, ctx.HasEnv("NONEXISTENT"))

	assert.Equal(t, "/work/dir", ctx.WorkingDir)
	assert.Equal(t, "config.yaml", ctx.SourceFile)
}

func TestExecutionContextNil(t *testing.T) {
	var ctx *ExecutionContext

	assert.Equal(t, "", ctx.GetEnv("HOME"))
	assert.False(t, ctx.HasEnv("HOME"))
}

func TestExecutionContextNilEnv(t *testing.T) {
	ctx := NewExecutionContext(nil, "", "")

	assert.NotNil(t, ctx.Env)
	assert.Equal(t, "", ctx.GetEnv("HOME"))
	assert.False(t, ctx.HasEnv("HOME"))
}

func TestBaseFunction(t *testing.T) {
	bf := &BaseFunction{
		FunctionName:    "test",
		FunctionAliases: []string{"t", "tst"},
		FunctionPhase:   PostMerge,
	}

	assert.Equal(t, "test", bf.Name())
	assert.Equal(t, []string{"t", "tst"}, bf.Aliases())
	assert.Equal(t, PostMerge, bf.Phase())
}

func TestBaseFunctionNilAliases(t *testing.T) {
	bf := &BaseFunction{
		FunctionName: "test",
	}

	aliases := bf.Aliases()
	assert.NotNil(t, aliases)
	assert.Len(t, aliases, 0)
}

func TestMockFunctionExecute(t *testing.T) {
	fn := newMockFunction("test", nil, PreMerge)
	fn.executeFunc = func(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
		return "executed: " + args, nil
	}

	result, err := fn.Execute(context.Background(), "arg1 arg2", nil)
	require.NoError(t, err)
	assert.Equal(t, "executed: arg1 arg2", result)
}
