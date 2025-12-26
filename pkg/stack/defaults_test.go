package stack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function"
	"github.com/cloudposse/atmos/pkg/stack/processor"
)

// mockShellExecutor implements function.ShellExecutor for testing.
type mockShellExecutor struct{}

func (m *mockShellExecutor) Execute(ctx context.Context, command, dir string, env []string) (string, error) {
	return "mock output", nil
}

func TestDefaultLoaderRegistry(t *testing.T) {
	r := DefaultLoaderRegistry()

	assert.NotNil(t, r)
	assert.Equal(t, 3, r.Len()) // YAML, JSON, HCL.

	// Verify YAML loader is registered.
	yamlLoader, err := r.GetByExtension(".yaml")
	assert.NoError(t, err)
	assert.Equal(t, "YAML", yamlLoader.Name())

	ymlLoader, err := r.GetByExtension(".yml")
	assert.NoError(t, err)
	assert.Equal(t, "YAML", ymlLoader.Name())

	// Verify JSON loader is registered.
	jsonLoader, err := r.GetByExtension(".json")
	assert.NoError(t, err)
	assert.Equal(t, "JSON", jsonLoader.Name())

	// Verify HCL loader is registered.
	hclLoader, err := r.GetByExtension(".hcl")
	assert.NoError(t, err)
	assert.Equal(t, "HCL", hclLoader.Name())

	tfLoader, err := r.GetByExtension(".tf")
	assert.NoError(t, err)
	assert.Equal(t, "HCL", tfLoader.Name())
}

func TestDefaultProcessor(t *testing.T) {
	t.Run("with shell executor", func(t *testing.T) {
		p := DefaultProcessor(&mockShellExecutor{})

		assert.NotNil(t, p)
		assert.NotNil(t, p.FunctionRegistry())
		assert.NotNil(t, p.LoaderRegistry())

		// Verify function registry has default functions.
		assert.True(t, p.FunctionRegistry().Has("env"))
		assert.True(t, p.FunctionRegistry().Has("template"))
		assert.True(t, p.FunctionRegistry().Has("repo-root"))
		assert.True(t, p.FunctionRegistry().Has("exec"))

		// Verify loader registry has default loaders.
		assert.True(t, p.LoaderRegistry().HasExtension(".yaml"))
		assert.True(t, p.LoaderRegistry().HasExtension(".json"))
		assert.True(t, p.LoaderRegistry().HasExtension(".hcl"))
	})

	t.Run("without shell executor", func(t *testing.T) {
		p := DefaultProcessor(nil)

		assert.NotNil(t, p)
		assert.True(t, p.FunctionRegistry().Has("env"))
		assert.False(t, p.FunctionRegistry().Has("exec")) // Not registered without executor.
	})
}

func TestIsExtensionSupported(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".yaml", true},
		{".yml", true},
		{".json", true},
		{".hcl", true},
		{".tf", true},
		{".txt", false},
		{".xml", false},
		{"yaml", true}, // Without leading dot.
		{"json", true}, // Without leading dot.
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := IsExtensionSupported(tt.ext)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultProcessorIntegration(t *testing.T) {
	p := DefaultProcessor(nil)

	// Test processing data with !env function.
	data := map[string]any{
		"region": "!env AWS_REGION",
	}

	// Set up environment in execution context.
	execCtx := &function.ExecutionContext{
		Env:        map[string]string{"AWS_REGION": "us-west-2"},
		SourceFile: "test.yaml",
	}
	stackCtx := processor.NewStackContext(execCtx)

	// Test post-merge processing.
	result, err := p.ProcessPostMerge(context.Background(), stackCtx, data)
	require.NoError(t, err)
	assert.NotNil(t, result)
}
