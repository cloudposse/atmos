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
	if err := s.Capabilities().CheckDelete(storeName, key, opts); err != nil {
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
	return s.deleteFromRoot(ctx, root, key, opts.IfMatch)
}

// deleteFromRoot removes the object and its attributes after an optional ETag check, then prunes
// directories the removal left empty. A key that is only a prefix of other objects names no object,
// so deleting it succeeds and leaves the directory and its children untouched.
func (s *Store) deleteFromRoot(ctx context.Context, root *os.Root, key, ifMatch string) error {
	target := filepath.FromSlash(key)
	if ifMatch != "" {
		if err := checkDeleteETag(ctx, root, key, target, ifMatch); err != nil {
			return err
		}
	}
	if info, err := root.Stat(target); err == nil && info.IsDir() {
		return nil
	}
	// Plain-file stores keep no attribute files, and a name that looks like one is another object.
	paths := []string{target}
	if !s.plainFiles {
		paths = append(paths, attrsPath(target))
	}
	for _, path := range paths {
		if err := root.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return wrapIO(blob.OpDelete, key, err)
		}
	}
	rootRemoveEmptyDirs(root, filepath.Dir(target))
	return nil
}

// checkDeleteETag verifies the if-match token of a conditional delete. Callers hold the lock.
func checkDeleteETag(ctx context.Context, root *os.Root, key, target, want string) error {
	state, err := inspect(root, target)
	if err != nil {
		return wrapIO(blob.OpDelete, key, err)
	}
	return matchETag(ctx, root, objectRef{op: blob.OpDelete, key: key}, state, want)
}
