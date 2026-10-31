package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Internal lock resources. They start with a NUL byte, which no artifact reference can contain,
// so they can never collide with a caller's reference lock.
const (
	claimsResource   = "\x00claims"
	locationResource = "\x00location\x00"
)

// heldLocks tracks the locks this process holds, keyed by lock file path. The OS lock excludes
// other processes; this table supplies the ownership-token check on release and diagnostics.
var (
	heldMu    sync.Mutex
	heldLocks = map[string]*fileLock{}
)

// fileLock is an OS file lock held until Release. It implements [artifact.LockHandle].
type fileLock struct {
	path        string
	resource    string
	owner       string
	operationID string
	token       string
	release     chan struct{}
	done        chan error
	released    bool
}

var _ artifact.LockHandle = (*fileLock)(nil)

// Lock implements [artifact.LockProvider] with an OS-backed file lock of host scope. It waits for
// a competing owner until ctx ends; a canceled wait leaks no ownership. Leases are rejected
// because a paused writer could not be fenced, and repository scope is rejected because a local
// file lock cannot exclude writers on other hosts.
func (r *Repository) Lock(ctx context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	defer perf.Track(nil, "local.Repository.Lock")()

	if request.Lease != 0 {
		return nil, artifact.NewUnsupportedCapability(r.alias, "lock", "lease", "local file locks do not expire, so a lease cannot be fenced")
	}
	if request.Required != "" && !artifact.LockScopeHost.Satisfies(request.Required) {
		return nil, artifact.NewUnsupportedCapability(r.alias, "lock", "lock/"+string(request.Required),
			"local repositories provide host-scope locks only; use an S3 repository or a lock delegate for repository scope")
	}
	if request.Resource == "" {
		return nil, fmt.Errorf("%w: a lock needs a resource", errUtils.ErrArtifactRefInvalid)
	}
	owner := request.Owner
	if owner == "" {
		owner = r.settings.owner
	}
	return r.acquire(ctx, request.Resource, owner, request.OperationID)
}

// acquire takes the OS lock for resource.
func (r *Repository) acquire(ctx context.Context, resource, owner, operationID string) (*fileLock, error) {
	lockPath := filepath.Join(r.root, native(layout.LockPath(resource)))
	if err := os.MkdirAll(filepath.Dir(lockPath), dirMode); err != nil {
		return nil, fmt.Errorf("%w: create lock directory for %q: %w", errUtils.ErrArtifactConfigInvalid, resource, err)
	}
	held := make(chan struct{})
	lock := &fileLock{
		path: lockPath, resource: resource, owner: owner, operationID: operationID,
		token: uuid.NewString(), release: make(chan struct{}), done: make(chan error, 1),
	}
	go func() {
		lock.done <- filelock.New(lockPath).WithExclusive(ctx, func() error {
			close(held)
			<-lock.release
			return nil
		})
	}()
	select {
	case <-held:
		heldMu.Lock()
		heldLocks[lockPath] = lock
		heldMu.Unlock()
		return lock, nil
	case err := <-lock.done:
		return nil, fmt.Errorf("waiting for lock %q in repository %q: %w", resource, r.alias, err)
	}
}

// Resource implements [artifact.LockHandle].
func (l *fileLock) Resource() string {
	defer perf.Track(nil, "local.fileLock.Resource")()

	return l.resource
}

// Owner implements [artifact.LockHandle].
func (l *fileLock) Owner() string {
	defer perf.Track(nil, "local.fileLock.Owner")()

	return l.owner
}

// Scope implements [artifact.LockHandle]; local locks are host scope.
func (l *fileLock) Scope() artifact.LockScope {
	defer perf.Track(nil, "local.fileLock.Scope")()

	return artifact.LockScopeHost
}

// Token implements [artifact.LockHandle].
func (l *fileLock) Token() string {
	defer perf.Track(nil, "local.fileLock.Token")()

	return l.token
}

// ExpiresAt implements [artifact.LockHandle]; local locks never expire.
func (l *fileLock) ExpiresAt() (time.Time, bool) {
	defer perf.Track(nil, "local.fileLock.ExpiresAt")()

	return time.Time{}, false
}

// Renew implements [artifact.LockHandle]; local locks have no leases.
func (l *fileLock) Renew(context.Context, time.Duration) error {
	defer perf.Track(nil, "local.fileLock.Renew")()

	return artifact.NewUnsupportedCapability("", "lock", "lease", "local file locks do not expire")
}

// Release implements [artifact.LockHandle]. It checks the ownership token against the lock this
// process holds, so a stale or foreign handle can never release another operation's lock. The OS
// also releases the lock if its owner exits.
func (l *fileLock) Release(context.Context) error {
	defer perf.Track(nil, "local.fileLock.Release")()

	heldMu.Lock()
	current, owned := heldLocks[l.path]
	if !owned || current != l || l.released || current.token != l.token {
		heldMu.Unlock()
		return fmt.Errorf("%w: lock %q is not held by token %s", errUtils.ErrArtifactConflict, l.resource, l.token)
	}
	delete(heldLocks, l.path)
	l.released = true
	heldMu.Unlock()
	close(l.release)
	if err := <-l.done; err != nil {
		return fmt.Errorf("release lock %q: %w", l.resource, err)
	}
	return nil
}

// releaseQuietly releases a lock acquired internally and returns the first error alongside err.
func releaseAll(ctx context.Context, locks ...*fileLock) error {
	var first error
	for index := len(locks) - 1; index >= 0; index-- {
		if locks[index] == nil {
			continue
		}
		if err := locks[index].Release(context.WithoutCancel(ctx)); err != nil && first == nil {
			first = err
		}
	}
	return first
}
