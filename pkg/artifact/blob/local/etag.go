package local

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// etagOf returns the content-based ETag of the file at path.
func etagOf(root *os.Root, path string) (string, error) {
	file, err := rootOpenFile(root, path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return etagOpen(file)
}

// etagOpen hashes the opened file and restores its position for the caller.
func etagOpen(file *os.File) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
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
