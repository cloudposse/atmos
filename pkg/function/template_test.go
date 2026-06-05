package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTemplateFunction(t *testing.T) {
	fn := NewTemplateFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagTemplate, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
	assert.Nil(t, fn.Aliases())
}

func TestTemplateFunction_Execute_EmptyString(t *testing.T) {
	fn := NewTemplateFunction()

	result, err := fn.Execute(context.Background(), "", nil)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestTemplateFunction_Execute_WhitespaceOnly(t *testing.T) {
	fn := NewTemplateFunction()

	result, err := fn.Execute(context.Background(), "   ", nil)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestTemplateFunction_Execute_JSONString(t *testing.T) {
	fn := NewTemplateFunction()

	// JSON string value.
	result, err := fn.Execute(context.Background(), `"hello"`, nil)
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestTemplateFunction_Execute_JSONNumber(t *testing.T) {
	fn := NewTemplateFunction()

	// JSON number.
	result, err := fn.Execute(context.Background(), `42`, nil)
	require.NoError(t, err)
	assert.Equal(t, float64(42), result)
}

func TestTemplateFunction_Execute_JSONBoolean(t *testing.T) {
	fn := NewTemplateFunction()

	// JSON boolean true.
	result, err := fn.Execute(context.Background(), `true`, nil)
	require.NoError(t, err)
	assert.Equal(t, true, result)

	// JSON boolean false.
	result, err = fn.Execute(context.Background(), `false`, nil)
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestTemplateFunction_Execute_JSONNull(t *testing.T) {
	fn := NewTemplateFunction()

	// JSON null.
	result, err := fn.Execute(context.Background(), `null`, nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestTemplateFunction_Execute_NestedJSON(t *testing.T) {
	fn := NewTemplateFunction()

	input := `{"outer": {"inner": {"value": 123}}}`
	result, err := fn.Execute(context.Background(), input, nil)
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)

	outer, ok := m["outer"].(map[string]any)
	require.True(t, ok)

	inner, ok := outer["inner"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, float64(123), inner["value"])
}

func TestTemplateFunction_Execute_InvalidJSON(t *testing.T) {
	fn := NewTemplateFunction()

	// Invalid JSON should be returned as-is.
	result, err := fn.Execute(context.Background(), `{invalid json}`, nil)
	require.NoError(t, err)
	assert.Equal(t, `{invalid json}`, result)
}

func TestProcessTemplateTagsOnly_Nil(t *testing.T) {
	result := ProcessTemplateTagsOnly(nil)
	assert.Nil(t, result)
}

func TestProcessTemplateTagsOnly_Empty(t *testing.T) {
	result := ProcessTemplateTagsOnly(map[string]any{})
	assert.Empty(t, result)
}

func TestProcessTemplateTagsOnly_NoTemplates(t *testing.T) {
	input := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	result := ProcessTemplateTagsOnly(input)

	assert.Equal(t, "value1", result["key1"])
	assert.Equal(t, 42, result["key2"])
	assert.Equal(t, true, result["key3"])
}

func TestProcessTemplateTagsOnly_WithTemplateTag(t *testing.T) {
	input := map[string]any{
		"regular":   "value",
		"templated": "!template [1, 2, 3]",
	}

	result := ProcessTemplateTagsOnly(input)

	assert.Equal(t, "value", result["regular"])

	arr, ok := result["templated"].([]any)
	require.True(t, ok)
	assert.Len(t, arr, 3)
	assert.Equal(t, float64(1), arr[0])
	assert.Equal(t, float64(2), arr[1])
	assert.Equal(t, float64(3), arr[2])
}

func TestProcessTemplateTagsOnly_NestedMap(t *testing.T) {
	input := map[string]any{
		"parent": map[string]any{
			"child": "!template {\"nested\": true}",
		},
	}

	result := ProcessTemplateTagsOnly(input)

	parent, ok := result["parent"].(map[string]any)
	require.True(t, ok)

	child, ok := parent["child"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, true, child["nested"])
}

func TestProcessTemplateTagsOnly_NestedSlice(t *testing.T) {
	input := map[string]any{
		"items": []any{
			"!template [\"a\", \"b\"]",
			"regular",
		},
	}

	result := ProcessTemplateTagsOnly(input)

	items, ok := result["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	// First item should be parsed as array.
	arr, ok := items[0].([]any)
	require.True(t, ok)
	assert.Equal(t, "a", arr[0])
	assert.Equal(t, "b", arr[1])

	// Second item should remain as string.
	assert.Equal(t, "regular", items[1])
}

func TestProcessTemplateTagsOnly_InvalidTemplateJSON(t *testing.T) {
	input := map[string]any{
		"invalid": "!template {not valid json}",
	}

	result := ProcessTemplateTagsOnly(input)

	// Invalid JSON should return the args portion as-is.
	assert.Equal(t, "{not valid json}", result["invalid"])
}

func TestProcessTemplateTagsOnly_OtherTags(t *testing.T) {
	input := map[string]any{
		"env_var":  "!env MY_VAR",
		"exec_cmd": "!exec echo hello",
		"store":    "!store mystore stack comp key",
	}

	result := ProcessTemplateTagsOnly(input)

	// Other tags should remain untouched.
	assert.Equal(t, "!env MY_VAR", result["env_var"])
	assert.Equal(t, "!exec echo hello", result["exec_cmd"])
	assert.Equal(t, "!store mystore stack comp key", result["store"])
}

func TestProcessTemplateTagsOnly_MixedContent(t *testing.T) {
	input := map[string]any{
		"string":       "plain string",
		"number":       42,
		"bool":         true,
		"nil_value":    nil,
		"template_obj": "!template {\"key\": \"value\"}",
		"template_arr": "!template [1, 2, 3]",
		"env_tag":      "!env HOME",
		"nested": map[string]any{
			"deep_template": "!template {\"deep\": true}",
			"deep_string":   "just a string",
		},
		"list": []any{
			"!template {\"in_list\": true}",
			"regular item",
			42,
		},
	}

	result := ProcessTemplateTagsOnly(input)

	// Regular values unchanged.
	assert.Equal(t, "plain string", result["string"])
	assert.Equal(t, 42, result["number"])
	assert.Equal(t, true, result["bool"])
	assert.Nil(t, result["nil_value"])

	// Template tags processed.
	obj, ok := result["template_obj"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", obj["key"])

	arr, ok := result["template_arr"].([]any)
	require.True(t, ok)
	assert.Len(t, arr, 3)

	// Other tags preserved.
	assert.Equal(t, "!env HOME", result["env_tag"])

	// Nested map processed.
	nested, ok := result["nested"].(map[string]any)
	require.True(t, ok)
	deepTemplate, ok := nested["deep_template"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, deepTemplate["deep"])
	assert.Equal(t, "just a string", nested["deep_string"])

	// List processed.
	list, ok := result["list"].([]any)
	require.True(t, ok)
	listItem, ok := list[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, listItem["in_list"])
	assert.Equal(t, "regular item", list[1])
	assert.Equal(t, 42, list[2])
}

func TestProcessTemplateTagsOnly_EmptyTemplate(t *testing.T) {
	input := map[string]any{
		"empty": "!template ",
	}

	result := ProcessTemplateTagsOnly(input)

	// Empty template args should be returned as empty string.
	assert.Equal(t, "", result["empty"])
}

func TestProcessTemplateTagsOnly_WhitespaceTemplate(t *testing.T) {
	input := map[string]any{
		"whitespace": "!template    ",
	}

	result := ProcessTemplateTagsOnly(input)

	// Whitespace-only template args should be trimmed to empty string.
	assert.Equal(t, "", result["whitespace"])
}

func TestYAMLTag_Template(t *testing.T) {
	// Verify the tag format is correct.
	expected := "!template"
	assert.Equal(t, expected, YAMLTag(TagTemplate))
}
