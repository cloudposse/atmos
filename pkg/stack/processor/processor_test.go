package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function"
	"github.com/cloudposse/atmos/pkg/stack/loader"
)

// mockFunction implements function.Function for testing.
type mockFunction struct {
	name    string
	aliases []string
	phase   function.Phase
	result  any
	err     error
}

func (f *mockFunction) Name() string          { return f.name }
func (f *mockFunction) Aliases() []string     { return f.aliases }
func (f *mockFunction) Phase() function.Phase { return f.phase }
func (f *mockFunction) Execute(ctx context.Context, args string, execCtx *function.ExecutionContext) (any, error) {
	return f.result, f.err
}

func newMockFunction(name string, phase function.Phase, result any) *mockFunction {
	return &mockFunction{
		name:   name,
		phase:  phase,
		result: result,
	}
}

func newMockFunctionWithError(name string, phase function.Phase, err error) *mockFunction {
	return &mockFunction{
		name:  name,
		phase: phase,
		err:   err,
	}
}

func TestNew(t *testing.T) {
	funcRegistry := function.NewRegistry()
	loaderRegistry := loader.NewRegistry()

	p := New(funcRegistry, loaderRegistry)

	assert.NotNil(t, p)
	assert.Same(t, funcRegistry, p.FunctionRegistry())
	assert.Same(t, loaderRegistry, p.LoaderRegistry())
}

func TestProcessorNil(t *testing.T) {
	var p *Processor

	assert.Nil(t, p.FunctionRegistry())
	assert.Nil(t, p.LoaderRegistry())

	_, err := p.ProcessPreMerge(context.Background(), nil, "test.yaml")
	assert.ErrorIs(t, err, errUtils.ErrNilProcessor)

	_, err = p.ProcessPostMerge(context.Background(), NewStackContext(nil), nil)
	assert.ErrorIs(t, err, errUtils.ErrNilProcessor)
}

