package pro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitSlice(t *testing.T) {
	t.Run("splits evenly", func(t *testing.T) {
		items := []int{1, 2, 3, 4, 5, 6}
		chunks := splitSlice(items, 2)
		require.Len(t, chunks, 3)
		assert.Equal(t, []int{1, 2}, chunks[0])
		assert.Equal(t, []int{3, 4}, chunks[1])
		assert.Equal(t, []int{5, 6}, chunks[2])
	})

	t.Run("splits with remainder", func(t *testing.T) {
		items := []int{1, 2, 3, 4, 5}
		chunks := splitSlice(items, 2)
		require.Len(t, chunks, 3)
		assert.Equal(t, []int{1, 2}, chunks[0])
		assert.Equal(t, []int{3, 4}, chunks[1])
		assert.Equal(t, []int{5}, chunks[2])
	})

	t.Run("single chunk when items fit", func(t *testing.T) {
		items := []int{1, 2, 3}
		chunks := splitSlice(items, 10)
		require.Len(t, chunks, 1)
		assert.Equal(t, []int{1, 2, 3}, chunks[0])
	})

	t.Run("chunk size of 1", func(t *testing.T) {
		items := []int{1, 2, 3}
		chunks := splitSlice(items, 1)
		require.Len(t, chunks, 3)
		assert.Equal(t, []int{1}, chunks[0])
		assert.Equal(t, []int{2}, chunks[1])
		assert.Equal(t, []int{3}, chunks[2])
	})

	t.Run("zero chunk size defaults to 1", func(t *testing.T) {
		items := []int{1, 2}
		chunks := splitSlice(items, 0)
		require.Len(t, chunks, 2)
	})

	t.Run("empty slice", func(t *testing.T) {
		chunks := splitSlice([]int{}, 5)
		assert.Empty(t, chunks)
	})
}

func TestMetadataOverhead(t *testing.T) {
	type testStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	overhead := metadataOverhead(testStruct{Name: "test", Value: 42})
	expected, _ := json.Marshal(testStruct{Name: "test", Value: 42})
	assert.Equal(t, len(expected), overhead)
}

func TestSendChunked(t *testing.T) {
	t.Run("small payload sends without batch info", func(t *testing.T) {
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
		// Create items that will exceed DefaultMaxPayloadBytes.
		// Each item is ~1000 bytes when serialized.
		largeString := make([]byte, 900)
		for i := range largeString {
			largeString[i] = 'x'
		}
		item := string(largeString)

		// Create enough items to exceed 4MB.
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
