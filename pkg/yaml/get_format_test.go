package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestGetFormatted(t *testing.T) {
	for _, tc := range []struct{ yaml, want string }{
		{`value: "false"`, `"false"`},
		{`value: false`, `false`},
		{`value: "123"`, `"123"`},
		{`value: 123`, `123`},
		{`value: "null"`, `"null"`},
		{`value: ""`, `""`},
		{`value: {items: [one, 2, true]}`, `{"items":["one",2,true]}`},
	} {
		t.Run(tc.yaml, func(t *testing.T) {
			value, err := GetFormatted([]byte(tc.yaml), "value", "json")
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, value)
		})
	}
	raw, err := GetFormatted([]byte(`value: "false"`), "value", "raw")
	require.NoError(t, err)
	assert.Equal(t, "false", raw)
	_, err = GetFormatted([]byte(`value: false`), "missing", "json")
	assert.ErrorIs(t, err, ErrYAMLPathNotFound)
	_, err = GetFormatted([]byte(`value: false`), "value", "invalid")
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}
