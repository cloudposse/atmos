package oci

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/local"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newLocalDir builds a local/dir repository that shares the verifier.
func newLocalDir(t *testing.T, verifier artifact.Verifier) artifact.Repository {
	t.Helper()
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, local.RegisterWith(registry, local.WithVerifier(verifier)))
	repo, err := registry.New(context.Background(), "local", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()},
	})
	require.NoError(t, err)
	return repo
}

// A Record signed in a local repository is restored into an OCI package of the same Record and
// verifies there: the evidence follows the portable Record, not the representation.
func TestRecordSignatureSurvivesLocalToOCIRestore(t *testing.T) {
	ctx := context.Background()
	tr := newTrust(t)
	verifier := tr.verifier()
	localRepo := newLocalDir(t, verifier)
	localPrepared := prepare(t, publishSpec{content: "signed", packaging: artifact.PackagingNone})
	_, err := localRepo.Publish(ctx, localPrepared, artifact.PublishOptions{})
	require.NoError(t, err)
	localPinned, err := localRepo.Resolve(ctx, artifact.Ref{Name: appName, Version: appVersion})
	require.NoError(t, err)
	inspection, err := localRepo.Inspect(ctx, localPinned)
	require.NoError(t, err)
	subjects := subjectsOfRecord(t, inspection)
	_, err = localRepo.Sign(ctx, subjects, tr.keyless, artifact.SignOptions{})
	require.NoError(t, err)

	reader, ok := localRepo.(artifact.EvidenceReader)
	require.True(t, ok)
	blobs, err := reader.ReadEvidence(ctx, &localPinned)
	require.NoError(t, err)
	require.NotEmpty(t, blobs)

	withVerifier(t, verifier, func(t *testing.T, env *signedEnv) {
		pinned := mustResolveRef(t, env.repo, artifact.Ref{Name: appName, Version: appVersion})
		// Same Record bytes under a different packaging and envelope.
		assert.Equal(t, localPinned.RecordDigest, pinned.RecordDigest)
		assert.NotEqual(t, localPinned.ResolvedDigest, pinned.ResolvedDigest, "the OCI envelope digest is its own")
		env.requireRejected(t, env.record, identityPolicy(trustedIdentity, trustedIssuer))

		stored, err := env.repo.WriteEvidence(ctx, &pinned, blobs, nil)
		require.NoError(t, err)
		assert.Len(t, stored, len(blobs))
		env.requireVerified(t, env.record, identityPolicy(trustedIdentity, trustedIssuer))

		again, err := env.repo.WriteEvidence(ctx, &pinned, blobs, nil)
		require.NoError(t, err)
		assert.Len(t, again, len(blobs))
		read, err := env.repo.ReadEvidence(ctx, &pinned)
		require.NoError(t, err)
		assert.Len(t, read, len(blobs), "importing the same evidence twice does not duplicate it")

		wrong := append([]artifact.EvidenceBlob(nil), blobs...)
		wrong[0].Evidence.Subject.Digest = pinned.ResolvedDigest
		_, err = env.repo.WriteEvidence(ctx, &pinned, wrong, nil)
		require.Error(t, err, "evidence bound to another subject is refused")
		wrong[0] = blobs[0]
		wrong[0].Data = []byte("changed bytes")
		_, err = env.repo.WriteEvidence(ctx, &pinned, wrong, nil)
		require.Error(t, err, "evidence bytes must match their descriptor")
	})
}

func subjectsOfRecord(t *testing.T, inspection *artifact.Inspection) artifact.SignatureSubject {
	t.Helper()
	canonical, err := inspection.Record.Canonical()
	require.NoError(t, err)
	recordDigest, err := inspection.Record.RecordDigest()
	require.NoError(t, err)
	return artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: recordDigest, MediaType: artifact.MediaTypeConfig, Size: int64(len(canonical))}
}

