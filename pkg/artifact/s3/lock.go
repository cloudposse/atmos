package s3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// opLock names the lock operation in capability errors.
const opLock = "lock"

const reconciliationOwnerPrefix = "pending reconciliation (do not delete until original remote request is known to have finished): "

// maxClaimBytes bounds a claim object read.
const maxClaimBytes int64 = 64 << 10

// claimDoc is the content of a lock claim object. It names the resource, owner, and operation so a
// recovery diagnostic can tell an operator whose claim is blocking.
type claimDoc struct {
	SchemaVersion int    `json:"schema_version"`
	Resource      string `json:"resource"`
	Owner         string `json:"owner"`
	OperationID   string `json:"operation_id,omitempty"`
	Token         string `json:"token"`
	AcquiredAt    string `json:"acquired_at"`
}

// claimHandle is an acquired lock claim. It implements [artifact.LockHandle].
type claimHandle struct {
	repo        *Repository
	key         string
	resource    string
	owner       string
	operationID string
	token       string

	mu       sync.Mutex
	released bool
}

var _ artifact.LockHandle = (*claimHandle)(nil)

// Lock implements [artifact.LockProvider] with repository-scope owner claims. A claim object is
// created with If-None-Match; a competing owner waits until ctx ends or the configured wait elapses
// and then receives a conflict that names the claim. Claims never expire and are never taken over,
// so leases are rejected, and an endpoint that cannot enforce If-None-Match offers no lock at all.
// A canceled acquisition leaks no ownership.
func (r *Repository) Lock(ctx context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	defer perf.Track(nil, "s3.Repository.Lock")()

	if request.Lease != 0 {
		return nil, artifact.NewUnsupportedCapability(r.alias, opLock, "lease", "S3 claims do not expire automatically, so a lease cannot be fenced")
	}
	if request.Required != "" && !artifact.LockScopeRepository.Satisfies(request.Required) {
		return nil, artifact.NewUnsupportedCapability(r.alias, opLock, "lock/"+string(request.Required), "S3 repositories provide host and repository scope locks")
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

// LockNamespace implements [artifact.LockNamespacer]: the bucket and prefix identify the collection.
func (r *Repository) LockNamespace() string {
	defer perf.Track(nil, "s3.Repository.LockNamespace")()

	if r.endpoint != "" {
		return "s3://" + r.endpoint + "/" + r.bucket + "/" + r.prefix
	}
	return "s3://" + r.bucket + "/" + r.prefix
}

// acquire takes the claim for resource, waiting for a competing owner.
func (r *Repository) acquire(ctx context.Context, resource, owner, operationID string) (*claimHandle, error) {
	if !r.store.Capabilities().ConditionalPut || !r.store.Capabilities().ConditionalDelete {
		return nil, artifact.NewUnsupportedCapability(r.alias, opLock, "lock/repository", "the S3 endpoint cannot enforce conditional writes and deletes, which owner claims require")
	}
	doc := claimDoc{
		SchemaVersion: layout.SchemaVersion, Resource: resource, Owner: owner, OperationID: operationID,
		Token: uuid.NewString(), AcquiredAt: time.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("%w: encode lock claim: %w", errUtils.ErrArtifactUploadFailed, err)
	}
	key := layout.LockPath(resource)
	deadline := time.Now().Add(r.settings.lockWait)
	for {
		handle, err := r.tryClaim(ctx, key, &doc, raw)
		if handle != nil || err != nil {
			return handle, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, r.heldError(ctx, key, resource, ctxErr)
		}
		if !time.Now().Before(deadline) {
			return nil, r.heldError(ctx, key, resource, nil)
		}
		if err := sleep(ctx, r.settings.lockPoll); err != nil {
			return nil, r.heldError(ctx, key, resource, err)
		}
	}
}

// sleep waits for d or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// tryClaim makes one attempt to create the claim. It returns the handle on success, (nil, nil)
// when another owner holds the claim, and an error for anything else. A response lost after the
// request was sent is settled by reading the claim back.
func (r *Repository) tryClaim(ctx context.Context, key string, doc *claimDoc, raw []byte) (*claimHandle, error) {
	_, err := r.putConditional(ctx, key, raw, contentTypeJSON, blob.IfAbsent())
	switch {
	case err == nil:
		return r.adopt(ctx, key, doc)
	case blob.IsPreconditionFailed(err):
		return nil, nil
	case blob.IsAmbiguous(err) || ctx.Err() != nil:
		// A request cut short by the context may still have created the claim, so read it back.
		return r.settleClaim(ctx, key, doc)
	case errors.Is(err, errUtils.ErrArtifactUnsupportedCapability):
		unsupported := artifact.NewUnsupportedCapability(r.alias, opLock, "lock/repository", "the S3 endpoint cannot enforce If-None-Match, which owner claims require")
		unsupported.Cause = err
		return nil, unsupported
	default:
		return nil, err
	}
}

// settleClaim reads the claim back after an ambiguous create. Finding our own token proves the
// create was applied.
func (r *Repository) settleClaim(ctx context.Context, key string, doc *claimDoc) (*claimHandle, error) {
	readCtx, cancel := detached(ctx)
	defer cancel()
	current, _, err := r.readClaim(readCtx, key)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%w: settle %s: %w", errUtils.ErrArtifactPublicationAmbiguous, r.attempt(key, doc), err)
	case current != nil && current.Token == doc.Token:
		return r.adopt(ctx, key, doc)
	case current == nil:
		handle, fenceErr := r.fenceUnknownClaim(ctx, readCtx, key, doc)
		if handle != nil {
			return handle, nil
		}
		return nil, errors.Join(fenceErr, ctx.Err())
	}
	return nil, errors.Join(ctx.Err(), errUtils.ErrArtifactConflict, fmt.Errorf("%w: %s found the claim held by another owner, but the prior create may still commit after it is released", errUtils.ErrArtifactPublicationAmbiguous, r.attempt(key, doc)))
}

// fenceUnknownClaim leaves a visible claim at an empty key while the first PUT may still arrive.
// The fence is intentionally not released: a finite wait or an empty read cannot prove that the
// first request has stopped executing. Legacy writers understand the existing claim format.
func (r *Repository) fenceUnknownClaim(originalCtx, ctx context.Context, key string, original *claimDoc) (*claimHandle, error) {
	fence := *original
	fence.Token = uuid.NewString()
	fence.Owner = reconciliationOwnerPrefix + original.Owner
	raw, err := json.Marshal(&fence)
	if err != nil {
		return nil, fmt.Errorf("%w: encode claim fence: %w", errUtils.ErrArtifactPublicationAmbiguous, err)
	}
	_, err = r.putConditional(ctx, key, raw, contentTypeJSON, blob.IfAbsent())
	if err == nil {
		return nil, fmt.Errorf("%w: %s is fenced (fence lock ID %s) pending reconciliation of a possibly late create; do not delete the fence until the original request is known to have finished", errUtils.ErrArtifactPublicationAmbiguous, r.attempt(key, original), fence.Token)
	}
	if blob.IsPreconditionFailed(err) {
		current, _, readErr := r.readClaim(ctx, key)
		if readErr == nil && current != nil && current.Token == original.Token {
			return r.adopt(originalCtx, key, original)
		}
	}
	return nil, fmt.Errorf("%w: could not conclusively fence %s; its original create or fence may still commit: %w", errUtils.ErrArtifactPublicationAmbiguous, r.attempt(key, original), err)
}

// attempt describes one acquisition attempt so an operator can match an ambiguous outcome to the
// claim object it may have left behind: the claim content carries the same owner, operation, and
// lock ID.
func (r *Repository) attempt(key string, doc *claimDoc) string {
	return fmt.Sprintf("lock %q attempt by owner %q (operation %q, lock ID %s) at claim object %s",
		doc.Resource, doc.Owner, doc.OperationID, doc.Token, r.uri(key))
}

// adopt turns a created claim into a handle. A context that ended while the claim was being created
// releases it again at once, so cancellation never leaves ownership behind.
func (r *Repository) adopt(ctx context.Context, key string, doc *claimDoc) (*claimHandle, error) {
	handle := &claimHandle{repo: r, key: key, resource: doc.Resource, owner: doc.Owner, operationID: doc.OperationID, token: doc.Token}
	if err := ctx.Err(); err != nil {
		if releaseErr := handle.Release(ctx); releaseErr != nil {
			return nil, fmt.Errorf("%w: and the claim %s it created could not be released: %w", err, r.uri(key), releaseErr)
		}
		return nil, err
	}
	return handle, nil
}

// readClaim reads a claim object. A missing claim returns (nil, "", nil); an unreadable one is
// returned as a claim with no token so diagnostics can still name the object.
func (r *Repository) readClaim(ctx context.Context, key string) (*claimDoc, string, error) {
	raw, etag, err := r.readObject(ctx, key, maxClaimBytes)
	if err != nil || raw == nil {
		return nil, "", err
	}
	var doc claimDoc
	if jsonErr := json.Unmarshal(raw, &doc); jsonErr != nil {
		return &claimDoc{Owner: fmt.Sprintf("(unreadable claim: %v)", jsonErr)}, etag, nil
	}
	return &doc, etag, nil
}

// heldError builds the recovery diagnostic for a claim that another owner holds. It names the claim
// object, the resource, the owner, and the operation, and says how an operator recovers.
func (r *Repository) heldError(ctx context.Context, key, resource string, cause error) error {
	readCtx, cancel := detached(ctx)
	defer cancel()
	holder, _, _ := r.readClaim(readCtx, key)
	message := fmt.Sprintf("lock %q claim object %s is no longer present", resource, r.uri(key))
	if holder != nil {
		message = fmt.Sprintf("lock %q is held by owner %q (operation %q, since %s) through claim object %s",
			resource, holder.Owner, holder.OperationID, holder.AcquiredAt, r.uri(key))
		if strings.HasPrefix(holder.Owner, reconciliationOwnerPrefix) {
			message += "; this claim fences an ambiguous create. Do not delete it until the original remote request is known to have finished"
		} else {
			message += "; claims never expire and are never taken over automatically. If that writer has stopped, confirm it is no longer running and delete the claim object to recover"
		}
	}
	if cause != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrArtifactConflict, message, cause)
	}
	return fmt.Errorf("%w: %s", errUtils.ErrArtifactConflict, message)
}

