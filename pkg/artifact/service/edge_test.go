package service_test

import (
	"bytes"
	"context"
	"errors"
	"io"
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

// mockSetup wires a service around one mock repository named "dst" (and optionally "src").
type mockSetup struct {
	ctrl *gomock.Controller
	repo *artifact.MockRepository
	svc  *service.Service
	e    *env
}

func newMockSetup(t *testing.T, caps *artifact.Capabilities, extra ...service.Option) *mockSetup {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := artifact.NewMockRepository(ctrl)
	repo.EXPECT().Capabilities().Return(*caps).AnyTimes()
	e := newEnv(t, envConfig{})
	repos := &service.StaticRepositories{
		Repositories: map[string]artifact.Repository{"dst": repo},
		Info:         map[string]service.RepositoryInfo{"dst": {Capabilities: *caps, Packaging: artifact.PackagingNone}},
	}
	options := append([]service.Option{service.WithStagingRoot(e.staging), service.WithClock(e.clock.Now), service.WithRetry(2, 0), service.WithSleep(func(context.Context, time.Duration) error { return nil })}, extra...)
	svc, err := service.New(repos, nil, options...)
	require.NoError(t, err)
	return &mockSetup{ctrl: ctrl, repo: repo, svc: svc, e: e}
}

func sealedRecord(t *testing.T) (artifact.Record, digest.Digest) {
	t.Helper()
	record := artifact.Record{Name: "tools/example", Version: "1.0.0", Kind: artifact.KindFiles, CreatedAt: artifact.NewTimestamp(startTime)}
	require.NoError(t, record.Seal())
	recordDigest, err := record.RecordDigest()
	require.NoError(t, err)
	return record, recordDigest
}

// expectNoLock lets a mock repository accept lock calls with a trivial handle.
func (m *mockSetup) expectLock() {
	handle := artifact.NewMockLockHandle(m.ctrl)
	handle.EXPECT().Resource().Return("r").AnyTimes()
	handle.EXPECT().Owner().Return("o").AnyTimes()
	handle.EXPECT().Scope().Return(artifact.LockScopeHost).AnyTimes()
	handle.EXPECT().Release(gomock.Any()).Return(nil).AnyTimes()
	m.repo.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(handle, nil).AnyTimes()
}

func TestPublish_ReconciliationNeverClaimsSuccessWithoutProof(t *testing.T) {
	src := newEnv(t, envConfig{}).source("hello")
	caps := artifacttest.DefaultCapabilities()
	_, otherDigest := sealedRecord(t)

	tests := []struct {
		name      string
		reconcile func(m *mockSetup)
		wantErr   error
	}{
		{"reference cannot be read", func(m *mockSetup) {
			m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(artifact.PinnedReference{}, errTransport)
		}, errUtils.ErrArtifactPublicationAmbiguous},
		{"reference now exists with unrelated content", func(m *mockSetup) {
			m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(artifact.PinnedReference{ResolvedDigest: otherDigest, RecordDigest: otherDigest}, nil)
		}, errUtils.ErrArtifactConflict},
		{"matching record but descriptors differ", func(m *mockSetup) {
			m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).DoAndReturn(func(context.Context, artifact.Ref) (artifact.PinnedReference, error) {
				return artifact.PinnedReference{ResolvedDigest: "sha256:x", RecordDigest: lastPreparedDigest}, nil
			})
			m.repo.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(&artifact.Inspection{
				Representation: artifact.Representation{Payload: []ocispec.Descriptor{{Digest: digest.FromString("other-bytes"), Size: 1}}},
			}, nil)
		}, errUtils.ErrArtifactIntegrityFailed},
		{"matching record but inspection fails", func(m *mockSetup) {
			m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).DoAndReturn(func(context.Context, artifact.Ref) (artifact.PinnedReference, error) {
				return artifact.PinnedReference{ResolvedDigest: "sha256:x", RecordDigest: lastPreparedDigest}, nil
			})
			m.repo.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(nil, errTransport)
		}, errUtils.ErrArtifactPublicationAmbiguous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockSetup(t, &caps)
			m.expectLock()
			// The first Resolve is the pre-write read of the current state.
			first := m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(artifact.PinnedReference{}, errUtils.ErrArtifactNotFound)
			m.repo.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, prepared *artifact.PreparedArtifact, _ artifact.PublishOptions) (*artifact.PublishResult, error) {
					d, err := prepared.Record.RecordDigest()
					require.NoError(t, err)
					lastPreparedDigest = d
					return nil, errTransport
				},
			).After(first)
			tt.reconcile(m)
			result, err := m.svc.Publish(context.Background(), m.e.publishRequest("dst", src))
			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, result.Outcome, "never reported as success")
		})
	}
}

