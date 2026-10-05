package lockfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PreparedRecord is a receipt built from staged content before any destination is mutated.
type PreparedRecord struct {
	id       string
	artifact Artifact
}

// Materialize copies content and records its receipt under the same mutation locks.
// Once acquired, the transaction finishes even if the caller cancels during copying.
func (r *PreparedRecord) Materialize(ctx context.Context, config *schema.AtmosConfiguration, copyFiles func() error) error {
	defer perf.Track(config, "lockfile.PreparedRecord.Materialize")()
	return WithMutation(ctx, config, func() error {
		// Reject a receipt changed or corrupted by another process before touching targets.
		previous, err := Load(config)
		if err != nil {
			return err
		}
		snapshot, err := snapshotTargets(config, previous, r)
		if err != nil {
			return err
		}
		err = copyFiles()
		if err == nil {
			err = replaceUnlocked(config, r.id, r.artifact)
		}
		if err != nil {
			if restoreErr := snapshot.restore(); restoreErr != nil {
				// Preserve the only recovery copy when the filesystem also rejects rollback.
				return errors.Join(err, fmt.Errorf("restore vendor target (backup retained at %s): %w", snapshot.dir, restoreErr))
			}
		}
		return errors.Join(err, snapshot.close())
	})
}

// WithMutation serializes project writes and lockfile read-modify-write transactions,
// including when different configurations use the same committed lockfile.
// Callbacks must use unlocked helpers and must not acquire these locks again.
func WithMutation(ctx context.Context, config *schema.AtmosConfiguration, fn func() error) error {
	defer perf.Track(config, "lockfile.WithMutation")()
	if err := ctx.Err(); err != nil {
		return err
	}
	base, err := projectBase(config)
	if err != nil {
		return err
	}
	base, err = canonicalPath(base)
	if err != nil {
		return err
	}
	lockPath, err := canonicalPath(Path(config))
	if err != nil {
		return err
	}
	projectLock := filepath.Join(base, ".atmos", "vendor-mutation.lock")
	if err := os.MkdirAll(filepath.Dir(projectLock), directoryMode); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), directoryMode); err != nil {
		return err
	}
	return filelock.New(projectLock).WithExclusive(ctx, func() error {
		return filelock.New(lockPath+".lock").WithExclusive(ctx, fn)
	})
}

// canonicalPath resolves symlinks in the existing prefix, including before a lockfile exists.
func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return "", err
	}
	resolved, err = canonicalPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(abs)), nil
}
