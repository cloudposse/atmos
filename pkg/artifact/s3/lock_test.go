package s3

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

const lockResource = "team/app:1.0.0"

func lockObjects(fake *s3test.Server) []string {
	return managedKeys(fake, "", layout.ManagedRoot+"/locks")
}

// peerOf returns a second repository over the same bucket, the way a second host would see it.
func peerOf(t *testing.T, spec *rspec, opts ...Option) *Repository {
	t.Helper()
	return newRepo(t, spec.withOpts(opts...))
}

func TestLock_HandleContract(t *testing.T) {
	spec := noneSpec().fresh(t).with(func(s *rspec) { s.tags = map[string]string{"team": "platform"} })
	repo := newRepo(t, spec)
	handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "owner-a", OperationID: "op-a", Required: artifact.LockScopeRepository})
	require.NoError(t, err)
	assert.Equal(t, lockResource, handle.Resource())
	assert.Equal(t, "owner-a", handle.Owner())
	assert.NotEmpty(t, handle.Token())
	assert.Equal(t, artifact.LockScopeRepository, handle.Scope(), "the claim coordinates writers across hosts")
	_, expires := handle.ExpiresAt()
	assert.False(t, expires, "V1 claims never expire")
	require.ErrorIs(t, handle.Renew(context.Background(), time.Minute), errUtils.ErrArtifactUnsupportedCapability)

	keys := lockObjects(spec.fake)
	require.Len(t, keys, 1)
	assert.Equal(t, layout.LockPath(lockResource), keys[0])
	object, ok := spec.fake.Object(keys[0])
	require.True(t, ok)
	var doc claimDoc
	require.NoError(t, json.Unmarshal(object.Data, &doc))
	assert.Equal(t, claimDoc{SchemaVersion: 1, Resource: lockResource, Owner: "owner-a", OperationID: "op-a", Token: handle.Token(), AcquiredAt: doc.AcquiredAt}, doc)
	assert.Equal(t, map[string]string{"team": "platform"}, object.Tags, "claims are tagged like every other object")
	creates := 0
	for _, request := range spec.fake.RequestsFor(s3test.OpPutObject) {
		if request.Key == keys[0] {
			creates++
			assert.Equal(t, "*", request.Header.Get("If-None-Match"), "the claim is created if absent")
		}
	}
	assert.Equal(t, 1, creates)

	require.NoError(t, handle.Release(context.Background()))
	assert.Empty(t, lockObjects(spec.fake))
	var release s3test.Request
	for _, request := range spec.fake.RequestsFor(s3test.OpDeleteObject) {
		release = request
	}
	assert.NotEmpty(t, release.Header.Get("If-Match"), "release deletes conditionally on the observed claim")
}

func TestLock_RejectsUnsupportedRequests(t *testing.T) {
	repo := newRepo(t, noneSpec())
	tests := []struct {
		name    string
		request artifact.LockRequest
		wantErr error
	}{
		{name: "lease", request: artifact.LockRequest{Resource: "x", Lease: time.Minute}, wantErr: errUtils.ErrArtifactUnsupportedCapability},
		{name: "unknown scope", request: artifact.LockRequest{Resource: "x", Required: "galaxy"}, wantErr: errUtils.ErrArtifactUnsupportedCapability},
		{name: "no resource", request: artifact.LockRequest{}, wantErr: errUtils.ErrArtifactRefInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handle, err := repo.Lock(context.Background(), tt.request)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, handle)
		})
	}
	for _, scope := range []artifact.LockScope{"", artifact.LockScopeHost, artifact.LockScopeRepository} {
		handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: "scope-" + string(scope), Required: scope})
		require.NoError(t, err, "scope %q", scope)
		require.NoError(t, handle.Release(context.Background()))
	}
}

