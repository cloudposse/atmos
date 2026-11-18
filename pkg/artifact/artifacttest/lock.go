package artifacttest

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// lockState is one claimed resource.
type lockState struct {
	owner       string
	operationID string
	token       string
	released    chan struct{}
}

// handle is an ownership-checked lock handle.
type handle struct {
	repo     *Repository
	resource string
	owner    string
	token    string
	scope    artifact.LockScope
}

var _ artifact.LockHandle = (*handle)(nil)

// Lock implements [artifact.LockProvider]. Acquisition waits for a competing owner and returns the
// context error, leaking nothing, when canceled. Locks never expire and are never stolen.
func (r *Repository) Lock(ctx context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	defer perf.Track(nil, "artifacttest.Repository.Lock")()

	r.mu.Lock()
	r.count("Lock")
	r.mu.Unlock()
	if request.Lease != 0 {
		return nil, artifact.NewUnsupportedCapability(r.opts.Alias, "lock", "lease", "claims never expire, so a lease cannot be fenced")
	}
	required := request.Required
	if required == "" {
		required = artifact.LockScopeHost
	}
	if !r.caps.HasLockScope(required) {
		return nil, artifact.NewUnsupportedCapability(r.opts.Alias, "lock", "lock/"+string(required), "the repository cannot provide this guarantee")
	}
	for {
		r.mu.Lock()
		held, busy := r.locks[request.Resource]
		if !busy {
			token := uuid.NewString()
			r.locks[request.Resource] = &lockState{owner: request.Owner, operationID: request.OperationID, token: token, released: make(chan struct{})}
			r.mu.Unlock()
			return &handle{repo: r, resource: request.Resource, owner: request.Owner, token: token, scope: r.bestScope()}, nil
		}
		wait := held.released
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for lock %s held by %s: %w", request.Resource, held.owner, ctx.Err())
		case <-wait:
		}
	}
}

// bestScope is the strongest scope the repository advertises.
func (r *Repository) bestScope() artifact.LockScope {
	if r.caps.HasLockScope(artifact.LockScopeRepository) {
		return artifact.LockScopeRepository
	}
	return artifact.LockScopeHost
}

// HeldLocks lists the currently claimed resources, sorted.
func (r *Repository) HeldLocks() []string {
	defer perf.Track(nil, "artifacttest.Repository.HeldLocks")()

	r.mu.Lock()
	defer r.mu.Unlock()
	resources := make([]string, 0, len(r.locks))
	for resource := range r.locks {
		resources = append(resources, resource)
	}
	sort.Strings(resources)
	return resources
}

// Resource implements [artifact.LockHandle].
func (h *handle) Resource() string {
	defer perf.Track(nil, "artifacttest.handle.Resource")()

	return h.resource
}

// Owner implements [artifact.LockHandle].
func (h *handle) Owner() string {
	defer perf.Track(nil, "artifacttest.handle.Owner")()

	return h.owner
}

// Scope implements [artifact.LockHandle].
func (h *handle) Scope() artifact.LockScope {
	defer perf.Track(nil, "artifacttest.handle.Scope")()

	return h.scope
}

// Token implements [artifact.LockHandle].
func (h *handle) Token() string {
	defer perf.Track(nil, "artifacttest.handle.Token")()

	return h.token
}

// ExpiresAt implements [artifact.LockHandle]; claims never expire.
func (h *handle) ExpiresAt() (time.Time, bool) {
	defer perf.Track(nil, "artifacttest.handle.ExpiresAt")()

	return time.Time{}, false
}

// Renew implements [artifact.LockHandle]; there are no leases.
func (h *handle) Renew(context.Context, time.Duration) error {
	defer perf.Track(nil, "artifacttest.handle.Renew")()

	return artifact.NewUnsupportedCapability(h.repo.opts.Alias, "lock", "lease", "claims never expire")
}

// Release implements [artifact.LockHandle]. Releasing checks the ownership token.
func (h *handle) Release(context.Context) error {
	defer perf.Track(nil, "artifacttest.handle.Release")()

	h.repo.mu.Lock()
	defer h.repo.mu.Unlock()
	h.repo.count("Release")
	held, ok := h.repo.locks[h.resource]
	if !ok || held.token != h.token {
		return fmt.Errorf("%w: lock %s is not owned by token %s", errUtils.ErrArtifactConflict, h.resource, h.token)
	}
	delete(h.repo.locks, h.resource)
	close(held.released)
	return nil
}
