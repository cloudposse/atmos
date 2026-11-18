package artifact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestKindDataJSONBehavior(t *testing.T) {
	data, err := NewKindData(nil)
	require.NoError(t, err)
	assert.Empty(t, data)
	record := vectorRecordB(t)
	record.KindData = data
	raw, err := record.Canonical()
	require.NoError(t, err)
	decoded, err := DecodeRecord(raw)
	require.NoError(t, err)
	assert.Empty(t, decoded.KindData)
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

	t.Run("decode failures carry the artifact sentinel", func(t *testing.T) {
		type target struct {
			Count int `json:"count"`
		}
		tests := []struct {
			name string
			data KindData
			want int
		}{
			{"string where an integer is required", KindData(`{"count":"x"}`), 0},
			{"array where an object is required", KindData(`[1]`), 0},
			{"matching data decodes", KindData(`{"count":7}`), 7},
		}
		for _, tt := range tests {
			var got target
			err := tt.data.Decode(&got)
			if tt.want == 0 {
				require.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid, tt.name)
				var typeErr *json.UnmarshalTypeError
				require.ErrorAs(t, err, &typeErr, "the cause is preserved")
				continue
			}
			require.NoError(t, err, tt.name)
			assert.Equal(t, tt.want, got.Count)
		}
	})

	t.Run("marshal failure surfaces as an invalid record", func(t *testing.T) {
		_, err := NewKindData(make(chan int))
		require.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid)
	})
}
