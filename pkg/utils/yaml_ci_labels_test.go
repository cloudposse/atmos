package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCILabelsYAMLDeferral(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input, want string
	}{
		{"value: !ci.labels", "!ci.labels"},
		{"value: !ci.labels [preview]", `!ci.labels ["preview"]`},
		{`value: !ci.labels [preview, "needs review", 'a,b', "true"]`, `!ci.labels ["preview","needs review","a,b","true"]`},
		{"value: !ci.labels\n  - preview\n  - deploy", `!ci.labels ["preview","deploy"]`},
		{`value: !ci.labels ["!env SECRET"]`, `!ci.labels ["!env SECRET"]`},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := UnmarshalYAML[map[string]any](tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got["value"])
		})
	}
}

func TestCILabelsYAMLInvalid(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"[]", "[123]", "[true]", "[null]", "{}", "[!env TOKEN]", "[[preview]]", "preview"} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			_, err := UnmarshalYAML[map[string]any]("value: !ci.labels " + input)
			require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
		})
	}
}
