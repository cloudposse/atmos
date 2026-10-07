package pro

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSendChunked_SkewedSizes(t *testing.T) {
	t.Parallel()

	type request struct {
		Items []string `json:"items"`
		*BatchInfo
	}
	items := make([]string, 100)
	for i := range items {
		items[i] = "small"
		if i < 10 {
			items[i] = strings.Repeat("x", 1000)
		}
	}
	const budget = 10000
	var received []string
	err := sendChunked(items, budget, metadataOverhead(request{Items: []string{}}), func(chunk []string, batch *BatchInfo) error {
		data, err := json.Marshal(request{Items: chunk, BatchInfo: batch})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(data), budget, "every serialized request must fit")
		received = append(received, chunk...)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, items, received)
}

func TestMetadataOverhead(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	overhead := metadataOverhead(testStruct{Name: "test", Value: 42})
	expected, _ := json.Marshal(testStruct{Name: "test", Value: 42})
	assert.Equal(t, len(expected), overhead)
}

func TestSendChunked(t *testing.T) {
	t.Parallel()

	t.Run("small payload sends without batch info", func(t *testing.T) {
		t.Parallel()
		items := []string{"a", "b", "c"}
		var calls []struct {
			chunk []string
			batch *BatchInfo
		}

		err := sendChunked(items, 0, 10, func(chunk []string, batch *BatchInfo) error {
			calls = append(calls, struct {
				chunk []string
				batch *BatchInfo
			}{chunk, batch})
			return nil
		})

		require.NoError(t, err)
		require.Len(t, calls, 1)
		assert.Equal(t, items, calls[0].chunk)
		assert.Nil(t, calls[0].batch, "small payloads should not have batch info")
	})

	t.Run("empty items sends without batch info", func(t *testing.T) {
		t.Parallel()
		var calls int
		err := sendChunked([]string{}, 0, 10, func(chunk []string, batch *BatchInfo) error {
			calls++
			assert.Empty(t, chunk)
			assert.Nil(t, batch)
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 1, calls)
	})

	t.Run("large payload is chunked with batch info", func(t *testing.T) {
		t.Parallel()
		// Create items that will exceed DefaultMaxPayloadBytes.
		// Each item is ~1000 bytes when serialized.
		largeString := make([]byte, 900)
		for i := range largeString {
			largeString[i] = 'x'
		}
		item := string(largeString)

		// Create enough items to exceed the default budget.
		numItems := (DefaultMaxPayloadBytes / 900) + 100
		items := make([]string, numItems)
		for i := range items {
			items[i] = item
		}

		var calls []struct {
			chunkLen int
			batch    *BatchInfo
		}

		err := sendChunked(items, 0, 100, func(chunk []string, batch *BatchInfo) error {
			calls = append(calls, struct {
				chunkLen int
				batch    *BatchInfo
			}{len(chunk), batch})
			return nil
		})

		require.NoError(t, err)
		assert.Greater(t, len(calls), 1, "should have multiple chunks")

		// Verify batch metadata.
		batchID := calls[0].batch.BatchID
		assert.NotEmpty(t, batchID)

		totalItems := 0
		for i, call := range calls {
			require.NotNil(t, call.batch)
			assert.Equal(t, batchID, call.batch.BatchID, "all chunks should have same batch ID")
			assert.Equal(t, i, call.batch.BatchIndex)
			assert.Equal(t, len(calls), call.batch.BatchTotal)
			totalItems += call.chunkLen
		}
		assert.Equal(t, numItems, totalItems, "all items should be accounted for")
	})

	t.Run("chunk failure stops and returns error", func(t *testing.T) {
		t.Parallel()
		largeString := make([]byte, 900)
		for i := range largeString {
			largeString[i] = 'x'
		}
		numItems := (DefaultMaxPayloadBytes / 900) + 100
		items := make([]string, numItems)
		for i := range items {
			items[i] = string(largeString)
		}

		callCount := 0
		expectedErr := assert.AnError

		err := sendChunked(items, 0, 100, func(chunk []string, batch *BatchInfo) error {
			callCount++
			if callCount == 2 {
				return expectedErr
			}
			return nil
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
		assert.Equal(t, 2, callCount, "should stop after first failure")
	})
}

func TestSendChunked_UniformSizes(t *testing.T) {
	t.Parallel()
	items := []string{"aaa", "bbb", "ccc", "ddd", "eee"}
	// Each string uses five bytes; two items plus a comma consume eleven.
	const overhead = 12
	const budget = overhead + BatchFieldsOverheadBytes + 11
	// Force batching by using enough items to exceed the unbatched budget.
	for range 6 {
		items = append(items, items...)
	}
	var got []string
	var counts []int
	err := sendChunked(items, budget, overhead, func(chunk []string, batch *BatchInfo) error {
		require.NotNil(t, batch)
		assert.Equal(t, len(items)/2, batch.BatchTotal)
		counts = append(counts, len(chunk))
		got = append(got, chunk...)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, items, got)
	for _, count := range counts {
		assert.Equal(t, 2, count)
	}
}

func TestSendChunked_ExactFitAndEscaping(t *testing.T) {
	t.Parallel()
	type request struct {
		Name  string   `json:"name"`
		Items []string `json:"items"`
		*BatchInfo
	}
	items := []string{"<>&\"\n", "日本語"}
	envelope := request{Name: "<repo>", Items: items}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	overhead := metadataOverhead(request{Name: envelope.Name, Items: []string{}})
	calls := 0
	err = sendChunked(items, len(body), overhead, func(chunk []string, batch *BatchInfo) error {
		calls++
		assert.Nil(t, batch)
		assert.Equal(t, items, chunk)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestSendChunked_SerializationFailsBeforeSending(t *testing.T) {
	t.Parallel()
	calls := 0
	err := sendChunked([]any{"valid", make(chan int)}, 1, 0, func(_ []any, _ *BatchInfo) error {
		calls++
		return nil
	})
	require.ErrorIs(t, err, errUtils.ErrFailedToMarshalPayload)
	assert.Zero(t, calls)
}

func TestSendChunked_NilItems(t *testing.T) {
	t.Parallel()
	calls := 0
	err := sendChunked([]string(nil), 1, 0, func(chunk []string, batch *BatchInfo) error {
		calls++
		assert.Nil(t, chunk)
		assert.Nil(t, batch)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestSendChunked_OverheadExceedsBudget(t *testing.T) {
	t.Parallel()
	items := []string{"a", "b", "c"}
	var got []string
	err := sendChunked(items, 1, 100, func(chunk []string, batch *BatchInfo) error {
		require.Len(t, chunk, 1)
		require.NotNil(t, batch)
		assert.Equal(t, 3, batch.BatchTotal)
		got = append(got, chunk...)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, items, got)
}
