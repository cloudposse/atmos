package local

import (
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Default permissions for created directories and files. They match the legacy local CI store.
const (
	DefaultDirPerm  os.FileMode = 0o755
	DefaultFilePerm os.FileMode = 0o644
)

// Option configures a [Store].
type Option func(*Store)

// WithDirPerm sets the permission bits of created directories.
func WithDirPerm(perm os.FileMode) Option {
	defer perf.Track(nil, "blob.local.WithDirPerm")()

	return func(s *Store) { s.dirPerm = perm }
}

// WithFilePerm sets the permission bits of created files.
func WithFilePerm(perm os.FileMode) Option {
	defer perf.Track(nil, "blob.local.WithFilePerm")()

	return func(s *Store) { s.filePerm = perm }
}

// WithLockAllWrites serializes unconditional writes and deletes with conditional ones through the
// per-root lock. Enable it when one key is written both conditionally and unconditionally.
func WithLockAllWrites() Option {
	defer perf.Track(nil, "blob.local.WithLockAllWrites")()

	return func(s *Store) { s.lockAll = true }
}