func TestLock_ReleaseChecksTheOwnershipToken(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	other := peerOf(t, spec)
	handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "owner-a"})
	require.NoError(t, err)
	claim, ok := handle.(*claimHandle)
	require.True(t, ok)

	t.Run("a handle with the wrong token cannot release", func(t *testing.T) {
		forged := &claimHandle{repo: other, key: claim.key, resource: lockResource, token: "not-the-token"}
		require.ErrorIs(t, forged.Release(context.Background()), errUtils.ErrArtifactConflict)
		assert.Len(t, lockObjects(spec.fake), 1, "the claim is untouched")
	})

	t.Run("an operator removal and a new owner make the old handle harmless", func(t *testing.T) {
		require.NoError(t, repo.store.Delete(context.Background(), claim.key, blob.DeleteOptions{}))
		successor, err := other.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "owner-b"})
		require.NoError(t, err)
		err = handle.Release(context.Background())
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
		require.ErrorContains(t, err, "owner-b", "the diagnostic names the real owner")
		assert.Len(t, lockObjects(spec.fake), 1, "the successor's claim survives the stale release")
		require.NoError(t, successor.Release(context.Background()))
	})

	t.Run("a claim that is gone cannot be released", func(t *testing.T) {
		require.ErrorIs(t, handle.Release(context.Background()), errUtils.ErrArtifactConflict)
	})
}

func TestLock_DoubleReleaseIsRejected(t *testing.T) {
	repo := newRepo(t, noneSpec())
	handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: lockResource})
	require.NoError(t, err)
	require.NoError(t, handle.Release(context.Background()))
	require.ErrorIs(t, handle.Release(context.Background()), errUtils.ErrArtifactConflict)
}

// A competing owner is rejected with a diagnostic that names the claim object, never taken over.
func TestLock_CompetingOwnerIsRejectedWithARecoveryDiagnostic(t *testing.T) {
	spec := noneSpec().fresh(t)
	holder := newRepo(t, spec)
	waiter := peerOf(t, spec, WithLockWait(40*time.Millisecond))
	held, err := holder.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "host-a:100", OperationID: "op-holder"})
	require.NoError(t, err)

	handle, err := waiter.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "host-b:200", OperationID: "op-waiter"})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	assert.Nil(t, handle)
	for _, want := range []string{"host-a:100", "op-holder", "claim object s3://" + spec.fake.Bucket() + "/" + layout.LockPath(lockResource), "never expire", "delete the claim object"} {
		assert.ErrorContains(t, err, want)
	}
	assert.Len(t, lockObjects(spec.fake), 1, "the waiter wrote nothing")

	require.NoError(t, held.Release(context.Background()))
	handle, err = waiter.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "host-b:200"})
	require.NoError(t, err)
	require.NoError(t, handle.Release(context.Background()))
}

// The context bounds the wait too, and its error stays visible alongside the conflict.
func TestLock_WaitIsBoundedByTheContext(t *testing.T) {
	spec := noneSpec().fresh(t)
	holder := newRepo(t, spec)
	_, err := holder.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "owner-a"})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = peerOf(t, spec).Lock(ctx, artifact.LockRequest{Resource: lockResource, Owner: "owner-b"})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
}

// A crash leaves the claim; nothing expires or steals it, and removing exactly that object recovers.
func TestLock_CrashLeftClaimBlocksUntilTheOperatorRemovesIt(t *testing.T) {
	spec := noneSpec().fresh(t)
	crashed := newRepo(t, spec)
	_, err := crashed.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "crashed-host:1", OperationID: "op-crashed"})
	require.NoError(t, err)
	// The process dies here: no Release.

	survivor := peerOf(t, spec, WithLockWait(20*time.Millisecond))
	for attempt := 0; attempt < 3; attempt++ {
		_, err = survivor.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "survivor:2"})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "claims never expire, however long they sit")
		require.ErrorContains(t, err, "crashed-host:1")
		require.ErrorContains(t, err, "op-crashed")
	}

	keys := lockObjects(spec.fake)
	require.Len(t, keys, 1)
	require.NoError(t, survivor.store.Delete(context.Background(), keys[0], blob.DeleteOptions{}), "the operator deletes the one named claim object")
	handle, err := survivor.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "survivor:2"})
	require.NoError(t, err)
	require.NoError(t, handle.Release(context.Background()))
}

