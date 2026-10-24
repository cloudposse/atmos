package execbatch_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.ExecFunctionSettings{MaxConcurrency: nil}

func TestParallelExecDocument(t *testing.T) {
	for _, limit := range []int{1, 3} {
		config := &schema.AtmosConfiguration{}
		config.Settings.Functions.Exec.MaxConcurrency = &limit
		input := map[string]any{"vars": map[string]any{
			"one": "!exec printf 'first\\n'", "nested": []any{"!exec printf '[1,2]'", "!exec printf '!env MUST_NOT_BE_EVALUATED'"},
		}}
		got, err := exec.ProcessCustomYamlTags(config, input, "test", nil, nil)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"vars": map[string]any{"one": "first", "nested": []any{[]any{float64(1), float64(2)}, "!env MUST_NOT_BE_EVALUATED"}}}, got)
		skipped, err := exec.ProcessCustomYamlTags(config, input, "test", []string{"exec"}, nil)
		require.NoError(t, err)
		assert.Equal(t, input, skipped)
	}
}

func TestParallelExecWithStarlarkReferences(t *testing.T) {
	limit := 2
	config := &schema.AtmosConfiguration{}
	config.Settings.Functions.Exec.MaxConcurrency = &limit
	input := map[string]any{"vars": map[string]any{
		"count":    "!exec printf '3'",
		"computed": (starlarksource.Source{Code: `return ctx.vars["count"] * 2`, File: "test.yaml", Line: 1}).Encode(),
	}}
	info := &schema.ConfigAndStacksInfo{ComponentSection: input}
	got, err := exec.ProcessCustomYamlTags(config, input, "dev", nil, info)
	require.NoError(t, err)
	assert.Equal(t, float64(3), got["vars"].(map[string]any)["count"])
	assert.EqualValues(t, 6, got["vars"].(map[string]any)["computed"])
}
