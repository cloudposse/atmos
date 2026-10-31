package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// wrapIO classifies an OS error: a missing path is NotFound, everything else is a plain failure.
func wrapIO(op, key string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return blob.NewError(blob.KindNotFound, op, key, err)
	}
	return blob.NewError(blob.KindOther, op, key, err)
}

func preconditionFailed(op, key, reason string) error {
	return blob.NewError(blob.KindPrecondition, op, key, fmt.Errorf("%w: %s", errUtils.ErrBlobPreconditionFailed, reason))
}

// classifyCopy maps a body copy failure: size mismatches are invalid requests, the rest are I/O.
func classifyCopy(key string, err error) error {
	if errors.Is(err, errUtils.ErrBlobInvalid) {
		return blob.NewError(blob.KindInvalid, blob.OpPut, key, err)
	}
	return blob.NewError(blob.KindOther, blob.OpPut, key, err)
}

// classifyCreate explains why a temporary file could not be created beside target. An existing
// object occupying a parent path is a key conflict, which the filesystem cannot represent.
func (s *Store) classifyCreate(key, target string, err error) error {
	for dir := filepath.Dir(target); strings.HasPrefix(dir, s.root) && dir != s.root; dir = filepath.Dir(dir) {
		if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
			return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: a parent path is an existing object", errUtils.ErrBlobInvalid))
		}
	}
	return wrapIO(blob.OpPut, key, err)
}

func firstError(errs ...error) error {
	return errors.Join(errs...)
}

// ctxReader fails reads once its context is done so a canceled put stops promptly.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "local.ctxReader.Read")()

	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
