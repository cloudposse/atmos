package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

var errTransport = errors.New("connection reset")

func TestPublish_InterruptionBeforePayloadLeavesNothingAndRetryCreates(t *testing.T) {
	e := newEnv(t, envConfig{})
	src := e.source("hello")
	e.repos["dst"].InjectFailure(artifacttest.FailBeforePayload, errTransport, 1)

	outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", src))
	require.ErrorIs(t, err, errTransport)
	assert.Empty(t, outcome.Operation.Outcome, "a failed write never reports success")
	_, found := e.repos["dst"].RecordDigestOf(exampleRef)
	assert.False(t, found)
	assert.False(t, e.repos["dst"].IsPending(exampleRef))
	assert.Equal(t, 1, e.repos["dst"].Calls("Publish"), "a non-temporary failure is never blind-retried")
	assert.Empty(t, e.repos["dst"].HeldLocks())

	retry, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", src))
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, retry.Operation.Outcome)
}

func TestPublish_InterruptionDuringPayloadOrAfterMetadataIsAmbiguousUntilSameIntentRetry(t *testing.T) {
	for _, point := range []artifacttest.FailPoint{artifacttest.FailDuringPayload, artifacttest.FailAfterMetadata} {
		t.Run(string(point), func(t *testing.T) {
			e := newEnv(t, envConfig{})
			src := e.source("hello")
			e.repos["dst"].InjectFailure(point, errTransport, 1)

			outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", src))
			require.ErrorIs(t, err, errUtils.ErrArtifactPublicationAmbiguous)
			require.ErrorIs(t, err, errTransport, "the cause is retained for diagnostics")
			assert.Contains(t, err.Error(), outcome.Operation.OperationID, "recovery diagnostics name the operation")
			assert.Empty(t, outcome.Operation.Outcome, "partial work is never advertised as success")
			assert.True(t, e.repos["dst"].IsPending(exampleRef), "the reference stays unavailable")
			assert.Equal(t, 1, e.repos["dst"].Calls("Publish"), "no blind retry of an ambiguous mutation")
			assert.Empty(t, e.repos["dst"].HeldLocks())

			_, err = e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
			require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "readers cannot see a pending publication")

			retry, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", src))
			require.NoError(t, err, "the same-intent retry repairs the interrupted publication")
			assert.Equal(t, artifact.OutcomeCreated, retry.Operation.Outcome)
			assert.False(t, e.repos["dst"].IsPending(exampleRef))
			_, err = e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
			require.NoError(t, err)
		})
	}
}

func TestPublish_DifferentIntentCannotStealAPendingPublication(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.repos["dst"].InjectFailure(artifacttest.FailDuringPayload, errTransport, 1)
	_, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", e.source("first")))
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationAmbiguous)

	other, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", e.source("different")))
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
	assert.Contains(t, err.Error(), "op-1", "the pending operation ID is surfaced for recovery")
	assert.Empty(t, other.Operation.Outcome)
}

func TestPublish_CommitWithLostResponseIsReconciledNotRetried(t *testing.T) {
	e := newEnv(t, envConfig{})
	src := e.source("hello")
	e.repos["dst"].InjectFailure(artifacttest.FailAfterCommit, errTransport, 1)

	outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", src))
	require.NoError(t, err, "success only after reconciliation read the completed reference")
	assert.Equal(t, artifact.OutcomeCreated, outcome.Operation.Outcome)
	assert.Equal(t, 1, e.repos["dst"].Calls("Publish"), "reconciliation replaces the retry")
	var reconciled bool
	for _, diagnostic := range outcome.Operation.Diagnostics {
		reconciled = reconciled || diagnostic.Code == "reconciled"
	}
	assert.True(t, reconciled, "the reconciliation is reported")
	assert.Contains(t, e.journal.phases(), service.PhaseReconciled)
	current, ok := e.repos["dst"].RecordDigestOf(exampleRef)
	require.True(t, ok)
	assert.Equal(t, outcome.Operation.RecordDigest, current)
}

