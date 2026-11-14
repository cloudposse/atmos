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

// WithLockAllWrites serializes unconditional writes and deletes with conditional ones. Managed
// stores already do this. A plain-file store enforces if-absent and if-match conditions only with
// this option, because only then does every mutation honor the lock that makes a condition check
// and its commit atomic; without it the plain-file store refuses conditional operations.
func WithLockAllWrites() Option {
	defer perf.Track(nil, "blob.local.WithLockAllWrites")()

	return func(s *Store) { s.lockAll = true }
}

// WithPlainFileCompatibility preserves the legacy CI layout and unlocked unconditional operations.
// Because unconditional operations stay unlocked, the store refuses conditional puts and deletes
// and does not advertise them unless [WithLockAllWrites] is also set. Do not mix this mode with managed stores on the same root when consistency is required.
func WithPlainFileCompatibility() Option {
	defer perf.Track(nil, "local.WithPlainFileCompatibility")()

	return func(s *Store) { s.plainFiles = true }
}

// WithLiteralBackslashes permits literal backslashes on filesystems where they are not separators.
// This supports legacy local artifact names; other key validation remains in effect.
func WithLiteralBackslashes() Option {
	defer perf.Track(nil, "blob.local.WithLiteralBackslashes")()

	return func(s *Store) { s.allowBackslash = true }
}
