//nolint:gocritic // OCI descriptors are passed by value, mirroring the ORAS content contracts.
package oci

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
)

// flakyTags wraps a registry transport so the tag write reports failure; when lands is true the
// write actually reaches the registry first, like a response lost after a server-side commit.
type flakyTags struct {
	atmosoci.Transport
	lands bool
}

func (f flakyTags) Repository(ctx context.Context, repository atmosoci.Repository) (atmosoci.RepositoryStore, error) {
	inner, err := f.Transport.Repository(ctx, repository)
	if err != nil {
		return nil, err
	}
	return flakyStore{RepositoryStore: inner, lands: f.lands}, nil
}

type flakyStore struct {
	atmosoci.RepositoryStore
	lands bool
}

func (f flakyStore) PushReference(ctx context.Context, expected ocispec.Descriptor, content io.Reader, reference string) error {
	if f.lands {
		if err := f.RepositoryStore.PushReference(ctx, expected, content, reference); err != nil {
			return err
		}
	}
	return errors.New("connection reset by peer")
}

func TestUnknownTagWriteOutcomeIsReconciledByResolving(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t, nil)

	landed, err := NewRepository(ctx, "landed", registryConfig(reg.server.Host), WithTransport(flakyTags{reg.transport, true}), WithLockDir(t.TempDir()))
	require.NoError(t, err)
	result, err := landed.Publish(ctx, prepare(t, publishSpec{content: "lost response"}), artifact.PublishOptions{})
	require.NoError(t, err, "the tag is at the new manifest, so the publication completed")
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)

	lost, err := NewRepository(ctx, "lost", registryConfig(reg.server.Host), WithTransport(flakyTags{reg.transport, false}), WithLockDir(t.TempDir()))
	require.NoError(t, err)
	_, err = lost.Publish(ctx, prepare(t, publishSpec{name: "other/app", content: "never landed"}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
	_, err = newRegistryRepo(t, reg).Resolve(ctx, artifact.Ref{Name: "other/app", Version: appVersion})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "a failed write is not advertised as published")
}

func TestStoredEvidenceIsValidatedBeforeItIsWritten(t *testing.T) {
	tr := newTrust(t)
	withVerifier(t, tr.verifier(), func(t *testing.T, env *signedEnv) {
		valid, err := tr.keyless.Sign(context.Background(), env.record)
		require.NoError(t, err)
		tests := map[string]*artifact.EvidencePayload{
			"nil":            nil,
			"unknown format": {Format: "cosign-tag", MediaType: bundleMediaTypePrefix + "v0.3+json", Data: valid.Data},
			"not a bundle":   {Format: valid.Format, MediaType: "application/json", Data: valid.Data},
			"empty":          {Format: valid.Format, MediaType: valid.MediaType},
			"over the bound": {Format: valid.Format, MediaType: valid.MediaType, Data: make([]byte, artifact.MaxEvidenceSize+1)},
		}
		for name, payload := range tests {
			t.Run(name, func(t *testing.T) {
				require.Error(t, env.repo.checkPayload(payload))
			})
		}
		require.NoError(t, env.repo.checkPayload(valid))
	})
}

func TestSigningNeedsAVerifierAndAKnownSubject(t *testing.T) {
	tr := newTrust(t)
	plain := newLayoutRepo(t)
	result := publish(t, plain, publishSpec{content: "no verifier"})
	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: result.Pinned.RecordDigest, MediaType: artifact.MediaTypeConfig}
	_, err := plain.Sign(context.Background(), subject, tr.keyless, artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	_, err = plain.Verify(context.Background(), subject, identityPolicy(trustedIdentity, trustedIssuer))
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)

	withVerifier(t, tr.verifier(), func(t *testing.T, env *signedEnv) {
		_, err := env.repo.Sign(context.Background(), env.record, nil, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
		wrongSize := env.record
		wrongSize.Size++
		_, err = env.repo.Sign(context.Background(), wrongSize, tr.keyless, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
		bad := env.record
		bad.Kind = "unknown"
		_, err = env.repo.Sign(context.Background(), bad, tr.keyless, artifact.SignOptions{})
		require.Error(t, err)
	})
}

func TestMissingDocumentLayerAndHostGuardLeases(t *testing.T) {
	repo := newLayoutRepo(t)
	publish(t, repo, publishSpec{content: "bare", noDocs: true})
	pinned := mustResolveRef(t, repo, artifact.Ref{Name: appName, Version: appVersion})
	for _, role := range []artifact.DocumentRole{artifact.DocumentReadme, artifact.DocumentChangelog} {
		_, err := repo.OpenDocument(context.Background(), pinned, role)
		require.ErrorIs(t, err, errUtils.ErrArtifactDocumentMissing, string(role))
	}

	handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: "x:1", Owner: "a"})
	require.NoError(t, err)
	require.ErrorIs(t, handle.Renew(context.Background(), time.Minute), errUtils.ErrArtifactUnsupportedCapability)
	require.NoError(t, handle.Release(context.Background()))
}

func TestOptionsApplyDefaultsAndOverrides(t *testing.T) {
	defaults := applySettings(nil)
	assert.Equal(t, artifact.DefaultConcurrency, defaults.concurrency)
	assert.Equal(t, defaultPageSize, defaults.pageSize)
	assert.NotEmpty(t, defaults.owner)
	custom := applySettings([]Option{WithConcurrency(7), WithPageSize(3), WithOwner("me"), nil})
	assert.Equal(t, 7, custom.concurrency)
	assert.Equal(t, 3, custom.pageSize)
	assert.Equal(t, "me", custom.owner)

	repo := newLayoutRepo(t, WithPageSize(1))
	publish(t, repo, publishSpec{version: "1.0.0", content: "a"})
	publish(t, repo, publishSpec{version: "2.0.0", content: "b"})
	page, err := repo.List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	assert.Len(t, page.Entries, 1, "the configured page size applies when the query has none")
	assert.NotEmpty(t, page.NextCursor)
}

func TestReaderClosedBeforeReadingFetchesNothing(t *testing.T) {
	reg := newRegistry(t, nil)
	repo := newRegistryRepo(t, reg)
	result := publish(t, repo, publishSpec{content: "unread"})
	pinned := mustResolveRef(t, repo, artifact.Ref{Name: appName, Version: appVersion})
	reg.recorder.reset()
	content, err := repo.Read(context.Background(), pinned)
	require.NoError(t, err)
	require.NoError(t, content.Close())
	assert.Zero(t, reg.recorder.count("GET", payloadDigest(t, result).Encoded()))
}