func TestProcessPreMerge(t *testing.T) {
	tests := []struct {
		name      string
		data      any
		functions []*mockFunction
		expected  any
		expectErr bool
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: nil,
		},
		{
			name: "simple map without functions",
			data: map[string]any{
				"key": "value",
			},
			expected: map[string]any{
				"key": "value",
			},
		},
		{
			name: "map with pre-merge function",
			data: map[string]any{
				"region": "!env AWS_REGION",
			},
			functions: []*mockFunction{
				newMockFunction("env", function.PreMerge, "us-east-1"),
			},
			expected: map[string]any{
				"region": "us-east-1",
			},
		},
		{
			name: "map with post-merge function (not processed)",
			data: map[string]any{
				"vpc_id": "!terraform.output vpc/vpc_id",
			},
			functions: []*mockFunction{
				newMockFunction("terraform.output", function.PostMerge, "vpc-12345"),
			},
			expected: map[string]any{
				"vpc_id": "!terraform.output vpc/vpc_id",
			},
		},
		{
			name: "nested map with function",
			data: map[string]any{
				"vars": map[string]any{
					"region": "!env AWS_REGION",
				},
			},
			functions: []*mockFunction{
				newMockFunction("env", function.PreMerge, "us-west-2"),
			},
			expected: map[string]any{
				"vars": map[string]any{
					"region": "us-west-2",
				},
			},
		},
		{
			name: "slice with function",
			data: map[string]any{
				"regions": []any{"!env AWS_REGION", "us-west-2"},
			},
			functions: []*mockFunction{
				newMockFunction("env", function.PreMerge, "us-east-1"),
			},
			expected: map[string]any{
				"regions": []any{"us-east-1", "us-west-2"},
			},
		},
		{
			name: "function execution error",
			data: map[string]any{
				"value": "!failing_func arg",
			},
			functions: []*mockFunction{
				newMockFunctionWithError("failing_func", function.PreMerge, errors.New("execution failed")),
			},
			expectErr: true,
		},
		{
			name: "unknown function preserved",
			data: map[string]any{
				"value": "!unknown_function arg",
			},
			expected: map[string]any{
				"value": "!unknown_function arg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			funcRegistry := function.NewRegistry()
			for _, fn := range tt.functions {
				err := funcRegistry.Register(fn)
				require.NoError(t, err)
			}

			p := New(funcRegistry, loader.NewRegistry())
			result, err := p.ProcessPreMerge(context.Background(), tt.data, "test.yaml")

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessPostMerge(t *testing.T) {
	tests := []struct {
		name      string
		data      any
		functions []*mockFunction
		stackCtx  *StackContext
		expected  any
		expectErr bool
	}{
		{
			name:      "nil context error",
			data:      map[string]any{"key": "value"},
			stackCtx:  nil,
			expectErr: true,
		},
		{
			name: "post-merge function processed",
			data: map[string]any{
				"vpc_id": "!terraform.output vpc/vpc_id",
			},
			functions: []*mockFunction{
				newMockFunction("terraform.output", function.PostMerge, "vpc-12345"),
			},
			stackCtx: NewStackContext(nil),
			expected: map[string]any{
				"vpc_id": "vpc-12345",
			},
		},
		{
			name: "pre-merge function not processed in post-merge",
			data: map[string]any{
				"region": "!env AWS_REGION",
			},
			functions: []*mockFunction{
				newMockFunction("env", function.PreMerge, "us-east-1"),
			},
			stackCtx: NewStackContext(nil),
			expected: map[string]any{
				"region": "!env AWS_REGION",
			},
		},
		{
			name: "skipped function preserved",
			data: map[string]any{
				"vpc_id": "!terraform.output vpc/vpc_id",
			},
			functions: []*mockFunction{
				newMockFunction("terraform.output", function.PostMerge, "vpc-12345"),
			},
			stackCtx: NewStackContext(nil).WithSkip([]string{"terraform.output"}),
			expected: map[string]any{
				"vpc_id": "!terraform.output vpc/vpc_id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			funcRegistry := function.NewRegistry()
			for _, fn := range tt.functions {
				err := funcRegistry.Register(fn)
				require.NoError(t, err)
			}

			p := New(funcRegistry, loader.NewRegistry())
			result, err := p.ProcessPostMerge(context.Background(), tt.stackCtx, tt.data)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessDataContextCancellation(t *testing.T) {
	funcRegistry := function.NewRegistry()
	p := New(funcRegistry, loader.NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.ProcessPreMerge(ctx, map[string]any{"key": "value"}, "test.yaml")
	assert.Error(t, err)
}

func TestDetectFunctions(t *testing.T) {
	tests := []struct {
		name     string
		data     any
		expected []string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: nil,
		},
		{
			name: "no functions",
			data: map[string]any{
				"key": "value",
			},
			expected: nil,
		},
		{
			name: "single function",
			data: map[string]any{
				"region": "!env AWS_REGION",
			},
			expected: []string{"env"},
		},
		{
			name: "multiple functions",
			data: map[string]any{
				"region": "!env AWS_REGION",
				"vpc_id": "!terraform.output vpc/vpc_id",
			},
			expected: []string{"env", "terraform.output"},
		},
		{
			name: "nested functions",
			data: map[string]any{
				"vars": map[string]any{
					"region": "!env AWS_REGION",
				},
				"outputs": []any{
					"!terraform.output vpc/vpc_id",
				},
			},
			expected: []string{"env", "terraform.output"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(function.NewRegistry(), loader.NewRegistry())
			result := p.DetectFunctions(tt.data)

			if tt.expected == nil {
				assert.Empty(t, result)
			} else {
				assert.ElementsMatch(t, tt.expected, result)
			}
		})
	}
}

func TestHasFunctions(t *testing.T) {
	p := New(function.NewRegistry(), loader.NewRegistry())

	assert.False(t, p.HasFunctions(nil))
	assert.False(t, p.HasFunctions(map[string]any{"key": "value"}))
	assert.True(t, p.HasFunctions(map[string]any{"key": "!env VAR"}))
}

func TestHasPreMergeFunctions(t *testing.T) {
	funcRegistry := function.NewRegistry()
	err := funcRegistry.Register(newMockFunction("env", function.PreMerge, nil))
	require.NoError(t, err)
	err = funcRegistry.Register(newMockFunction("terraform.output", function.PostMerge, nil))
	require.NoError(t, err)

	p := New(funcRegistry, loader.NewRegistry())

	assert.False(t, p.HasPreMergeFunctions(nil))
	assert.False(t, p.HasPreMergeFunctions(map[string]any{"key": "value"}))
	assert.True(t, p.HasPreMergeFunctions(map[string]any{"key": "!env VAR"}))
	assert.False(t, p.HasPreMergeFunctions(map[string]any{"key": "!terraform.output vpc/id"}))
}

func TestHasPostMergeFunctions(t *testing.T) {
	funcRegistry := function.NewRegistry()
	err := funcRegistry.Register(newMockFunction("env", function.PreMerge, nil))
	require.NoError(t, err)
	err = funcRegistry.Register(newMockFunction("terraform.output", function.PostMerge, nil))
	require.NoError(t, err)

	p := New(funcRegistry, loader.NewRegistry())

	assert.False(t, p.HasPostMergeFunctions(nil))
	assert.False(t, p.HasPostMergeFunctions(map[string]any{"key": "value"}))
	assert.False(t, p.HasPostMergeFunctions(map[string]any{"key": "!env VAR"}))
	assert.True(t, p.HasPostMergeFunctions(map[string]any{"key": "!terraform.output vpc/id"}))
}

func TestParseFunction(t *testing.T) {
	p := New(function.NewRegistry(), loader.NewRegistry())

	tests := []struct {
		input  string
		name   string
		args   string
		isFunc bool
	}{
		{"!env AWS_REGION", "env", "AWS_REGION", true},
		{"!terraform.output vpc/vpc_id", "terraform.output", "vpc/vpc_id", true},
		{"!exec echo hello", "exec", "echo hello", true},
		{"!env", "env", "", true},
		{"regular string", "", "", false},
		{"", "", "", false},
		{"!", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, args, isFunc := p.parseFunction(tt.input)
			assert.Equal(t, tt.name, name)
			assert.Equal(t, tt.args, args)
			assert.Equal(t, tt.isFunc, isFunc)
		})
	}
}