// lastPreparedDigest is set by the publish expectation for use in later reconcile expectations.
var lastPreparedDigest digest.Digest

func TestPublish_ExistingReferenceVanishingDuringReconcileIsAmbiguous(t *testing.T) {
	src := newEnv(t, envConfig{}).source("hello")
	caps := artifacttest.DefaultCapabilities()
	e := newEnv(t, envConfig{})
	prior := e.publish("dst", src)
	changed := e.source("changed")

	m := newMockSetup(t, &caps)
	m.expectLock()
	pinned := artifact.PinnedReference{Ref: exampleRef, ResolvedDigest: prior.Operation.RecordDigest, RecordDigest: prior.Operation.RecordDigest}
	record := prior.Record
	gomock.InOrder(
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil),
		m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned, Record: &record}, nil),
		m.repo.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errTransport),
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(artifact.PinnedReference{}, errUtils.ErrArtifactNotFound),
	)
	request := m.e.publishRequest("dst", changed)
	request.Force = true
	_, err := m.svc.Publish(context.Background(), request)
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationAmbiguous)
}

func TestPublish_NotCommittedReconciliationReturnsTheOriginalError(t *testing.T) {
	src := newEnv(t, envConfig{}).source("hello")
	caps := artifacttest.DefaultCapabilities()
	e := newEnv(t, envConfig{})
	prior := e.publish("dst", src)

	m := newMockSetup(t, &caps)
	m.expectLock()
	pinned := artifact.PinnedReference{Ref: exampleRef, ResolvedDigest: prior.Operation.RecordDigest, RecordDigest: prior.Operation.RecordDigest}
	record := prior.Record
	gomock.InOrder(
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil),
		m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned, Record: &record}, nil),
		m.repo.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errTransport),
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil),
	)
	request := m.e.publishRequest("dst", m.e.source("changed"))
	request.Force = true
	_, err := m.svc.Publish(context.Background(), request)
	require.ErrorIs(t, err, errTransport)
	require.NotErrorIs(t, err, errUtils.ErrArtifactPublicationAmbiguous, "the reference provably still holds the prior state")
}

func TestPublish_ReadFailuresOfTheCurrentStateAreReported(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	t.Run("resolve fails", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.expectLock()
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(artifact.PinnedReference{}, errTransport)
		_, err := m.svc.Publish(context.Background(), m.e.publishRequest("dst", m.e.source("x")))
		require.ErrorIs(t, err, errTransport)
	})
	t.Run("inspect fails", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.expectLock()
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(artifact.PinnedReference{ResolvedDigest: "sha256:abc"}, nil)
		m.repo.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(nil, errTransport)
		_, err := m.svc.Publish(context.Background(), m.e.publishRequest("dst", m.e.source("x")))
		require.ErrorIs(t, err, errTransport)
	})
	t.Run("open fails", func(t *testing.T) {
		e := newEnv(t, envConfig{})
		ctrl := gomock.NewController(t)
		repos := service.NewMockRepositories(ctrl)
		repos.EXPECT().Describe("dst").Return(service.RepositoryInfo{Alias: "dst", Capabilities: caps, Packaging: artifact.PackagingNone}, nil)
		repos.EXPECT().Open(gomock.Any(), "dst").Return(nil, errTransport)
		svc, err := service.New(repos, nil, service.WithStagingRoot(e.staging))
		require.NoError(t, err)
		_, err = svc.Publish(context.Background(), e.publishRequest("dst", e.source("x")))
		require.ErrorIs(t, err, errTransport)
		e.assertStagingClean()
	})
}

func TestPublish_ExistingForeignGraphIsNotComparableAndNeedsForce(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	m := newMockSetup(t, &caps)
	m.expectLock()
	pinned := artifact.PinnedReference{Ref: exampleRef, ResolvedDigest: digest.FromString("graph")}
	m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil)
	m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned}, nil)
	_, err := m.svc.Publish(context.Background(), m.e.publishRequest("dst", m.e.source("x")))
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	assert.Contains(t, err.Error(), "no portable record")
}

func TestPull_Edges(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	record, recordDigest := sealedRecord(t)
	pinned := artifact.PinnedReference{Ref: exampleRef, ResolvedDigest: recordDigest, RecordDigest: recordDigest}

	t.Run("native graph cannot be materialized", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil)
		m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned}, nil)
		_, err := m.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: out(t)})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	})
	t.Run("content bound to another record is rejected", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil)
		m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned, Record: &record}, nil)
		other := record
		other.Version = "9.9.9"
		require.NoError(t, other.Seal())
		m.repo.EXPECT().Read(gomock.Any(), pinned).Return(&artifact.Content{Record: &other}, nil)
		dest := out(t)
		_, err := m.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: dest})
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		assert.True(t, missing(dest))
	})
	t.Run("read fails", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil)
		m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned, Record: &record}, nil)
		m.repo.EXPECT().Read(gomock.Any(), pinned).Return(nil, errTransport)
		_, err := m.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: out(t)})
		require.ErrorIs(t, err, errTransport)
	})
	t.Run("inspect fails", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil)
		m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(nil, errTransport)
		_, err := m.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: out(t)})
		require.ErrorIs(t, err, errTransport)
	})
}

