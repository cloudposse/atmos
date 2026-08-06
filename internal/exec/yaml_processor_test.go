package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

// templatingEnabledConfig returns a minimal *schema.AtmosConfiguration with Go-template
// processing enabled (required for ProcessTmplWithDatasources to actually render, rather than
// short-circuit and return the input unchanged).
func templatingEnabledConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}
}

// TestNewTemplateAwareYAMLProcessor tests the NewTemplateAwareYAMLProcessor constructor.
func TestNewTemplateAwareYAMLProcessor(t *testing.T) {
	t.Run("creates processor with all fields", func(t *testing.T) {
		processor := NewTemplateAwareYAMLProcessor(&TemplateAwareYAMLProcessorOptions{
			AtmosConfig:              templatingEnabledConfig(),
			ConfigAndStacksInfo:      &schema.ConfigAndStacksInfo{Stack: "test-stack"},
			SettingsSection:          schema.Settings{},
			ComponentTemplateContext: map[string]any{"foo": "bar"},
			ResolutionCtx:            &ResolutionContext{},
		})

		require.NotNil(t, processor)
	})
}

// TestTemplateAwareYAMLProcessor_ProcessYAMLFunctionString tests the ProcessYAMLFunctionString method.
func TestTemplateAwareYAMLProcessor_ProcessYAMLFunctionString(t *testing.T) {
	t.Run("delegates non-!template values to the inner processor unchanged", func(t *testing.T) {
		processor := NewTemplateAwareYAMLProcessor(&TemplateAwareYAMLProcessorOptions{
			AtmosConfig:         templatingEnabledConfig(),
			ConfigAndStacksInfo: &schema.ConfigAndStacksInfo{Stack: "test-stack"},
			SettingsSection:     schema.Settings{},
			// No template context needed: this value never reaches the template-render path.
			ResolutionCtx: &ResolutionContext{},
		})

		result, err := processor.ProcessYAMLFunctionString("regular string")

		require.NoError(t, err)
		assert.Equal(t, "regular string", result)
	})

	t.Run("fast-paths a !template value with no {{ }} expression without requiring template context", func(t *testing.T) {
		processor := NewTemplateAwareYAMLProcessor(&TemplateAwareYAMLProcessorOptions{
			AtmosConfig:         templatingEnabledConfig(),
			ConfigAndStacksInfo: &schema.ConfigAndStacksInfo{Stack: "test-stack"},
			SettingsSection:     schema.Settings{},
			// Must not be required: the fast path skips the render call entirely.
			ResolutionCtx: &ResolutionContext{},
		})

		result, err := processor.ProcessYAMLFunctionString("!template 'hello'")

		require.NoError(t, err)
		assert.Equal(t, "'hello'", result)
	})

	t.Run("renders a deferred !template value's {{ }} expression against the component template context", func(t *testing.T) {
		processor := NewTemplateAwareYAMLProcessor(&TemplateAwareYAMLProcessorOptions{
			AtmosConfig:              templatingEnabledConfig(),
			ConfigAndStacksInfo:      &schema.ConfigAndStacksInfo{Stack: "test-stack"},
			SettingsSection:          schema.Settings{},
			ComponentTemplateContext: map[string]any{"foo": "bar"},
			ResolutionCtx:            &ResolutionContext{},
		})

		result, err := processor.ProcessYAMLFunctionString(`!template '{{ .foo }}'`)

		require.NoError(t, err)
		// The rendered value ('bar') isn't valid JSON (single-quoted), so processTagTemplate's
		// inner json.Unmarshal fails and it falls back to returning the rendered text as-is —
		// mirroring ComponentYAMLProcessor's own "quotes preserved" test above. What matters here
		// is that {{ .foo }} was substituted with "bar" before that fallback ran.
		assert.Equal(t, "'bar'", result)
	})

	t.Run("fails loudly instead of silently skipping when template context is unavailable", func(t *testing.T) {
		processor := NewTemplateAwareYAMLProcessor(&TemplateAwareYAMLProcessorOptions{
			AtmosConfig:         templatingEnabledConfig(),
			ConfigAndStacksInfo: &schema.ConfigAndStacksInfo{Stack: "test-stack"},
			SettingsSection:     schema.Settings{},
			// processTemplates was disabled for this invocation.
			ResolutionCtx: &ResolutionContext{},
		})

		_, err := processor.ProcessYAMLFunctionString(`!template '{{ .foo }}'`)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrDeferredTemplateContextMissing)
	})

	t.Run("fails loudly on a render error instead of silently returning unrendered text", func(t *testing.T) {
		processor := NewTemplateAwareYAMLProcessor(&TemplateAwareYAMLProcessorOptions{
			AtmosConfig:              templatingEnabledConfig(),
			ConfigAndStacksInfo:      &schema.ConfigAndStacksInfo{Stack: "test-stack"},
			SettingsSection:          schema.Settings{},
			ComponentTemplateContext: map[string]any{"foo": "bar"},
			ResolutionCtx:            &ResolutionContext{},
		})

		// Unclosed Go template action: always a render error, regardless of context contents.
		result, err := processor.ProcessYAMLFunctionString(`!template '{{ .foo'`)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrRenderTemplate)
		// Must not silently fall back to unrendered/partial text.
		assert.Nil(t, result)
	})
}
