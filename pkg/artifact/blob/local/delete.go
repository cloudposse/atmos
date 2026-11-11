package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Delete implements [blob.Store].
func (s *Store) Delete(ctx context.Context, key string, opts blob.DeleteOptions) error {
	defer perf.Track(nil, "blob.local.Delete")()

	if err := s.validate(blob.OpDelete, key); err != nil {
		return err
	}
	if err := checkContext(ctx, blob.OpDelete, key); err != nil {
		return err
	}
	release, err := s.guard(ctx, blob.OpDelete, key, opts.IfMatch != "")
	if err != nil {
		return err
	}
	defer release()

	root, err := s.openRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return wrapIO(blob.OpDelete, key, err)
	}
	defer root.Close()
	target := filepath.FromSlash(key)
	if opts.IfMatch != "" {
		if err := checkDeleteETag(root, key, target, opts.IfMatch); err != nil {
			return err
		}
	}
	for _, path := range []string{target, attrsPath(target)} {
		if err := root.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return wrapIO(blob.OpDelete, key, err)
		}
	}
	rootRemoveEmptyDirs(root, filepath.Dir(target))
	return nil
}

// checkDeleteETag verifies the if-match token of a conditional delete. Callers hold the lock.
func checkDeleteETag(root *os.Root, key, target, want string) error {
	state, err := inspect(root, target)
	if err != nil {
		return wrapIO(blob.OpDelete, key, err)
	}
	return matchETag(root, blob.OpDelete, key, state, want)
}
