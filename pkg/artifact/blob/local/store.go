package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName = "local"

	// Name prefix of in-flight temporary files, which listings never show.
	tempPrefix = ".atmos-blob-tmp-"

	// Name suffix of the optional attribute file stored beside an object.
	attrsSuffix = ".atmos-blob.json"

	// Name of the per-root lock file. Only conditional operations create it.
	lockName = ".atmos-blob.lock"
)

// Store is a filesystem [blob.Store].
type Store struct {
	root     string
	dirPerm  os.FileMode
	filePerm os.FileMode
	lockAll  bool
}

// New returns a Store rooted at root. The directory is created on first write.
func New(root string, opts ...Option) (*Store, error) {
	defer perf.Track(nil, "blob.local.New")()

	if root == "" {
		return nil, blob.NewError(blob.KindInvalid, "new", "", fmt.Errorf("%w: root directory is required", errUtils.ErrBlobInvalid))
	}
	store := &Store{root: filepath.Clean(root), dirPerm: DefaultDirPerm, filePerm: DefaultFilePerm}
	for _, opt := range opts {
		opt(store)
	}
	return store, nil
}

// Name implements [blob.Store].
func (s *Store) Name() string {
	defer perf.Track(nil, "local.Store.Name")()

	return storeName
}

// Root returns the root directory.
func (s *Store) Root() string {
	defer perf.Track(nil, "local.Store.Root")()

	return s.root
}

// Capabilities implements [blob.Store].
func (s *Store) Capabilities() blob.Capabilities {
	defer perf.Track(nil, "local.Store.Capabilities")()

	return blob.Capabilities{
		ConditionalPut:    true,
		ConditionalDelete: true,
		Multipart:         true,
		Tags:              true,
		Metadata:          true,
		Range:             true,
		Listing:           true,
		ListDelimiter:     true,
	}
}

// validate rejects keys that are unsafe on a filesystem or collide with the store's own files.
func (s *Store) validate(op, key string) error {
	if err := blob.ValidateKey(key); err != nil {
		return err
	}
	last := key[strings.LastIndexByte(key, '/')+1:]
	if strings.HasPrefix(last, tempPrefix) || strings.HasSuffix(last, attrsSuffix) || key == lockName {
		return blob.NewError(blob.KindInvalid, op, key, fmt.Errorf("%w: key uses a reserved name", errUtils.ErrBlobInvalid))
	}
	return nil
}

// path maps a validated key to its file path.
func (s *Store) path(key string) string {
	return filepath.Join(s.root, filepath.FromSlash(key))
}

// checkContext fails fast when ctx is already done.
func checkContext(ctx context.Context, op, key string) error {
	if err := ctx.Err(); err != nil {
		return blob.NewError(blob.KindOther, op, key, err)
	}
	return nil
}

var _ blob.Store = (*Store)(nil)
