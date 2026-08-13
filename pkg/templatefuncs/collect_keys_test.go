package templatefuncs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCollectKeys_TopLevel(t *testing.T) {
	m := map[string]interface{}{"staging": nil, "dev": nil, "production": nil}

	got, err := CollectKeys(m)
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "production", "staging"}, got)
}

func TestCollectKeys_NestedFlattensAndDedupes(t *testing.T) {
	m := map[string]interface{}{
		"dev": map[string]interface{}{
			"regions": map[string]interface{}{"us-east-1": nil},
		},
		"staging": map[string]interface{}{
			"regions": map[string]interface{}{"us-east-1": nil},
		},
		"production": map[string]interface{}{
			"regions": map[string]interface{}{"us-east-1": nil, "us-west-2": nil},
		},
	}

	got, err := CollectKeys(m, "regions")
	require.NoError(t, err)
	assert.Equal(t, []string{"us-east-1", "us-west-2"}, got)
}

func TestCollectKeys_NestedSkipsEntriesMissingTheKey(t *testing.T) {
	m := map[string]interface{}{
		"dev":     map[string]interface{}{"regions": map[string]interface{}{"us-east-1": nil}},
		"staging": map[string]interface{}{"other": "value"},
		"weird":   "not a map at all",
	}

	got, err := CollectKeys(m, "regions")
	require.NoError(t, err)
	assert.Equal(t, []string{"us-east-1"}, got)
}

func TestCollectKeys_NotAMapErrors(t *testing.T) {
	_, err := CollectKeys("not a map")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCollectKeysNotMap)
}

func TestCollectKeys_EmptyMapYieldsEmptySlice(t *testing.T) {
	got, err := CollectKeys(map[string]interface{}{})
	require.NoError(t, err)
	assert.Empty(t, got)
}
