package snapshot

import (
	"context"
	"io"
)

// copyChunkSize is the chunk size of context-aware copies.
const copyChunkSize = 32 * 1024

// copyContext copies src to dst in chunks and stops as soon as ctx is canceled, so large copies
// end promptly on cancellation.
func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, copyChunkSize)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			written, writeErr := dst.Write(buffer[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
