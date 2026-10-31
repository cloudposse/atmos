package oci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// Operation name used in capability errors.
const opLock = "lock"

const (
	lockDirMode = 0o700
	lockSubdir  = "artifacts/oci-locks"
)

// heldLocks tracks the host guards this process holds, keyed by lock file path. The OS lock
// excludes other processes; this table supplies the ownership-token check on release.
var (
	heldMu    sync.Mutex
	heldLocks = map[string]*hostLock{}
)

// hostLock is an OS file lock held until Release. It implements [artifact.LockHandle].
type hostLock struct {
	path     string
	resource string
	owner    string
	token    string
	release  chan struct{}
	done     chan error
	released bool
}

var (
	_ artifact.LockHandle     = (*hostLock)(nil)
	_ artifact.LockNamespacer = (*Repository)(nil)
)

// Lock implements [artifact.LockProvider].
//
// By default it is a host-scope guard: an OS file lock keyed by registry, repository, and tag in
// the user data directory (or the configured lock directory). That excludes other Atmos
// processes on this host only; a registry tag is not a distributed lease, so repository scope is
// never claimed here. When the repository configures `locking.repository`, every lock is taken
// from that delegate, which keeps its own identity and storage root and reports its own scope. A
// canceled wait leaks no ownership, and no lock is ever stolen.
func (r *Repository) Lock(ctx context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	defer perf.Track(nil, "oci.Repository.Lock")()

	if request.Resource == "" {
		return nil, fmt.Errorf("%w: a lock needs a resource", errUtils.ErrArtifactRefInvalid)
	}
	if request.Lease != 0 {
		return nil, artifact.NewUnsupportedCapability(r.alias, opLock, "lease", "OCI host guards do not expire, so a lease cannot be fenced")
	}
	if r.delegate != "" {
		return r.delegateLock(ctx, request)
	}
	if request.Required != "" && !artifact.LockScopeHost.Satisfies(request.Required) {
		return nil, artifact.NewUnsupportedCapability(r.alias, opLock, "lock/"+string(request.Required),
			"an OCI tag is not a distributed lease, so this repository provides host-scope locks only; set locking.repository to delegate to a repository that can provide repository scope")
	}
	owner := request.Owner
	if owner == "" {
		owner = r.settings.owner
	}
	return r.acquireHost(ctx, request.Resource, owner)
}

// delegateLock takes the lock from the configured delegate repository. The resource is scoped by
// this repository's location, so unrelated registries sharing one coordinator never contend.
func (r *Repository) delegateLock(ctx context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	if r.lookup == nil {
		return nil, fmt.Errorf("%w: repository %q delegates locking to %q but no repository lookup was supplied", errUtils.ErrArtifactConfigInvalid, r.alias, r.delegate)
	}
	if r.delegate == r.alias {
		return nil, fmt.Errorf("%w: repository %q cannot delegate locking to itself", errUtils.ErrArtifactConfigInvalid, r.alias)
	}
	delegate, err := r.lookup(r.delegate)
	if err != nil {
		return nil, fmt.Errorf("lock delegate %q of repository %q: %w", r.delegate, r.alias, err)
	}
	caps := delegate.Capabilities()
	required := request.Required
	if required == "" {
		required = artifact.LockScopeHost
	}
	if !caps.HasLockScope(required) {
		return nil, artifact.NewUnsupportedCapability(r.delegate, opLock, "lock/"+string(required),
			fmt.Sprintf("the lock delegate of repository %q cannot provide %s-scope coordination", r.alias, required))
	}
	request.Resource = r.lockNamespace() + "/" + request.Resource
	handle, err := delegate.Lock(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("lock delegate %q of repository %q: %w", r.delegate, r.alias, err)
	}
	return handle, nil
}

// LockNamespace implements [artifact.LockNamespacer].
func (r *Repository) LockNamespace() string {
	defer perf.Track(nil, "oci.Repository.LockNamespace")()

	return r.lockNamespace()
}

// lockNamespace identifies this repository's collection for lock keys.
func (r *Repository) lockNamespace() string {
	switch be := r.be.(type) {
	case *registryBackend:
		return be.registry + "/" + be.prefix
	case *layoutBackend:
		return filepath.ToSlash(be.root)
	default:
		return r.alias
	}
}

// lockPath returns the lock file of a resource. The key is a digest of the location and
// resource, so no reference text reaches the file system.
func (r *Repository) lockPath(resource string) (string, error) {
	dir := r.settings.lockDir
	if dir == "" {
		resolved, err := xdg.GetXDGDataDir(lockSubdir, lockDirMode)
		if err != nil {
			return "", fmt.Errorf("%w: lock directory: %w", errUtils.ErrArtifactConfigInvalid, err)
		}
		dir = resolved
	}
	if err := os.MkdirAll(dir, lockDirMode); err != nil {
		return "", fmt.Errorf("%w: create lock directory %q: %w", errUtils.ErrArtifactConfigInvalid, dir, err)
	}
	sum := sha256.Sum256([]byte(r.lockNamespace() + "\x00" + resource))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".lock"), nil
}

func (r *Repository) acquireHost(ctx context.Context, resource, owner string) (*hostLock, error) {
	path, err := r.lockPath(resource)
	if err != nil {
		return nil, err
	}
	held := make(chan struct{})
	lock := &hostLock{path: path, resource: resource, owner: owner, token: uuid.NewString(), release: make(chan struct{}), done: make(chan error, 1)}
	go func() {
		lock.done <- filelock.New(path).WithExclusive(ctx, func() error {
			close(held)
			<-lock.release
			return nil
		})
	}()
	select {
	case <-held:
		heldMu.Lock()
		heldLocks[path] = lock
		heldMu.Unlock()
		return lock, nil
	case err := <-lock.done:
		return nil, fmt.Errorf("waiting for lock %q in repository %q: %w", resource, r.alias, err)
	}
}

// Resource implements [artifact.LockHandle].
func (l *hostLock) Resource() string {
	defer perf.Track(nil, "oci.hostLock.Resource")()

	return l.resource
}

// Owner implements [artifact.LockHandle].
func (l *hostLock) Owner() string {
	defer perf.Track(nil, "oci.hostLock.Owner")()

	return l.owner
}

// Scope implements [artifact.LockHandle]; the OCI guard is host scope.
func (l *hostLock) Scope() artifact.LockScope {
	defer perf.Track(nil, "oci.hostLock.Scope")()

	return artifact.LockScopeHost
}

// Token implements [artifact.LockHandle].
func (l *hostLock) Token() string {
	defer perf.Track(nil, "oci.hostLock.Token")()

	return l.token
}

// ExpiresAt implements [artifact.LockHandle]; host guards never expire.
func (l *hostLock) ExpiresAt() (time.Time, bool) {
	defer perf.Track(nil, "oci.hostLock.ExpiresAt")()

	return time.Time{}, false
}

// Renew implements [artifact.LockHandle]; host guards have no leases.
func (l *hostLock) Renew(context.Context, time.Duration) error {
	defer perf.Track(nil, "oci.hostLock.Renew")()

	return artifact.NewUnsupportedCapability("", opLock, "lease", "OCI host guards do not expire")
}

// Release implements [artifact.LockHandle]. It checks the ownership token against the guard this
// process holds, so a stale or foreign handle can never release another operation's lock. The OS
// also releases the lock if its owner exits.
func (l *hostLock) Release(context.Context) error {
	defer perf.Track(nil, "oci.hostLock.Release")()

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
