package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestNewComponentYAMLProcessor tests the NewComponentYAMLProcessor constructor.
func TestNewComponentYAMLProcessor(t *testing.T) {
	t.Run("creates processor with all fields", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		currentStack := "test-stack"
		skip := []string{"skip1", "skip2"}
		resolutionCtx := &ResolutionContext{}
		stackInfo := &schema.ConfigAndStacksInfo{}

		processor := NewComponentYAMLProcessor(atmosConfig, currentStack, skip, resolutionCtx, stackInfo)

		require.NotNil(t, processor)

		// Verify it implements the interface.
		_, ok := processor.(interface {
			ProcessYAMLFunctionString(value string) (any, error)
		})
		assert.True(t, ok, "should implement YAMLFunctionProcessor interface")
	})

	t.Run("creates processor with nil values", func(t *testing.T) {
		processor := NewComponentYAMLProcessor(nil, "", nil, nil, nil)

		require.NotNil(t, processor)
	})
}

// TestComponentYAMLProcessor_ProcessYAMLFunctionString tests the ProcessYAMLFunctionString method.
func TestComponentYAMLProcessor_ProcessYAMLFunctionString(t *testing.T) {
	t.Run("processes template function", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		processor := NewComponentYAMLProcessor(atmosConfig, "", nil, nil, nil)

		// Simple template that doesn't require context.
		result, err := processor.ProcessYAMLFunctionString("!template 'hello'")

		require.NoError(t, err)
		// Template returns the rendered string (with quotes preserved).
		assert.Equal(t, "'hello'", result)
	})

	t.Run("processes non-YAML function string", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		processor := NewComponentYAMLProcessor(atmosConfig, "", nil, nil, nil)

		// Regular string should be returned as-is.
		result, err := processor.ProcessYAMLFunctionString("regular string")

		require.NoError(t, err)
		assert.Equal(t, "regular string", result)
	})

	t.Run("processes empty string", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		processor := NewComponentYAMLProcessor(atmosConfig, "", nil, nil, nil)

		result, err := processor.ProcessYAMLFunctionString("")

		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("processes template with JSON", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		processor := NewComponentYAMLProcessor(atmosConfig, "", nil, nil, nil)

		// Template with JSON object.
		result, err := processor.ProcessYAMLFunctionString(`!template '{"key": "value"}'`)

		require.NoError(t, err)
		// processCustomTagsWithContext returns the rendered template string.
		// JSON decoding happens later via ProcessTemplateTagsOnly.
		assert.Equal(t, `'{"key": "value"}'`, result)
	})

	t.Run("processes template with JSON array", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		processor := NewComponentYAMLProcessor(atmosConfig, "", nil, nil, nil)

		// Template with JSON array.
		result, err := processor.ProcessYAMLFunctionString(`!template '[1, 2, 3]'`)

		require.NoError(t, err)
		// processCustomTagsWithContext returns the rendered template string.
		// JSON decoding happens later via ProcessTemplateTagsOnly.
		assert.Equal(t, `'[1, 2, 3]'`, result)
	})
}

// TestComponentYAMLProcessor_Integration tests integration with processCustomTagsWithContext.
func TestComponentYAMLProcessor_Integration(t *testing.T) {
	t.Run("integrates with processCustomTagsWithContext", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		processor := NewComponentYAMLProcessor(atmosConfig, "", nil, nil, nil)

		// This should call processCustomTagsWithContext under the hood.
		result, err := processor.ProcessYAMLFunctionString("!template 'test'")

		require.NoError(t, err)
		assert.NotNil(t, result)
	})
}
