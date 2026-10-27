package setflag

import (
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseEntry(t *testing.T) {
	for _, tc := range []struct {
		name, entry, key, value string
		wantDetail              string
	}{
		{name: "plain", entry: "key=value", key: "key", value: "value"},
		{name: "trimmed", entry: " key = value ", key: "key", value: "value"},
		{name: "value keeps equals", entry: "url=a=b", key: "url", value: "a=b"},
		{name: "empty value", entry: "key=", key: "key", value: ""},
		{name: "missing equals", entry: "environment", wantDetail: "Malformed --set flag (missing '='): environment"},
		{name: "empty key", entry: "=value", wantDetail: "Malformed --set flag (empty key): =value"},
		{name: "blank key", entry: "  =value", wantDetail: "Malformed --set flag (empty key):   =value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, value, err := ParseEntry(tc.entry)
			if tc.wantDetail != "" {
				require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
				assert.Contains(t, strings.Join(cockroach.GetAllDetails(err), "\n"), tc.wantDetail)
				assert.Contains(t, strings.Join(cockroach.GetAllHints(err), "\n"), "Use the format: --set key=value")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.key, key)
			assert.Equal(t, tc.value, value)
		})
	}
}

func TestValidateReportsFirstMalformedEntry(t *testing.T) {
	require.NoError(t, Validate(nil))
	require.NoError(t, Validate([]string{"a=1", "b=2"}))
	err := Validate([]string{"a=1", "environment", "=x"})
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	details := strings.Join(cockroach.GetAllDetails(err), "\n")
	assert.Contains(t, details, "environment")
	assert.NotContains(t, details, "=x")
}