func TestPublish_CommitWithLostResponseOnReplacementIsReconciled(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.publish("dst", e.source("first"))
	e.repos["dst"].InjectFailure(artifacttest.FailAfterCommit, errTransport, 1)
	request := e.publishRequest("dst", e.source("second"))
	request.Force = true
	outcome, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeReplaced, outcome.Operation.Outcome)
}

func TestPublish_TemporaryFailuresRetryOnlyAfterReconciliationProvesNothingCommitted(t *testing.T) {
	e := newEnv(t, envConfig{})
	src := e.source("hello")
	e.repos["dst"].InjectFailure(artifacttest.FailBeforePayload, &temporaryError{"throttled"}, 2)

	outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", src))
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, outcome.Operation.Outcome)
	assert.Equal(t, 3, e.repos["dst"].Calls("Publish"))
	e.sleepMu.Lock()
	assert.Equal(t, []time.Duration{time.Millisecond, 2 * time.Millisecond}, e.sleeps, "exponential backoff")
	e.sleepMu.Unlock()

	var attempts []int
	for _, entry := range e.journal.snapshot() {
		assert.Equal(t, "op-1", entry.OperationID, "every attempt reuses the operation ID")
		if entry.Phase == service.PhasePending {
			attempts = append(attempts, entry.Attempt)
		}
	}
	assert.Equal(t, []int{1, 2, 3}, attempts)
}

func TestPublish_TemporaryFailureExhaustsAttempts(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.repos["dst"].InjectFailure(artifacttest.FailBeforePayload, &temporaryError{"throttled"}, 10)
	_, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", e.source("hello")))
	var temporary *temporaryError
	require.ErrorAs(t, err, &temporary)
	assert.Equal(t, 3, e.repos["dst"].Calls("Publish"))
}

func TestPublish_TemporaryFailureAfterCommitIsReconciledWithoutRetry(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.repos["dst"].InjectFailure(artifacttest.FailAfterCommit, &temporaryError{"timeout"}, 5)
	outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", e.source("hello")))
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, outcome.Operation.Outcome)
	assert.Equal(t, 1, e.repos["dst"].Calls("Publish"), "a committed publication is never repeated")
}

func TestPublish_DateRolloverAcrossRetriesReusesCapturedTime(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.clock.Set(time.Date(2026, 3, 4, 23, 59, 59, 0, time.UTC))
	e.onSleep = func(time.Duration) { e.clock.Advance(2 * time.Second) } // Crosses midnight between attempts.
	e.repos["dst"].InjectFailure(artifacttest.FailBeforePayload, &temporaryError{"throttled"}, 1)

	outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", e.source("hello")))
	require.NoError(t, err)
	assert.Equal(t, "2026-03-04T23:59:59Z", outcome.Record.CreatedAt.String(), "the retry reuses the time captured before the first attempt")
	assert.Equal(t, 2, e.repos["dst"].Calls("Publish"))
	assert.Equal(t, "op-1", outcome.Operation.OperationID)
	assert.Equal(t, []service.JournalPhase{
		service.PhasePrepared, service.PhaseLocked, service.PhasePending, service.PhasePending, service.PhaseCommitted, service.PhaseReleased,
	}, e.journal.phases(), "one lifecycle, two attempts")
}

func TestPublish_FailedForceReplacementLeavesReferenceUnavailableUntilSameIntentRetry(t *testing.T) {
	e := newEnv(t, envConfig{})
	first := e.publish("dst", e.source("first"))
	oldPinned, err := e.repos["dst"].Resolve(context.Background(), exampleRef)
	require.NoError(t, err)

	e.repos["dst"].InjectFailure(artifacttest.FailDuringPayload, errTransport, 1)
	request := e.publishRequest("dst", e.source("second"))
	request.Force = true
	_, err = e.svc.PublishDetailed(context.Background(), request)
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationAmbiguous)
	assert.True(t, e.repos["dst"].IsPending(exampleRef))
	_, err = e.repos["dst"].Resolve(context.Background(), exampleRef)
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "no rollback is claimed: the reference is unavailable, not silently the old version")

	retried, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeReplaced, retried.Operation.Outcome)
	assert.NotEqual(t, first.Operation.RecordDigest, retried.Operation.RecordDigest)

	_, err = e.repos["dst"].Read(context.Background(), oldPinned)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "the old digest never serves the replacement's bytes")
}

