package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIncludePreservesYQEvalExpressions(t *testing.T) {
	for _, tc := range []struct {
		input string
		query string
		eval  bool
	}{
		{`config.yaml .foo | "eval"`, `.foo | "eval"`, false},
		{`config.yaml .foo | "eval" | length`, `.foo | "eval" | length`, false},
		{`config.yaml . | eval (.path)`, `. | eval (.path)`, false},
		{`config.yaml . | eval(.path)`, `. | eval(.path)`, false},
		{`config.yaml . | eval (.path) | eval`, `. | eval (.path)`, true},
		{`config.yaml .foo | "eval" | eval`, `.foo | "eval"`, true},
		{`config.yaml .foo | eval`, `.foo`, true},
		{`config.yaml | eval`, ``, true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseInclude(tc.input)
			require.NoError(t, err)
			assert.Equal(t, IncludeArgs{Path: "config.yaml", Query: tc.query, Eval: tc.eval}, got)
		})
	}
}
