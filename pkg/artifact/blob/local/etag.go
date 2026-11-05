package local

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
)

// contentETagLimit is the largest object whose ETag is its SHA-256. Larger objects get a weak
// size/mtime token so a head request never reads gigabytes.
const contentETagLimit = 4 << 20

// hexBase formats sizes and times inside weak ETags.
const hexBase = 16

// etagOf returns the ETag of the file at path described by info.
func etagOf(path string, info os.FileInfo) (string, error) {
	if info.Size() > contentETagLimit {
		return weakETag(info), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return contentETag(hash.Sum(nil)), nil
}

// contentETag formats a content hash as an ETag.
func contentETag(sum []byte) string {
	return `"` + hex.EncodeToString(sum) + `"`
}

// weakETag derives a token from size and modification time for objects over the content limit.
func weakETag(info os.FileInfo) string {
	return fmt.Sprintf(`W/"%s-%s"`, strconv.FormatInt(info.Size(), hexBase), strconv.FormatInt(info.ModTime().UnixNano(), hexBase))
}