// Cancellation, in every shape, leaves no claim of ours behind.
func TestLock_CanceledAcquisitionLeaksNothing(t *testing.T) {
	t.Run("canceled while waiting", func(t *testing.T) {
		spec := noneSpec().fresh(t)
		holder := newRepo(t, spec)
		held, err := holder.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "owner-a"})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = peerOf(t, spec).Lock(ctx, artifact.LockRequest{Resource: lockResource, Owner: "owner-b"})
		require.ErrorIs(t, err, context.Canceled)
		require.NoError(t, held.Release(context.Background()))
		assert.Empty(t, lockObjects(spec.fake), "only the holder's claim ever existed")
	})

	for _, lostResponse := range []bool{false, true} {
		name := "canceled just after the claim was created"
		if lostResponse {
			name = "canceled with the create response lost"
		}
		t.Run(name, func(t *testing.T) {
			spec := noneSpec().fresh(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &hookStore{Store: newStoreFor(t, spec)}
			store.afterPut = func(key string, info *blob.ObjectInfo, err error) (*blob.ObjectInfo, error) {
				cancel()
				if lostResponse {
					return nil, blob.NewError(blob.KindAmbiguous, blob.OpPut, key, nil)
				}
				return info, err
			}
			repo, err := newRepository("test", spec.config(spec.fake.Bucket()), []Option{WithStore(store)}, &artifact.RepositoryOptions{})
			require.NoError(t, err)

			handle, err := repo.Lock(ctx, artifact.LockRequest{Resource: lockResource, Owner: "owner-a"})
			require.ErrorIs(t, err, context.Canceled)
			assert.Nil(t, handle)
			assert.Empty(t, lockObjects(spec.fake), "the claim that was created is released again")
		})
	}
}

// A lost response to the create or to the release is settled by reading the claim back.
func TestLock_LostResponsesAreReconciled(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	key := layout.LockPath(lockResource)

	spec.fake.Inject(s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: key, Action: s3test.DropAfterCommit})
	handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "owner-a"})
	require.NoError(t, err, "the create was applied, and the claim proves it")
	assert.Len(t, lockObjects(spec.fake), 1)

	spec.fake.Inject(s3test.Fault{Op: s3test.OpDeleteObject, KeyPrefix: key, Action: s3test.DropAfterCommit})
	require.NoError(t, handle.Release(context.Background()), "an absent claim proves the release was applied")
	assert.Empty(t, lockObjects(spec.fake))
}

// Concurrent holders of one resource never overlap.
func TestLock_CompetingHandlesAreMutuallyExclusive(t *testing.T) {
	spec := noneSpec().fresh(t)
	const workers, rounds = 4, 3
	var active, overlaps atomic.Int32
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		repo := peerOf(t, spec, WithLockWait(time.Minute))
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := 0; round < rounds; round++ {
				handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "worker"})
				if !assert.NoError(t, err) {
					return
				}
				if active.Add(1) > 1 {
					overlaps.Add(1)
				}
				time.Sleep(2 * time.Millisecond)
				active.Add(-1)
				assert.NoError(t, handle.Release(context.Background()))
			}
		}()
	}
	wg.Wait()
	assert.Zero(t, overlaps.Load(), "two owners held the same claim at once")
	assert.Empty(t, lockObjects(spec.fake))
}

// Publish honors a reference lock held by another owner and leaks nothing when canceled while
// waiting for it.
func TestPublish_HonorsTheReferenceLock(t *testing.T) {
	spec := noneSpec().fresh(t)
	holder := newRepo(t, spec)
	held, err := holder.Lock(context.Background(), artifact.LockRequest{Resource: testName + ":" + testVersion, Owner: "owner-a"})
	require.NoError(t, err)

	writer := peerOf(t, spec, WithLockWait(30*time.Millisecond))
	_, err = writer.Publish(context.Background(), prepare(t, writer, &prepareSpec{}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	assert.Empty(t, managedKeys(spec.fake, "", layout.ReferencesDir()), "nothing was written while the lock was held elsewhere")
	assert.Empty(t, payloadKeys(spec.fake, ""))

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = writer.Publish(canceled, prepare(t, writer, &prepareSpec{}), artifact.PublishOptions{})
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, held.Release(context.Background()))
	publish(t, writer, &prepareSpec{}, artifact.PublishOptions{})
	assert.Empty(t, lockObjects(spec.fake), "the writer released its own lock and the claims lock")
}

// A publication holding a service lock does not take (and deadlock on) its own.
func TestPublish_UsesTheCallersLock(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: testName + ":" + testVersion, Owner: "service"})
	require.NoError(t, err)
	result, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{}), artifact.PublishOptions{Lock: handle})
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeRepository, result.LockScope)
	assert.Len(t, lockObjects(spec.fake), 1, "the caller's lock is still held")
	require.NoError(t, handle.Release(context.Background()))
}
