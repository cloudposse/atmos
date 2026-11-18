package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// etagOf returns the content-based ETag of the file at path. Hashing stops when ctx is done.
func etagOf(ctx context.Context, root *os.Root, path string) (string, error) {
	file, err := rootOpenFile(root, path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return etagOpen(ctx, file)
}

// etagOpen hashes the opened file and restores its position for the caller. Cancellation is
// observed between reads, so a canceled caller releases the root lock instead of finishing the scan.
func etagOpen(ctx context.Context, file *os.File) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, &ctxReader{ctx: ctx, r: file}); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return contentETag(hash.Sum(nil)), nil
}

// contentETag formats a content hash as an ETag.
func contentETag(sum []byte) string {
	return `"` + hex.EncodeToString(sum) + `"`
}
