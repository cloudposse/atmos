package pro

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// DefaultMaxPayloadBytes is the maximum payload size before chunking is triggered.
	// Set below Vercel's ~4.5MB serverless function body limit.
	DefaultMaxPayloadBytes = 4 * 1024 * 1024

	// BatchFieldsOverheadBytes is the estimated JSON byte overhead for batch metadata fields.
	BatchFieldsOverheadBytes = 200

	// DefaultMetadataOverheadBytes is the conservative fallback for metadata size estimation.
	DefaultMetadataOverheadBytes = 512
)

// BatchInfo holds metadata for chunked uploads.
type BatchInfo struct {
	BatchID    string `json:"batch_id"`
	BatchIndex int    `json:"batch_index"`
	BatchTotal int    `json:"batch_total"`
}

// sendChunked splits items into chunks that fit under the max payload size and
// sends each chunk via sendFn. If the full payload fits in a single request,
// sendFn is called once with batch=nil (no batch fields). Otherwise, each chunk
// includes BatchInfo for server-side reassembly. The maxBytes parameter controls
// the threshold (0 uses DefaultMaxPayloadBytes). The estimateOverhead parameter
// is the JSON byte size of the request metadata (everything except the items
// array).
func sendChunked[T any](
	items []T,
	maxBytes int,
	estimateOverhead int,
	sendFn func(chunk []T, batch *BatchInfo) error,
) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxPayloadBytes
	}

	if len(items) == 0 {
		return sendFn(items, nil)
	}

	// Estimate total payload size by marshaling all items.
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to estimate payload size: %w", err)
	}

	totalSize := estimateOverhead + len(itemsJSON)
	if totalSize <= maxBytes {
		return sendFn(items, nil)
	}

	return sendItemsInChunks(items, itemsJSON, maxBytes, estimateOverhead, sendFn)
}

// sendItemsInChunks handles the actual chunking and sequential sending.
func sendItemsInChunks[T any](
	items []T,
	itemsJSON []byte,
	maxBytes int,
	estimateOverhead int,
	sendFn func(chunk []T, batch *BatchInfo) error,
) error {
	// Calculate chunk size based on average item size.
	avgItemSize := len(itemsJSON) / len(items)
	if avgItemSize == 0 {
		avgItemSize = 1
	}
	availableForItems := maxBytes - estimateOverhead - BatchFieldsOverheadBytes
	if availableForItems <= 0 {
		availableForItems = maxBytes / 2
	}
	chunkSize := availableForItems / avgItemSize
	if chunkSize < 1 {
		chunkSize = 1
	}

	chunks := splitSlice(items, chunkSize)
	batchID := uuid.New().String()

	log.Debug("Splitting payload into chunks.",
		"batch_id", batchID,
		"chunk_count", len(chunks),
		"chunk_size", chunkSize,
	)

	for i, chunk := range chunks {
		batch := &BatchInfo{
			BatchID:    batchID,
			BatchIndex: i,
			BatchTotal: len(chunks),
		}
		if err := sendFn(chunk, batch); err != nil {
			return fmt.Errorf("failed to send chunk %d/%d (batch_id=%s): %w",
				i+1, len(chunks), batchID, err)
		}
	}

	return nil
}

// splitSlice divides a slice into chunks of at most chunkSize elements.
func splitSlice[T any](items []T, chunkSize int) [][]T {
	if chunkSize <= 0 {
		chunkSize = 1
	}
	var chunks [][]T
	for i := 0; i < len(items); i += chunkSize {
		end := i + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

// metadataOverhead returns the JSON byte size of a struct with the items field
// set to an empty array. This is used to estimate the overhead for chunking.
func metadataOverhead(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return DefaultMetadataOverheadBytes
	}
	return len(data)
}