func TestShowDocument_TamperedDocumentFailsItsDescriptor(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	record, recordDigest := sealedRecord(t)
	pinned := artifact.PinnedReference{Ref: exampleRef, ResolvedDigest: recordDigest, RecordDigest: recordDigest}
	want := []byte("genuine readme")
	descriptor := artifact.Document{Role: artifact.DocumentReadme, SourcePath: "README.md", MediaType: artifact.DocumentMediaType, Size: int64(len(want)), Digest: digest.FromBytes(want)}
	_ = record

	for name, content := range map[string][]byte{"altered bytes": []byte("tampered readme"), "oversize": bytes.Repeat([]byte("x"), int(artifact.MaxDocumentSize)+1)} {
		t.Run(name, func(t *testing.T) {
			m := newMockSetup(t, &caps)
			m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil)
			m.repo.EXPECT().OpenDocument(gomock.Any(), pinned, artifact.DocumentReadme).Return(&artifact.DocumentStream{Document: descriptor, Reader: io.NopCloser(bytes.NewReader(content))}, nil)
			_, err := m.svc.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: "dst", Ref: exampleRef, Role: artifact.DocumentReadme})
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrArtifactIntegrityFailed) || errors.Is(err, errUtils.ErrArtifactTooLarge), "%v", err)
		})
	}
	t.Run("open fails", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil)
		m.repo.EXPECT().OpenDocument(gomock.Any(), pinned, artifact.DocumentReadme).Return(nil, errTransport)
		_, err := m.svc.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: "dst", Ref: exampleRef, Role: artifact.DocumentReadme})
		require.ErrorIs(t, err, errTransport)
	})
}

func TestList_PaginationGuards(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	t.Run("non-advancing cursor", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.repo.EXPECT().List(gomock.Any(), gomock.Any()).Return(&artifact.ListPage{NextCursor: "stuck"}, nil).Times(2)
		_, err := m.svc.List(context.Background(), artifact.ListRequest{Repositories: []string{"dst"}})
		require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
	})
	t.Run("list error", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		m.repo.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errTransport)
		_, err := m.svc.List(context.Background(), artifact.ListRequest{Repositories: []string{"dst"}})
		require.ErrorIs(t, err, errTransport)
	})
	t.Run("canceled", func(t *testing.T) {
		m := newMockSetup(t, &caps)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := m.svc.List(ctx, artifact.ListRequest{Repositories: []string{"dst"}})
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("write-only repository", func(t *testing.T) {
		writeOnly := caps
		writeOnly.Read = false
		m := newMockSetup(t, &writeOnly)
		_, err := m.svc.List(context.Background(), artifact.ListRequest{Repositories: []string{"dst"}})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	})
}

// graphSource is a source repository whose artifact is a foreign OCI graph without a Record.
func graphSource(ctrl *gomock.Controller, pinned *artifact.PinnedReference) *artifact.MockRepository {
	repo := artifact.NewMockRepository(ctrl)
	repo.EXPECT().Capabilities().Return(artifacttest.DefaultCapabilities()).AnyTimes()
	repo.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(*pinned, nil).AnyTimes()
	root := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("graph-root"), Size: 10}
	repo.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(&artifact.Inspection{
		Pinned:         *pinned,
		Representation: artifact.Representation{Packaging: artifact.PackagingOCIGraph, OriginalRoot: &root},
	}, nil).AnyTimes()
	return repo
}

func TestCopy_ForeignGraphNeedsANativeTransfer(t *testing.T) {
	pinned := artifact.PinnedReference{Ref: exampleRef, ResolvedDigest: digest.FromString("graph-root")}
	build := func(t *testing.T, dst artifact.Repository) *service.Service {
		ctrl := gomock.NewController(t)
		e := newEnv(t, envConfig{})
		src := graphSource(ctrl, &pinned)
		repos := &service.StaticRepositories{
			Repositories: map[string]artifact.Repository{"src": src, "dst": dst},
			Info: map[string]service.RepositoryInfo{
				"src": {Capabilities: artifacttest.DefaultCapabilities()},
				"dst": {Capabilities: artifacttest.DefaultCapabilities(), Packaging: artifact.PackagingNone},
			},
		}
		svc, err := service.New(repos, nil, service.WithStagingRoot(e.staging))
		require.NoError(t, err)
		return svc
	}

	t.Run("no native transfer is a capability error", func(t *testing.T) {
		svc := build(t, artifacttest.New(artifacttest.Options{}))
		_, err := svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	})
	t.Run("native transfer carries the graph", func(t *testing.T) {
		native := &nativeRepository{Repository: artifacttest.New(artifacttest.Options{}), compat: true}
		svc := build(t, native)
		result, err := svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
		require.NoError(t, err)
		assert.Equal(t, 1, native.copies)
		assert.Equal(t, "tools/example:1.0.0", result.Ref)
	})
	t.Run("a digest-only graph has no tag to publish under", func(t *testing.T) {
		native := &nativeRepository{Repository: artifacttest.New(artifacttest.Options{}), compat: true}
		svc := build(t, native)
		digestRef := artifact.Ref{Name: "tools/example", Digest: digest.FromString("graph-root")}
		_, err := svc.Copy(context.Background(), artifact.CopyRequest{Ref: digestRef, From: "src", To: "dst"})
		require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
	})
}