// Resource implements [artifact.LockHandle].
func (l *claimHandle) Resource() string {
	defer perf.Track(nil, "s3.claimHandle.Resource")()

	return l.resource
}

// Owner implements [artifact.LockHandle].
func (l *claimHandle) Owner() string {
	defer perf.Track(nil, "s3.claimHandle.Owner")()

	return l.owner
}

// Scope implements [artifact.LockHandle]; claims coordinate writers across hosts.
func (l *claimHandle) Scope() artifact.LockScope {
	defer perf.Track(nil, "s3.claimHandle.Scope")()

	return artifact.LockScopeRepository
}

// Token implements [artifact.LockHandle].
func (l *claimHandle) Token() string {
	defer perf.Track(nil, "s3.claimHandle.Token")()

	return l.token
}

// ExpiresAt implements [artifact.LockHandle]; claims never expire.
func (l *claimHandle) ExpiresAt() (time.Time, bool) {
	defer perf.Track(nil, "s3.claimHandle.ExpiresAt")()

	return time.Time{}, false
}

// Renew implements [artifact.LockHandle]; claims have no leases.
func (l *claimHandle) Renew(context.Context, time.Duration) error {
	defer perf.Track(nil, "s3.claimHandle.Renew")()

	return artifact.NewUnsupportedCapability(l.repo.alias, opLock, "lease", "S3 claims do not expire automatically, so there is nothing to renew")
}

