package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseCILabels(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"", nil},
		{" \n ", nil},
		{"[preview]", []string{"preview"}},
		{`[preview, deploy, "needs review", 'a,b', "true", "123"]`, []string{"preview", "deploy", "needs review", "a,b", "true", "123"}},
		{"- preview\n- deploy\n", []string{"preview", "deploy"}},
		{`["!env SECRET", "a:b", "quote\"here"]`, []string{"!env SECRET", "a:b", `quote"here`}},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseCILabels(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseCILabelsInvalid(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"[]", "preview", "{}", "null", "[true]", "[123]", "[null]", "[[preview]]",
		"[{preview: true}]", "[preview", "[!env SECRET]", "[&label preview, *label]",
		"[preview]\n---\n[deploy]", "# only a comment", "!other [preview]", `[""]`,
	} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			_, err := ParseCILabels(input)
			require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
		})
	}
}
