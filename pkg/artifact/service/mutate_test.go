package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

func TestDelete_ScopedToTheSelectedArtifact(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.publish("dst", e.source("keep"))
	other := e.publishRequest("dst", e.source("drop"))
	other.Version = "2.0.0"
	_, err := e.svc.Publish(context.Background(), other)
	require.NoError(t, err)

	result, err := e.svc.Delete(context.Background(), artifact.DeleteRequest{Repository: "dst", Ref: ref("tools/example", "2.0.0")})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeDeleted, result.Outcome)
	assert.NotEmpty(t, result.Location)
	assert.Equal(t, artifact.LockScopeHost, result.LockScope)
	assert.Empty(t, e.repos["dst"].HeldLocks())

	_, err = e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: ref("tools/example", "2.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
	require.NoError(t, err, "other versions are untouched")

	_, err = e.svc.Delete(context.Background(), artifact.DeleteRequest{Repository: "dst", Ref: ref("tools/example", "2.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	assert.Empty(t, e.repos["dst"].HeldLocks(), "locks are released on failure too")
}

func TestDelete_CapabilityAndRequestChecks(t *testing.T) {
	noDelete := artifacttest.DefaultCapabilities()
	noDelete.Delete = artifact.DeleteNone
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"dst": {Capabilities: &noDelete}}})
	e.publish("dst", e.source("hello"))
	locks := e.repos["dst"].Calls("Lock")
	_, err := e.svc.Delete(context.Background(), artifact.DeleteRequest{Repository: "dst", Ref: exampleRef})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.Equal(t, locks, e.repos["dst"].Calls("Lock"), "an unsupported delete takes no lock")

	_, err = e.svc.Delete(context.Background(), artifact.DeleteRequest{Ref: exampleRef})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = e.svc.Delete(context.Background(), artifact.DeleteRequest{Repository: "dst", Ref: artifact.Ref{Name: "x"}})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
}

func TestDelete_ReportsAffectedReferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := artifact.NewMockRepository(ctrl)
	caps := artifacttest.DefaultCapabilities()
	handle := artifact.NewMockLockHandle(ctrl)
	record := artifact.Record{Name: "x/y", Version: "1", Kind: artifact.KindFiles, CreatedAt: artifact.NewTimestamp(startTime)}
	require.NoError(t, record.Seal())
	recordDigest, err := record.RecordDigest()
	require.NoError(t, err)
	pinned := artifact.PinnedReference{Ref: ref("x/y", "1"), ResolvedDigest: recordDigest, RecordDigest: recordDigest}

	repo.EXPECT().Capabilities().Return(caps).AnyTimes()
	repo.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(handle, nil)
	handle.EXPECT().Release(gomock.Any()).Return(nil)
	handle.EXPECT().Resource().Return("x/y:1").AnyTimes()
	handle.EXPECT().Owner().Return("test-owner").AnyTimes()
	handle.EXPECT().Scope().Return(artifact.LockScopeHost).AnyTimes()
	repo.EXPECT().Resolve(gomock.Any(), ref("x/y", "1")).Return(pinned, nil)
	repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned, Record: &record}, nil)
	repo.EXPECT().Delete(gomock.Any(), pinned, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ artifact.PinnedReference, options artifact.DeleteOptions) (*artifact.DeleteResult, error) {
			assert.Equal(t, artifact.DeleteObject, options.Scope)
			require.NotNil(t, options.ExpectedCurrent)
			assert.Equal(t, recordDigest, options.ExpectedCurrent.ResolvedDigest)
			assert.NotNil(t, options.Lock)
			return &artifact.DeleteResult{Deleted: []string{"loc-1", "loc-2"}, Affected: []artifact.Ref{ref("x/y", "latest-ish")}}, nil
		},
	)
	repos := &service.StaticRepositories{Repositories: map[string]artifact.Repository{"dst": repo}, Info: map[string]service.RepositoryInfo{"dst": {Capabilities: caps}}}
	svc, err := service.New(repos, nil, service.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)

	result, err := svc.Delete(context.Background(), artifact.DeleteRequest{Repository: "dst", Ref: ref("x/y", "1")})
	require.NoError(t, err)
	assert.Equal(t, "loc-1", result.Location)
	assert.Equal(t, []string{"loc-2"}, result.Aliases)
	var affected bool
	for _, diagnostic := range result.Diagnostics {
		affected = affected || diagnostic.Code == "affected-reference"
	}
	assert.True(t, affected)
}

