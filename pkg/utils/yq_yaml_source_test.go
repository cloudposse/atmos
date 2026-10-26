package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
)

// TestEvaluateYqExpression_KeepsSkippedFunctionTag verifies that a skipped function carried as transported
// source is selected as its ordinary expression. Decoding the result to Go values drops custom tags, so
// emitting the tag would leave only the argument.
func TestEvaluateYqExpression_KeepsSkippedFunctionTag(t *testing.T) {
	source := yamlsource.Source{Tag: "!secret", Argument: "CREDENTIAL"}.Encode()
	data := map[string]any{
		"credential": source,
		"nested":     map[string]any{"list": []any{source}},
	}

	tests := []struct {
		name string
		yq   string
		want any
	}{
		{name: "scalar", yq: ".credential", want: "!secret CREDENTIAL"},
		{name: "nested list element", yq: ".nested.list[0]", want: "!secret CREDENTIAL"},
		{name: "whole map", yq: ".nested", want: map[string]any{"list": []any{"!secret CREDENTIAL"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateYqExpression(nil, data, tt.yq)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	// The input is only read: the transported source must still be intact for later evaluation.
	assert.Equal(t, source, data["credential"])
	_, tagged := yamlsource.Decode(data["credential"].(string))
	assert.True(t, tagged)
}

// TestEvaluateYqExpression_PlainFunctionString verifies that an ordinary string that looks like a function
// is untouched.
func TestEvaluateYqExpression_PlainFunctionString(t *testing.T) {
	got, err := EvaluateYqExpression(nil, map[string]any{"v": "!secret CREDENTIAL"}, ".v")
	require.NoError(t, err)
	assert.Equal(t, "!secret CREDENTIAL", got)
}
