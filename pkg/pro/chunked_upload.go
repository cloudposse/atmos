package pro

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// DefaultMaxPayloadBytes leaves substantial headroom below Vercel's ~4.5 MB
	// request body limit, including request metadata and batch fields.
	DefaultMaxPayloadBytes = 3 * 1024 * 1024

	// BatchFieldsOverheadBytes conservatively bounds JSON batch metadata,
	// including a UUID and the decimal representation of two integer fields.
	BatchFieldsOverheadBytes = 200

	// DefaultMetadataOverheadBytes is the conservative fallback for metadata size estimation.
	DefaultMetadataOverheadBytes = 512

	// Bound whole-upload retries after a 413 response.
	maxPayloadReductions = 3
	// Prevent recovery from producing excessively small requests.
	minPayloadBytes = 64 * 1024
)

// BatchInfo holds metadata for chunked uploads.
type BatchInfo struct {
	BatchID    string `json:"batch_id"`
	BatchIndex int    `json:"batch_index"`
	BatchTotal int    `json:"batch_total"`
}

// chunkedUploadOption configures diagnostics without changing the upload format.
type chunkedUploadOption func(*chunkedUploadOptions)

type chunkedUploadOptions struct {
	describeItem func(int) string
}

func withItemDescription(describe func(int) string) chunkedUploadOption {
	return func(options *chunkedUploadOptions) { options.describeItem = describe }
}

type itemRange struct {
	start int
	end   int
}

type chunkedUpload[T any] struct {
	items         []T
	itemSizes     []int
	metadataBytes int
	options       chunkedUploadOptions
	sendFn        func([]T, *BatchInfo) error
}

// sendChunked greedily packs items using their serialized sizes. Metadata overhead
// includes an empty items array. Small uploads preserve the original slice and
// omit batch fields; larger uploads finalize all chunks before the first request.
// A 413 restarts the entire upload with a fresh batch ID and a smaller budget.
func sendChunked[T any](
	items []T,
	maxBytes int,
	estimateOverhead int,
	sendFn func(chunk []T, batch *BatchInfo) error,
	options ...chunkedUploadOption,
) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxPayloadBytes
	}
	if len(items) == 0 {
		return sendFn(items, nil)
	}

	upload := chunkedUpload[T]{items: items, metadataBytes: estimateOverhead, sendFn: sendFn}
	for _, option := range options {
		option(&upload.options)
	}
	// Measure each item once, before any request, and reuse sizes on recovery.
	upload.itemSizes = make([]int, len(items))
	for i, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			return wrapErr(errUtils.ErrFailedToMarshalPayload, err)
		}
		upload.itemSizes[i] = len(data)
	}
	return upload.sendWithRecovery(maxBytes)
}

func (u *chunkedUpload[T]) sendWithRecovery(budget int) error {
	for reduction := 0; ; reduction++ {
		failed, err := u.send(budget, reduction > 0)
		if err == nil {
			return nil
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusRequestEntityTooLarge {
			return err
		}
		if failed.end-failed.start == 1 {
			return u.singleItemError(failed.start, err)
		}
		if reduction == maxPayloadReductions || budget <= minPayloadBytes {
			return fmt.Errorf("upload still exceeds the Atmos Pro body size limit after %d budget reductions (budget=%d bytes): %w",
				reduction, budget, err)
		}
		nextBudget := max(budget/2, minPayloadBytes)
		log.Debug("Restarting upload after HTTP 413 with a smaller byte budget.",
			"old_budget_bytes", budget, "new_budget_bytes", nextBudget, "attempt", reduction+1)
		budget = nextBudget
	}
}

func (u *chunkedUpload[T]) singleItemError(index int, err error) error {
	name := fmt.Sprintf("item %d", index+1)
	if u.options.describeItem != nil {
		name = u.options.describeItem(index)
	}
	return errUtils.Build(errUtils.ErrPayloadTooLarge).
		WithCause(fmt.Errorf("%s (%d serialized bytes) was rejected as a single-item request: %w", name, u.itemSizes[index], err)).
		WithHint("Reduce large settings or other uploaded data for this stack/component; a single item cannot be split across requests.").
		Err()
}

func (u *chunkedUpload[T]) send(budget int, forceBatch bool) (itemRange, error) {
	// The metadata estimate already contains the array brackets; add item bytes
	// and the commas between them to get the exact unbatched request size.
	total := u.metadataBytes + len(u.items) - 1
	for _, size := range u.itemSizes {
		total += size
	}
	if !forceBatch && total <= budget {
		return itemRange{end: len(u.items)}, u.sendFn(u.items, nil)
	}
	chunks := packItemRanges(u.itemSizes, budget-u.metadataBytes-BatchFieldsOverheadBytes)
	return u.sendItemsInChunks(chunks)
}

// packItemRanges preserves order and allows an oversized item only in a singleton.
// The available budget excludes metadata, including the empty array brackets.
func packItemRanges(sizes []int, available int) []itemRange {
	var chunks []itemRange
	start, used := 0, 0
	for i, size := range sizes {
		additional := size
		if i > start {
			additional++ // Comma between items.
		}
		if i > start && used+additional > available {
			chunks = append(chunks, itemRange{start: start, end: i})
			start, used, additional = i, 0, size
		}
		used += additional
	}
	return append(chunks, itemRange{start: start, end: len(sizes)})
}

// sendItemsInChunks fixes the batch ID and total before sending any request.
func (u *chunkedUpload[T]) sendItemsInChunks(chunks []itemRange) (itemRange, error) {
	batchID := uuid.New().String()
	log.Debug("Splitting payload into chunks.", "batch_id", batchID, "chunk_count", len(chunks))
	for i, chunk := range chunks {
		batch := &BatchInfo{BatchID: batchID, BatchIndex: i, BatchTotal: len(chunks)}
		if err := u.sendFn(u.items[chunk.start:chunk.end], batch); err != nil {
			return chunk, fmt.Errorf("failed to send chunk %d/%d (batch_id=%s): %w", i+1, len(chunks), batchID, err)
		}
	}
	return itemRange{}, nil
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