func TestPin_IsImmutableDataTakesNoLockAndGrantsNoTrust(t *testing.T) {
	e := newEnv(t, envConfig{options: []service.Option{service.WithSigner(releaseSigner)}})
	published := e.publish("dst", e.source("hello"))
	locks := e.repos["dst"].Calls("Lock")

	result, err := e.svc.Pin(context.Background(), artifact.PinRequest{Repository: "dst", Ref: exampleRef})
	require.NoError(t, err)
	assert.Equal(t, published.Operation.RecordDigest, result.Pinned.ResolvedDigest)
	assert.Equal(t, published.Operation.RecordDigest, result.Operation.RecordDigest)
	assert.Equal(t, locks, e.repos["dst"].Calls("Lock"), "a pin is not a write mutex")
	assert.Equal(t, 0, e.repos["dst"].Calls("Verify"), "a pin is not a substitute for verification")
	var note bool
	for _, diagnostic := range result.Operation.Diagnostics {
		note = note || diagnostic.Code == "pin-not-trust"
	}
	assert.True(t, note)

	_, err = e.svc.Pin(context.Background(), artifact.PinRequest{Repository: "dst", Ref: ref("tools/example", "9")})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = e.svc.Pin(context.Background(), artifact.PinRequest{Ref: exampleRef})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

func TestLocks_HeldByAnotherOwnerBlocksUntilCanceledAndLeaksNothing(t *testing.T) {
	e := newEnv(t, envConfig{})
	other, err := e.repos["dst"].Lock(context.Background(), artifact.LockRequest{Resource: exampleRef.String(), Owner: "someone-else"})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	outcome, err := e.svc.PublishDetailed(ctx, e.publishRequest("dst", e.source("hello")))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Zero(t, e.repos["dst"].Calls("Publish"), "nothing is written without the lock")
	assert.Equal(t, []string{exampleRef.String()}, e.repos["dst"].HeldLocks(), "the other owner's claim is neither stolen nor leaked")
	assert.Empty(t, outcome.Operation.Outcome)

	require.NoError(t, other.Release(context.Background()))
	retried, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", e.source("hello")))
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, retried.Operation.Outcome)
}

func TestLocks_ReleasedWithADetachedContextAfterCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := artifact.NewMockRepository(ctrl)
	handle := artifact.NewMockLockHandle(ctrl)
	caps := artifacttest.DefaultCapabilities()
	repo.EXPECT().Capabilities().Return(caps).AnyTimes()
	handle.EXPECT().Resource().Return("tools/example:1.0.0").AnyTimes()
	handle.EXPECT().Owner().Return("test-owner").AnyTimes()
	handle.EXPECT().Scope().Return(artifact.LockScopeHost).AnyTimes()
	repo.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(handle, nil)
	repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(artifact.PinnedReference{}, errUtils.ErrArtifactNotFound).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	repo.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *artifact.PreparedArtifact, artifact.PublishOptions) (*artifact.PublishResult, error) {
			cancel()
			return nil, context.Canceled
		},
	)
	var released bool
	handle.EXPECT().Release(gomock.Any()).DoAndReturn(func(releaseCtx context.Context) error {
		released = true
		assert.NoError(t, releaseCtx.Err(), "release must not inherit the canceled context")
		return nil
	})
	repos := &service.StaticRepositories{Repositories: map[string]artifact.Repository{"dst": repo}, Info: map[string]service.RepositoryInfo{"dst": {Capabilities: caps, Packaging: artifact.PackagingNone}}}
	e := newEnv(t, envConfig{})
	svc, err := service.New(repos, nil, service.WithStagingRoot(e.staging), service.WithClock(e.clock.Now), service.WithRetry(1, 0))
	require.NoError(t, err)

	_, err = svc.Publish(ctx, e.publishRequest("dst", e.source("hello")))
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, released, "the lock was released after cancellation")
	e.assertStagingClean()
}

func TestLocks_ReleaseFailureIsReportedForOperatorRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := artifact.NewMockRepository(ctrl)
	handle := artifact.NewMockLockHandle(ctrl)
	caps := artifacttest.DefaultCapabilities()
	repo.EXPECT().Capabilities().Return(caps).AnyTimes()
	handle.EXPECT().Resource().Return("tools/example:1.0.0").AnyTimes()
	handle.EXPECT().Owner().Return("test-owner").AnyTimes()
	handle.EXPECT().Scope().Return(artifact.LockScopeHost).AnyTimes()
	repo.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(handle, nil)
	repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(artifact.PinnedReference{}, errUtils.ErrArtifactNotFound)
	repo.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&artifact.PublishResult{Outcome: artifact.OutcomeCreated, Pinned: artifact.PinnedReference{ResolvedDigest: digest.FromString("x")}, Representation: artifact.Representation{Payload: []ocispec.Descriptor{}}}, nil,
	)
	handle.EXPECT().Release(gomock.Any()).Return(errTransport)
	repos := &service.StaticRepositories{Repositories: map[string]artifact.Repository{"dst": repo}, Info: map[string]service.RepositoryInfo{"dst": {Capabilities: caps, Packaging: artifact.PackagingNone}}}
	e := newEnv(t, envConfig{})
	svc, err := service.New(repos, nil, service.WithStagingRoot(e.staging))
	require.NoError(t, err)

	result, err := svc.Publish(context.Background(), e.publishRequest("dst", e.source("hello")))
	require.NoError(t, err, "the publication itself succeeded")
	var warning artifact.Diagnostic
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "lock-release-failed" {
			warning = diagnostic
		}
	}
	assert.Contains(t, warning.Message, "test-owner")
	assert.Contains(t, warning.Message, "tools/example:1.0.0")
}

func TestStaticRepositories(t *testing.T) {
	repos := &service.StaticRepositories{Repositories: map[string]artifact.Repository{"b": artifacttest.New(artifacttest.Options{}), "a": artifacttest.New(artifacttest.Options{})}}
	assert.Equal(t, []string{"a", "b"}, repos.Aliases())
	info, err := repos.Describe("a")
	require.NoError(t, err)
	assert.Equal(t, "a", info.Alias)
	assert.True(t, info.Capabilities.Write)
	assert.Equal(t, artifact.PackagingNone, info.Packaging)
	_, err = repos.Describe("missing")
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = repos.Open(context.Background(), "missing")
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	repo, err := repos.Open(context.Background(), "a")
	require.NoError(t, err)
	assert.NotNil(t, repo)
}
