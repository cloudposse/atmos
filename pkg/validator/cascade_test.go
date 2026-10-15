package validator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

// cascadeFixture validates a JSON document against a JSON Schema and returns the raw failures.
func cascadeFixture(t *testing.T, schemaJSON, documentJSON string) []gojsonschema.ResultError {
	t.Helper()
	var document any
	require.NoError(t, json.Unmarshal([]byte(documentJSON), &document))
	result, err := gojsonschema.Validate(gojsonschema.NewStringLoader(schemaJSON), gojsonschema.NewGoLoader(document))
	require.NoError(t, err)
	require.False(t, result.Valid(), "the fixture must fail validation")
	return result.Errors()
}

func cascadeFields(failures []gojsonschema.ResultError) []string {
	fields := make([]string, 0, len(failures))
	for _, failure := range failures {
		fields = append(fields, failure.Field()+"|"+failure.Type())
	}
	return fields
}

func TestFilterCascadeErrors(t *testing.T) {
	t.Run("oneOf ancestors of a deeper oneOf are dropped", func(t *testing.T) {
		failures := cascadeFixture(t, `{"type":"object","properties":{"a":{"oneOf":[{"type":"string"},{"type":"object","properties":{"b":{"oneOf":[{"type":"string"},{"type":"integer"}]}}}]}}}`, `{"a":{"b":true}}`)
		require.Contains(t, cascadeFields(failures), "a|number_one_of")
		require.Contains(t, cascadeFields(failures), "a.b|number_one_of")

		kept := cascadeFields(FilterCascadeErrors(failures))

		assert.NotContains(t, kept, "a|number_one_of")
		assert.NotContains(t, kept, "a.b|number_one_of")
		assert.Contains(t, kept, "a.b|invalid_type")
	})

	t.Run("anyOf ancestors of a specific failure are dropped", func(t *testing.T) {
		failures := cascadeFixture(t, `{"type":"object","properties":{"a":{"anyOf":[{"type":"null"},{"type":"object","properties":{"b":{"type":"integer"}}}]}}}`, `{"a":{"b":"x"}}`)

		kept := FilterCascadeErrors(failures)

		require.Len(t, kept, 1)
		assert.Equal(t, "a.b", kept[0].Field())
		assert.Equal(t, "invalid_type", kept[0].Type())
	})

	t.Run("a oneOf failure beside a specific failure at the same path is dropped", func(t *testing.T) {
		failures := cascadeFixture(t, `{"type":"object","properties":{"a":{"type":"object","required":["z"],"oneOf":[{"required":["x"]},{"required":["y"]}]}}}`, `{"a":{}}`)
		require.Contains(t, cascadeFields(failures), "a|number_one_of")

		kept := FilterCascadeErrors(failures)

		assert.NotContains(t, cascadeFields(kept), "a|number_one_of")
		assert.Contains(t, cascadeFields(kept), "a|required")
	})

	t.Run("the root composite failure yields to any deeper failure", func(t *testing.T) {
		failures := cascadeFixture(t, `{"anyOf":[{"type":"null"},{"type":"object","properties":{"a":{"type":"integer"}}}]}`, `{"a":"x"}`)
		require.Contains(t, cascadeFields(failures), "(root)|number_any_of")

		kept := FilterCascadeErrors(failures)

		assert.Equal(t, []string{"a|invalid_type"}, cascadeFields(kept))
	})

	t.Run("a lone composite failure is kept", func(t *testing.T) {
		failures := cascadeFixture(t, `{"type":"object","properties":{"a":{"oneOf":[{"type":"integer"},{"type":"number"}]}}}`, `{"a":1}`)

		assert.Equal(t, []string{"a|number_one_of"}, cascadeFields(FilterCascadeErrors(failures)))
	})

	t.Run("a composite failure beside an unrelated sibling failure is kept", func(t *testing.T) {
		failures := cascadeFixture(t, `{"type":"object","properties":{"a":{"oneOf":[{"type":"integer"},{"type":"number"}]},"ab":{"type":"integer"}}}`, `{"a":1,"ab":"x"}`)

		kept := cascadeFields(FilterCascadeErrors(failures))

		assert.ElementsMatch(t, []string{"a|number_one_of", "ab|invalid_type"}, kept)
	})

	t.Run("no failures", func(t *testing.T) {
		assert.Empty(t, FilterCascadeErrors(nil))
	})
}