func TestPublish_TwoWritersOneReferenceLockSerializesThem(t *testing.T) {
	const writers = 8
	e := newEnv(t, envConfig{})
	sources := make([]string, writers)
	for i := range sources {
		sources[i] = e.source(fmt.Sprintf("content-%d", i))
	}
	var wg sync.WaitGroup
	var created, conflicts atomic.Int64
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", sources[i]))
			switch {
			case err == nil:
				created.Add(1)
			case errors.Is(err, errUtils.ErrArtifactConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), created.Load(), "exactly one writer wins the reference")
	assert.Equal(t, int64(writers-1), conflicts.Load())
	assert.Empty(t, e.repos["dst"].HeldLocks())
	assert.Equal(t, 1, e.repos["dst"].Calls("Publish"), "losers conflict before writing anything")
}

func TestPublish_TwoWritersWithIdenticalIntentCreateOnce(t *testing.T) {
	const writers = 6
	e := newEnv(t, envConfig{})
	src := e.source("same")
	var wg sync.WaitGroup
	var created, unchanged atomic.Int64
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", src))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			switch outcome.Operation.Outcome {
			case artifact.OutcomeCreated:
				created.Add(1)
			case artifact.OutcomeUnchanged:
				unchanged.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), created.Load())
	assert.Equal(t, int64(writers-1), unchanged.Load())
	assert.Equal(t, 1, e.repos["dst"].Calls("Publish"))
}

// unlockedRepository hides the writer lock so the repository's own conditional guard is exercised.
type unlockedRepository struct {
	*artifacttest.Repository
}

func (u *unlockedRepository) Lock(_ context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	return noopLock{resource: request.Resource}, nil
}

type noopLock struct{ resource string }

func (n noopLock) Resource() string                           { return n.resource }
func (n noopLock) Owner() string                              { return "noop" }
func (n noopLock) Scope() artifact.LockScope                  { return artifact.LockScopeHost }
func (n noopLock) Token() string                              { return "noop" }
func (n noopLock) ExpiresAt() (time.Time, bool)               { return time.Time{}, false }
func (n noopLock) Renew(context.Context, time.Duration) error { return nil }
func (n noopLock) Release(context.Context) error              { return nil }

func TestPublish_ConditionalGuardRejectsRacingWritersEvenWithoutAWriterLock(t *testing.T) {
	const writers = 6
	e := newEnv(t, envConfig{})
	bare := artifacttest.New(artifacttest.Options{Alias: "bare"})
	e.static.Repositories["bare"] = &unlockedRepository{Repository: bare}
	e.static.Info["bare"] = service.RepositoryInfo{Alias: "bare", Capabilities: bare.Capabilities(), Packaging: artifact.PackagingNone}

	sources := make([]string, writers)
	for i := range sources {
		sources[i] = e.source(fmt.Sprintf("racing-%d", i))
	}
	var wg sync.WaitGroup
	var wins atomic.Int64
	var digests sync.Map
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("bare", sources[i]))
			if err == nil {
				wins.Add(1)
				digests.Store(outcome.Operation.RecordDigest, true)
			}
		}()
	}
	wg.Wait()
	assert.GreaterOrEqual(t, wins.Load(), int64(1))
	current, ok := bare.RecordDigestOf(exampleRef)
	require.True(t, ok)
	var count int
	digests.Range(func(key, _ any) bool { count++; return true })
	assert.Equal(t, int64(count), wins.Load(), "each reported success corresponds to a distinct committed record")
	pinned, err := bare.Resolve(context.Background(), exampleRef)
	require.NoError(t, err)
	assert.Equal(t, current, pinned.RecordDigest, "the reference is bound to exactly one artifact")
}
