package yamlsource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisplayTypedContainers(t *testing.T) {
	source := Source{Tag: "!secret", Argument: "CREDENTIAL"}.Encode()
	const want = "!secret CREDENTIAL"

	tests := []struct {
		name  string
		input any
		want  any
	}{
		{name: "string map", input: map[string]string{"a": source, "b": "plain"}, want: map[string]string{"a": want, "b": "plain"}},
		{name: "string slice", input: []string{"plain", source}, want: []string{"plain", want}},
		{name: "map of slices", input: map[string][]string{"a": {source, "plain"}}, want: map[string][]string{"a": {want, "plain"}}},
		{name: "nested typed maps", input: map[string]map[string]string{"a": {"b": source}}, want: map[string]map[string]string{"a": {"b": want}}},
		{name: "typed map inside any", input: map[string]any{"a": map[string]string{"b": source}}, want: map[string]any{"a": map[string]string{"b": want}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Display(tt.input))
		})
	}
}

func TestDisplayTypedContainersAreIsolatedFromInput(t *testing.T) {
	source := Source{Tag: "!secret", Argument: "CREDENTIAL"}.Encode()
	input := map[string]string{"a": source}

	result, ok := Display(input).(map[string]string)
	require.True(t, ok)

	// Result to source: changing the result leaves the input intact.
	result["a"] = "changed"
	assert.Equal(t, source, input["a"])

	// Source to result: changing the input after the call leaves the result intact.
	result = Display(input).(map[string]string)
	input["a"] = "changed"
	assert.Equal(t, "!secret CREDENTIAL", result["a"])
}

func TestDisplayTypedContainersWithoutSourcesAreReturnedUnchanged(t *testing.T) {
	plain := map[string]string{"a": "!secret CREDENTIAL"}
	assert.Equal(t, plain, Display(plain))

	var nilMap map[string]string
	assert.Nil(t, Display(nilMap))

	var nilSlice []string
	assert.Nil(t, Display(nilSlice))

	data := []byte("raw")
	assert.Equal(t, data, Display(data))

	numbers := map[string]int{"a": 1}
	assert.Equal(t, numbers, Display(numbers))
}