// Through the service, repacking from a local directory keeps Record and content identity but
// builds a new OCI envelope; a native OCI-to-OCI copy keeps the envelope digest too.
func TestServiceCopyIdentityLocalToOCIAndOCIToOCI(t *testing.T) {
	ctx := context.Background()
	localRepo := newLocalDir(t, nil)
	registryRepo := newRegistryRepo(t, newRegistry(t, nil))
	layoutRepo := newLayoutRepo(t)
	svc, err := service.New(&service.StaticRepositories{Repositories: map[string]artifact.Repository{"local": localRepo, "reg": registryRepo, "lay": layoutRepo}}, nil,
		service.WithImmutabilityPolicy(PluginImmutability()), service.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)

	source := t.TempDir()
	writeSource(t, source)
	_, err = svc.PublishDetailed(ctx, artifact.PublishRequest{Repository: "local", Source: source, Name: appName, Version: appVersion})
	require.NoError(t, err)
	ref := artifact.Ref{Name: appName, Version: appVersion}

	_, err = svc.Copy(ctx, artifact.CopyRequest{Ref: ref, From: "local", To: "reg"})
	require.NoError(t, err)
	fromLocal, err := localRepo.Resolve(ctx, ref)
	require.NoError(t, err)
	inRegistry := mustResolveRef(t, registryRepo, ref)
	assert.Equal(t, fromLocal.RecordDigest, inRegistry.RecordDigest, "Record identity survives repackaging")
	inspection, err := registryRepo.Inspect(ctx, inRegistry)
	require.NoError(t, err)
	assert.Equal(t, fromLocal.ContentDigest, inspection.Pinned.ContentDigest, "content identity survives repackaging")
	assert.NotEqual(t, fromLocal.ResolvedDigest, inRegistry.ResolvedDigest, "the envelope digest is the OCI manifest, not the Record")

	_, err = svc.Copy(ctx, artifact.CopyRequest{Ref: ref, From: "reg", To: "lay"})
	require.NoError(t, err)
	inLayout := mustResolveRef(t, layoutRepo, ref)
	assert.Equal(t, inRegistry.ResolvedDigest, inLayout.ResolvedDigest, "a native OCI copy keeps the envelope digest")
	assert.Equal(t, inRegistry.RecordDigest, inLayout.RecordDigest)

	again, err := svc.Copy(ctx, artifact.CopyRequest{Ref: ref, From: "reg", To: "lay"})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeUnchanged, again.Outcome)
}

// A layout is usable with no network at all: every operation works with a transport that fails
// every request, because a layout never opens one.
func TestLayoutOperatesOfflineWithATransportThatFailsEveryRequest(t *testing.T) {
	ctx := context.Background()
	offline := &recorder{}
	offline.failAll(context.DeadlineExceeded)
	failing := newRegistry(t, nil)
	failing.recorder = offline
	tr := newTrust(t)
	repo := newLayoutRepo(t, WithTransport(failing.transport), WithVerifier(tr.verifier()))

	result := publish(t, repo, publishSpec{content: "offline"})
	pinned := mustResolveRef(t, repo, artifact.Ref{Name: appName, Version: appVersion})
	inspection, err := repo.Inspect(ctx, pinned)
	require.NoError(t, err)
	_, err = repo.Sign(ctx, subjectsOfRecord(t, inspection), tr.keyless, artifact.SignOptions{})
	require.NoError(t, err)
	_, err = repo.Verify(ctx, subjectsOfRecord(t, inspection), identityPolicy(trustedIdentity, trustedIssuer))
	require.NoError(t, err)
	assert.Equal(t, []string{"team/app:1.0.0"}, listAll(t, repo, artifact.ListQuery{}))
	content, err := repo.Read(ctx, pinned)
	require.NoError(t, err)
	require.NoError(t, content.Close())

	carrier := newLayoutRepo(t, WithTransport(failing.transport))
	_, err = carrier.CopyFrom(ctx, repo, pinned, artifact.CopyFromOptions{})
	require.NoError(t, err)
	_, err = repo.Delete(ctx, pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: artifact.DeleteReference})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Location)
	assert.Zero(t, offline.total(), "no network request was attempted")
}
