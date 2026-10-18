package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

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
			var doc yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tc.input), &doc))
			got, err := EncodeCILabels(doc.Content[0].Content[1])
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCILabelsYAMLInvalid(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"[]", "[123]", "[true]", "[null]", "{}", "[!env TOKEN]", "[[preview]]", "preview", `"[preview]"`, `[""]`} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			var doc yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("value: !ci.labels "+input), &doc))
			_, err := EncodeCILabels(doc.Content[0].Content[1])
			require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
		})
	}
}
