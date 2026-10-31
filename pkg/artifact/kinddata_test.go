package artifact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestKindDataJSONBehavior(t *testing.T) {
	t.Run("empty kind data marshals to null and cannot be decoded", func(t *testing.T) {
		raw, err := KindData(nil).MarshalJSON()
		require.NoError(t, err)
		assert.Equal(t, "null", string(raw))
		require.ErrorIs(t, KindData(nil).Decode(&struct{}{}), errUtils.ErrArtifactRecordInvalid)
	})

	t.Run("unmarshal canonicalizes and treats null as absent", func(t *testing.T) {
		var data KindData
		require.NoError(t, json.Unmarshal([]byte(` {"b": 1, "a": 2} `), &data))
		assert.Equal(t, `{"a":2,"b":1}`, string(data))
		require.NoError(t, data.UnmarshalJSON([]byte("null")))
		assert.Empty(t, data)
	})

	t.Run("unmarshal rejects floats", func(t *testing.T) {
		var data KindData
		require.ErrorIs(t, json.Unmarshal([]byte(`{"a":1.5}`), &data), errUtils.ErrArtifactRecordInvalid)
	})

	t.Run("marshal failure surfaces as an invalid record", func(t *testing.T) {
		_, err := NewKindData(make(chan int))
		require.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid)
	})
}
