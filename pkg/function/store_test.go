package function

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// mockStore implements the store.Store interface for testing.
type mockStore struct {
	getFunc    func(stack, component, key string) (any, error)
	getKeyFunc func(key string) (any, error)
	setFunc    func(stack, component, key string, value any) error
}

func (m *mockStore) Get(stack, component, key string) (any, error) {
	if m.getFunc != nil {
		return m.getFunc(stack, component, key)
	}
	return nil, errors.New("not implemented")
}

func (m *mockStore) GetKey(key string) (any, error) {
	if m.getKeyFunc != nil {
		return m.getKeyFunc(key)
	}
	return nil, errors.New("not implemented")
}

func (m *mockStore) Set(stack, component, key string, value any) error {
	if m.setFunc != nil {
		return m.setFunc(stack, component, key, value)
	}
	return nil
}

func TestNewStoreFunction(t *testing.T) {
	fn := NewStoreFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagStore, fn.Name())
	assert.Equal(t, PostMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestNewStoreGetFunction(t *testing.T) {
	fn := NewStoreGetFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagStoreGet, fn.Name())
	assert.Equal(t, PostMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestStoreFunction_Execute(t *testing.T) {
	tests := []struct {
		name         string
		args         string
		currentStack string
		storeValue   any
		storeErr     error
		want         any
		wantErr      bool
		errContains  string
	}{
		{
			name:         "basic 4-param usage",
			args:         "mystore tenant1-ue2-dev vpc outputs",
			currentStack: "default-stack",
			storeValue:   map[string]any{"vpc_id": "vpc-123"},
			want:         map[string]any{"vpc_id": "vpc-123"},
		},
		{
			name:         "basic 3-param usage with current stack",
			args:         "mystore vpc outputs",
			currentStack: "tenant1-ue2-dev",
			storeValue:   "simple-value",
			want:         "simple-value",
		},
		{
			name:         "with default value on error",
			args:         "mystore tenant1-ue2-dev vpc outputs | default \"fallback\"",
			currentStack: "default",
			storeErr:     errors.New("key not found"),
			want:         "fallback",
		},
		{
			name:         "error without default",
			args:         "mystore tenant1-ue2-dev vpc outputs",
			currentStack: "default",
			storeErr:     errors.New("key not found"),
			wantErr:      true,
			errContains:  "failed to get key",
		},
		{
			name:         "store not found",
			args:         "nonexistent tenant1-ue2-dev vpc outputs",
			currentStack: "default",
			wantErr:      true,
			errContains:  "store 'nonexistent' not found",
		},
		{
			name:         "invalid argument count - too few",
			args:         "mystore vpc",
			currentStack: "default",
			wantErr:      true,
			errContains:  "requires 3 or 4 parameters",
		},
		{
			name:         "invalid argument count - too many",
			args:         "mystore a b c d e",
			currentStack: "default",
			wantErr:      true,
			errContains:  "requires 3 or 4 parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{
				getFunc: func(stack, component, key string) (any, error) {
					return tt.storeValue, tt.storeErr
				},
			}

			atmosConfig := &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"mystore": mock,
				},
			}

			execCtx := &ExecutionContext{
				AtmosConfig: atmosConfig,
				Stack:       tt.currentStack,
			}

			fn := NewStoreFunction()
			result, err := fn.Execute(context.Background(), tt.args, execCtx)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestStoreFunction_Execute_NilContext(t *testing.T) {
	fn := NewStoreFunction()

	// Test with nil context.
	_, err := fn.Execute(context.Background(), "mystore stack comp key", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
	assert.Contains(t, err.Error(), "requires AtmosConfig")

	// Test with nil AtmosConfig.
	execCtx := &ExecutionContext{AtmosConfig: nil}
	_, err = fn.Execute(context.Background(), "mystore stack comp key", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
}

func TestStoreGetFunction_Execute(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		storeValue  any
		storeErr    error
		want        any
		wantErr     bool
		errContains string
	}{
		{
			name:       "basic usage",
			args:       "mystore mykey",
			storeValue: "retrieved-value",
			want:       "retrieved-value",
		},
		{
			name:     "with default value on error",
			args:     "mystore mykey | default \"fallback\"",
			storeErr: errors.New("key not found"),
			want:     "fallback",
		},
		{
			name:       "with default value on nil",
			args:       "mystore mykey | default \"fallback\"",
			storeValue: nil,
			want:       "fallback",
		},
		{
			name:        "error without default",
			args:        "mystore mykey",
			storeErr:    errors.New("key not found"),
			wantErr:     true,
			errContains: "failed to get key",
		},
		{
			name:        "store not found",
			args:        "nonexistent mykey",
			wantErr:     true,
			errContains: "store 'nonexistent' not found",
		},
		{
			name:        "invalid argument count - too few",
			args:        "mystore",
			wantErr:     true,
			errContains: "requires 2 parameters",
		},
		{
			name:        "invalid argument count - too many",
			args:        "mystore key1 key2",
			wantErr:     true,
			errContains: "requires 2 parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStore{
				getKeyFunc: func(key string) (any, error) {
					return tt.storeValue, tt.storeErr
				},
			}

			atmosConfig := &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"mystore": mock,
				},
			}

			execCtx := &ExecutionContext{
				AtmosConfig: atmosConfig,
			}

			fn := NewStoreGetFunction()
			result, err := fn.Execute(context.Background(), tt.args, execCtx)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestStoreGetFunction_Execute_NilContext(t *testing.T) {
	fn := NewStoreGetFunction()

	// Test with nil context.
	_, err := fn.Execute(context.Background(), "mystore mykey", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
	assert.Contains(t, err.Error(), "requires AtmosConfig")

	// Test with nil AtmosConfig.
	execCtx := &ExecutionContext{AtmosConfig: nil}
	_, err = fn.Execute(context.Background(), "mystore mykey", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
}

func TestParseStoreParams(t *testing.T) {
	tests := []struct {
		name         string
		args         string
		currentStack string
		wantStore    string
		wantStack    string
		wantComp     string
		wantKey      string
		wantDefault  *string
		wantQuery    string
		wantErr      bool
	}{
		{
			name:         "4 params",
			args:         "store1 stack1 comp1 key1",
			currentStack: "current",
			wantStore:    "store1",
			wantStack:    "stack1",
			wantComp:     "comp1",
			wantKey:      "key1",
		},
		{
			name:         "3 params uses current stack",
			args:         "store1 comp1 key1",
			currentStack: "current",
			wantStore:    "store1",
			wantStack:    "current",
			wantComp:     "comp1",
			wantKey:      "key1",
		},
		{
			name:         "with default value",
			args:         "store1 stack1 comp1 key1 | default \"mydefault\"",
			currentStack: "current",
			wantStore:    "store1",
			wantStack:    "stack1",
			wantComp:     "comp1",
			wantKey:      "key1",
			wantDefault:  strPtr("mydefault"),
		},
		{
			name:         "with query",
			args:         "store1 stack1 comp1 key1 | query \".foo.bar\"",
			currentStack: "current",
			wantStore:    "store1",
			wantStack:    "stack1",
			wantComp:     "comp1",
			wantKey:      "key1",
			wantQuery:    ".foo.bar",
		},
		{
			name:         "with both default and query",
			args:         "store1 stack1 comp1 key1 | default \"def\" | query \".x\"",
			currentStack: "current",
			wantStore:    "store1",
			wantStack:    "stack1",
			wantComp:     "comp1",
			wantKey:      "key1",
			wantDefault:  strPtr("def"),
			wantQuery:    ".x",
		},
		{
			name:         "too few params",
			args:         "store1 comp1",
			currentStack: "current",
			wantErr:      true,
		},
		{
			name:         "too many params",
			args:         "store1 a b c d",
			currentStack: "current",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := parseStoreParams(tt.args, tt.currentStack)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStore, params.storeName)
			assert.Equal(t, tt.wantStack, params.stack)
			assert.Equal(t, tt.wantComp, params.component)
			assert.Equal(t, tt.wantKey, params.key)
			assert.Equal(t, tt.wantQuery, params.query)

			if tt.wantDefault == nil {
				assert.Nil(t, params.defaultValue)
			} else {
				require.NotNil(t, params.defaultValue)
				assert.Equal(t, *tt.wantDefault, *params.defaultValue)
			}
		})
	}
}

func TestParseStoreGetParams(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		wantStore   string
		wantKey     string
		wantDefault *string
		wantQuery   string
		wantErr     bool
	}{
		{
			name:      "basic 2 params",
			args:      "store1 key1",
			wantStore: "store1",
			wantKey:   "key1",
		},
		{
			name:        "with default",
			args:        "store1 key1 | default \"fallback\"",
			wantStore:   "store1",
			wantKey:     "key1",
			wantDefault: strPtr("fallback"),
		},
		{
			name:      "with query",
			args:      "store1 key1 | query \".path\"",
			wantStore: "store1",
			wantKey:   "key1",
			wantQuery: ".path",
		},
		{
			name:    "too few params",
			args:    "store1",
			wantErr: true,
		},
		{
			name:    "too many params",
			args:    "store1 key1 extra",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := parseStoreGetParams(tt.args)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStore, params.storeName)
			assert.Equal(t, tt.wantKey, params.key)
			assert.Equal(t, tt.wantQuery, params.query)

			if tt.wantDefault == nil {
				assert.Nil(t, params.defaultValue)
			} else {
				require.NotNil(t, params.defaultValue)
				assert.Equal(t, *tt.wantDefault, *params.defaultValue)
			}
		})
	}
}

func TestExtractPipeOptions(t *testing.T) {
	tests := []struct {
		name        string
		parts       []string
		wantDefault *string
		wantQuery   string
		wantErr     bool
	}{
		{
			name:  "empty parts",
			parts: []string{},
		},
		{
			name:        "default only",
			parts:       []string{"default \"value\""},
			wantDefault: strPtr("value"),
		},
		{
			name:      "query only",
			parts:     []string{"query \".foo\""},
			wantQuery: ".foo",
		},
		{
			name:        "both default and query",
			parts:       []string{"default \"val\"", "query \".bar\""},
			wantDefault: strPtr("val"),
			wantQuery:   ".bar",
		},
		{
			name:    "invalid - no value",
			parts:   []string{"default"},
			wantErr: true,
		},
		{
			name:    "invalid - unknown key",
			parts:   []string{"unknown \"value\""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defVal, query, err := extractPipeOptions(tt.parts)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantQuery, query)

			if tt.wantDefault == nil {
				assert.Nil(t, defVal)
			} else {
				require.NotNil(t, defVal)
				assert.Equal(t, *tt.wantDefault, *defVal)
			}
		})
	}
}

// strPtr is a helper to create a pointer to a string.
func strPtr(s string) *string {
	return &s
}