// Release implements [artifact.LockHandle]. It reads the claim, checks that it still carries this
// handle's ownership token, and deletes it with a conditional delete on the observed ETag, so a
// stale or foreign handle can never release another operation's claim. Release proceeds even when
// ctx was canceled: a canceled operation must still let go.
func (l *claimHandle) Release(ctx context.Context) error {
	defer perf.Track(nil, "s3.claimHandle.Release")()

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return fmt.Errorf("%w: lock %q was already released", errUtils.ErrArtifactConflict, l.resource)
	}
	releaseCtx, cancel := detached(ctx)
	defer cancel()
	current, etag, err := l.repo.readClaim(releaseCtx, l.key)
	switch {
	case err != nil:
		return err
	case current == nil:
		return fmt.Errorf("%w: lock %q is not held: claim object %s is gone", errUtils.ErrArtifactConflict, l.resource, l.repo.uri(l.key))
	case current.Token != l.token:
		return fmt.Errorf("%w: lock %q is held by owner %q (operation %q), not by token %s; refusing to release another owner's claim",
			errUtils.ErrArtifactConflict, l.resource, current.Owner, current.OperationID, l.token)
	}
	if err := l.repo.deleteConditional(releaseCtx, l.key, etag); err != nil && !l.goneAfter(releaseCtx, err) {
		return fmt.Errorf("release lock %q: %w", l.resource, err)
	}
	l.released = true
	return nil
}

// goneAfter reports whether a failed conditional delete nevertheless removed the claim: after a
// lost response, an absent claim proves it did.
func (l *claimHandle) goneAfter(ctx context.Context, err error) bool {
	if !blob.IsAmbiguous(err) {
		return false
	}
	current, _, readErr := l.repo.readClaim(ctx, l.key)
	return readErr == nil && current == nil
}

// holdClaim validates an existing claim or acquires one, and keeps it held through a mutation.
func (r *Repository) holdClaim(ctx context.Context, supplied artifact.LockHandle, resource, operationID string) (func() error, error) {
	handle, local := supplied.(*claimHandle)
	if !local || handle == nil || handle.repo != r {
		handle, err := r.acquire(ctx, resource, r.settings.owner, operationID)
		if err != nil {
			return nil, err
		}
		return func() error { return handle.Release(ctx) }, nil
	}
	if handle.resource != resource {
		return nil, fmt.Errorf("%w: supplied lock does not guard %s", errUtils.ErrArtifactConflict, resource)
	}
	handle.mu.Lock()
	if handle.released {
		handle.mu.Unlock()
		return nil, fmt.Errorf("%w: supplied lock is released", errUtils.ErrArtifactConflict)
	}
	claim, _, err := r.readClaim(ctx, handle.key)
	if err != nil {
		handle.mu.Unlock()
		return nil, err
	}
	if claim == nil || claim.Token != handle.token {
		handle.mu.Unlock()
		return nil, fmt.Errorf("%w: supplied lock is no longer held", errUtils.ErrArtifactConflict)
	}
	return func() error { handle.mu.Unlock(); return nil }, nil
}