func TestCopy_EvidenceCarryFailuresAreReported(t *testing.T) {
	e := copyEnv(t, artifact.PackagingNone, artifact.PackagingNone, service.WithSigner(releaseSigner))
	published := e.publish("src", e.source("hello"))
	_, err := e.svc.Sign(context.Background(), artifact.SignRequest{Repository: "src", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.NoError(t, err)
	e.static.Repositories["src"] = failingEvidenceReader{Repository: e.repos["src"]}

	result, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.NoError(t, err)
	var warned bool
	for _, diagnostic := range result.Diagnostics {
		warned = warned || diagnostic.Code == "evidence-read-failed"
	}
	assert.True(t, warned)
}

type failingEvidenceReader struct{ artifact.Repository }

func (failingEvidenceReader) ReadEvidence(context.Context, *artifact.PinnedReference) ([]artifact.EvidenceBlob, error) {
	return nil, errTransport
}

func TestSign_RejectsADigestThatNamesNoIdentityOfTheResolvedArtifact(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	record, recordDigest := sealedRecord(t)
	// An OCI-style repository resolves the digest to a manifest that is not the Record digest.
	manifest := digest.FromString("manifest")
	pinned := artifact.PinnedReference{Ref: artifact.Ref{Name: "tools/example", Digest: manifest}, ResolvedDigest: manifest, RecordDigest: recordDigest}
	m := newMockSetup(t, &caps, service.WithSigner(releaseSigner))
	m.repo.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(pinned, nil)
	m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned, Record: &record}, nil)
	_, err := m.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: digest.FromString("unrelated")}})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
}

func TestCopy_RepackFailureCleansUpStaging(t *testing.T) {
	repacker := service.NewMockRepacker(gomock.NewController(t))
	repacker.EXPECT().Repack(gomock.Any(), gomock.Any()).Return(nil, errTransport)
	e := copyEnv(t, artifact.PackagingNone, artifact.PackagingTgz, service.WithRepacker(repacker))
	e.publish("src", e.source("hello"))
	_, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.ErrorIs(t, err, errTransport)
	e.assertStagingClean()
	_, found := e.repos["dst"].RecordDigestOf(exampleRef)
	assert.False(t, found)
}

func TestPublish_DefaultBackoffSleepAndCancellationDuringBackoff(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.repos["dst"].InjectFailure(artifacttest.FailBeforePayload, &temporaryError{"throttled"}, 1)
	svc, err := service.New(e.static, nil, service.WithRetry(2, time.Millisecond), service.WithStagingRoot(e.staging))
	require.NoError(t, err)
	result, err := svc.Publish(context.Background(), e.publishRequest("dst", e.source("hello")))
	require.NoError(t, err, "the default context-aware sleep elapses and the retry succeeds")
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)

	other := newEnv(t, envConfig{})
	other.repos["dst"].InjectFailure(artifacttest.FailBeforePayload, &temporaryError{"throttled"}, 5)
	ctx, cancel := context.WithCancel(context.Background())
	svc, err = service.New(other.static, nil, service.WithRetry(3, time.Hour), service.WithStagingRoot(other.staging),
		service.WithSleep(func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }))
	require.NoError(t, err)
	_, err = svc.Publish(ctx, other.publishRequest("dst", other.source("hello")))
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, other.repos["dst"].Calls("Publish"))

	real, err := service.New(other.static, nil, service.WithRetry(2, time.Hour), service.WithStagingRoot(other.staging))
	require.NoError(t, err)
	other.repos["dst"].InjectFailure(artifacttest.FailBeforePayload, &temporaryError{"throttled"}, 5)
	cancelled, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	_, err = real.Publish(cancelled, other.publishRequest("dst", other.source("hello")))
	require.ErrorIs(t, err, context.DeadlineExceeded, "an hour-long default backoff is interrupted by the context")
}
