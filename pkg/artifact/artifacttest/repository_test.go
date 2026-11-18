package artifacttest_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
)

const (
	testName  = "team/app"
	testPerm  = 0o644
	aliasName = "team/alias"
)

// publishOne publishes a single-file artifact and returns its pinned reference.
func publishOne(t *testing.T, repo *artifacttest.Repository) artifact.PinnedReference {
	t.Helper()
	name, version := testName, "1.0.0"
	src := t.TempDir()
	fixtures.WriteFile(t, src, "data.txt", "content of "+name+"@"+version, testPerm)
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: name, Version: version, Packaging: artifact.PackagingTar,
		Metadata:    artifact.Metadata{Labels: map[string]string{"suite": "isolation"}},
		OperationID: "op-" + name + "-" + version, PreparedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.NoError(t, err)
	pinned, err := repo.Resolve(context.Background(), artifact.Ref{Name: name, Version: version})
	require.NoError(t, err)
	return pinned
}

// TestRepository_CapabilitySlicesAreIsolated proves the repository's guarantees are static: the
// slices a caller passes in or receives back never share storage with the repository.
func TestRepository_CapabilitySlicesAreIsolated(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *artifact.Capabilities)
		read   func(c artifact.Capabilities) any
		want   any
	}{
		{
			name:   "packagings",
			mutate: func(c *artifact.Capabilities) { c.Packagings[0] = artifact.PackagingZip },
			read:   func(c artifact.Capabilities) any { return c.Packagings },
			want:   artifacttest.DefaultCapabilities().Packagings,
		},
		{
			name:   "subject kinds",
			mutate: func(c *artifact.Capabilities) { c.SubjectKinds[0] = "other" },
			read:   func(c artifact.Capabilities) any { return c.SubjectKinds },
			want:   artifacttest.DefaultCapabilities().SubjectKinds,
		},
		{
			name:   "evidence formats",
			mutate: func(c *artifact.Capabilities) { c.EvidenceFormats[0] = "other" },
			read:   func(c artifact.Capabilities) any { return c.EvidenceFormats },
			want:   artifacttest.DefaultCapabilities().EvidenceFormats,
		},
		{
			name:   "lock scopes",
			mutate: func(c *artifact.Capabilities) { c.LockScopes[0] = artifact.LockScopeRepository },
			read:   func(c artifact.Capabilities) any { return c.LockScopes },
			want:   artifacttest.DefaultCapabilities().LockScopes,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/returned value", func(t *testing.T) {
			repo := artifacttest.New(artifacttest.Options{})
			returned := repo.Capabilities()
			tt.mutate(&returned)
			assert.Equal(t, tt.want, tt.read(repo.Capabilities()), "mutating the returned slices must not change the repository")
		})
		t.Run(tt.name+"/accepted option", func(t *testing.T) {
			supplied := artifacttest.DefaultCapabilities()
			repo := artifacttest.New(artifacttest.Options{Capabilities: &supplied})
			tt.mutate(&supplied)
			assert.Equal(t, tt.want, tt.read(repo.Capabilities()), "mutating the supplied option must not change the repository")
		})
	}
}

func TestRepository_MutatedCapabilitiesDoNotChangeLockGuarantees(t *testing.T) {
	repo := artifacttest.New(artifacttest.Options{})
	returned := repo.Capabilities()
	returned.LockScopes[0] = artifact.LockScopeRepository

	_, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: "r", Owner: "o", Required: artifact.LockScopeRepository})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a host-only repository never grants repository scope because a caller edited its copy")
}

func TestRepository_DeleteScope(t *testing.T) {
	aliasRef := artifact.Ref{Name: aliasName, Version: "latest"}
	primary := artifact.Ref{Name: testName, Version: "1.0.0"}

	tests := []struct {
		name  string
		caps  artifact.DeleteSemantics
		scope artifact.DeleteSemantics
		pin   artifact.Ref

		wantErr         error
		wantDeleted     []string
		wantAffected    []artifact.Ref
		wantRecordAlive bool
		wantPrimary     bool
		wantAlias       bool
		wantEvidence    bool
	}{
		{
			name: "object scope removes every alias and reports each location once", caps: artifact.DeleteObject, scope: artifact.DeleteObject, pin: primary,
			wantDeleted:  []string{"mem://memory/team/app/1.0.0", "mem://memory/team/alias/latest"},
			wantAffected: []artifact.Ref{aliasRef},
		},
		{
			name: "object scope through the alias reports the other reference as affected", caps: artifact.DeleteObject, scope: artifact.DeleteObject, pin: aliasRef,
			wantDeleted:  []string{"mem://memory/team/app/1.0.0", "mem://memory/team/alias/latest"},
			wantAffected: []artifact.Ref{primary},
		},
		{
			name: "manifest scope affects every reference", caps: artifact.DeleteManifest, scope: artifact.DeleteManifest, pin: primary,
			wantDeleted:  []string{"mem://memory/team/app/1.0.0", "mem://memory/team/alias/latest"},
			wantAffected: []artifact.Ref{aliasRef},
		},
		{
			name: "reference scope removes only the pinned reference and keeps shared data", caps: artifact.DeleteReference, scope: artifact.DeleteReference, pin: primary,
			wantDeleted: []string{"mem://memory/team/app/1.0.0"}, wantRecordAlive: true, wantAlias: true, wantEvidence: true,
		},
		{
			name: "reference scope through the alias leaves the primary reference", caps: artifact.DeleteReference, scope: artifact.DeleteReference, pin: aliasRef,
			wantDeleted: []string{"mem://memory/team/alias/latest"}, wantRecordAlive: true, wantPrimary: true, wantEvidence: true,
		},
		{name: "an empty scope is rejected", caps: artifact.DeleteObject, scope: "", pin: primary, wantErr: errUtils.ErrArtifactUnsupportedCapability, wantRecordAlive: true, wantPrimary: true, wantAlias: true, wantEvidence: true},
		{name: "an unknown scope is rejected", caps: artifact.DeleteObject, scope: "everything", pin: primary, wantErr: errUtils.ErrArtifactUnsupportedCapability, wantRecordAlive: true, wantPrimary: true, wantAlias: true, wantEvidence: true},
		{name: "a scope the repository does not advertise is rejected", caps: artifact.DeleteReference, scope: artifact.DeleteObject, pin: primary, wantErr: errUtils.ErrArtifactUnsupportedCapability, wantRecordAlive: true, wantPrimary: true, wantAlias: true, wantEvidence: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := artifacttest.DefaultCapabilities()
			caps.Delete = tt.caps
			repo := artifacttest.New(artifacttest.Options{Capabilities: &caps})
			pinned := publishOne(t, repo)
			require.True(t, repo.MoveRef(aliasRef, pinned.RecordDigest))
			signEvidence(t, repo, &pinned)

			target, err := repo.Resolve(context.Background(), tt.pin)
			require.NoError(t, err)
			result, err := repo.Delete(context.Background(), target, artifact.DeleteOptions{ExpectedCurrent: &target, Scope: tt.scope})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, tt.wantDeleted, result.Deleted, "each removed reference is reported once, by its own location")
				assert.Len(t, result.Deleted, len(tt.wantDeleted))
				assert.ElementsMatch(t, tt.wantAffected, result.Affected)
				assert.Equal(t, pinned.RecordDigest, result.Digest)
			}

			_, primaryAlive := repo.RecordDigestOf(primary)
			_, aliasAlive := repo.RecordDigestOf(aliasRef)
			assert.Equal(t, tt.wantPrimary, primaryAlive, "primary reference")
			assert.Equal(t, tt.wantAlias, aliasAlive, "alias reference")
			assert.Equal(t, tt.wantRecordAlive, recordAlive(repo, pinned.RecordDigest), "shared record")
			assert.Equal(t, tt.wantEvidence, evidenceAlive(t, repo, &pinned), "shared evidence")
		})
	}
}

func TestRepository_DeleteReferenceScopeRemovesRecordWithItsLastReference(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.Delete = artifact.DeleteReference
	repo := artifacttest.New(artifacttest.Options{Capabilities: &caps})
	pinned := publishOne(t, repo)
	signEvidence(t, repo, &pinned)

	result, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: artifact.DeleteReference})
	require.NoError(t, err)
	assert.Equal(t, []string{"mem://memory/team/app/1.0.0"}, result.Deleted)
	assert.Empty(t, result.Affected)
	assert.False(t, recordAlive(repo, pinned.RecordDigest), "an unreferenced record is no longer retained")
	assert.False(t, evidenceAlive(t, repo, &pinned))
}

func TestRepository_DeleteReferenceScopeNeedsAMutableReference(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.Delete = artifact.DeleteReference
	repo := artifacttest.New(artifacttest.Options{Capabilities: &caps})
	pinned := publishOne(t, repo)
	byDigest := pinned
	byDigest.Ref = artifact.Ref{Name: testName, Digest: pinned.RecordDigest}

	_, err := repo.Delete(context.Background(), byDigest, artifact.DeleteOptions{Scope: artifact.DeleteReference})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
	_, alive := repo.RecordDigestOf(artifact.Ref{Name: testName, Version: "1.0.0"})
	assert.True(t, alive, "a rejected delete changes nothing")
}

func recordAlive(repo *artifacttest.Repository, recordDigest digest.Digest) bool {
	_, err := repo.Inspect(context.Background(), artifact.PinnedReference{RecordDigest: recordDigest, ResolvedDigest: recordDigest})
	return err == nil
}

func evidenceAlive(t *testing.T, repo *artifacttest.Repository, pinned *artifact.PinnedReference) bool {
	t.Helper()
	blobs, err := repo.ReadEvidence(context.Background(), pinned)
	require.NoError(t, err)
	return len(blobs) > 0
}

func signEvidence(t *testing.T, repo *artifacttest.Repository, pinned *artifact.PinnedReference) {
	t.Helper()
	inspection, err := repo.Inspect(context.Background(), *pinned)
	require.NoError(t, err)
	canonical, err := inspection.Record.Canonical()
	require.NoError(t, err)
	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digest.FromBytes(canonical), MediaType: artifact.MediaTypeConfig, Size: int64(len(canonical))}
	signer := &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}
	_, err = repo.Sign(context.Background(), subject, signer, artifact.SignOptions{OperationID: "sign-op"})
	require.NoError(t, err)
}

// TestRepository_StoredStateIsIsolatedFromCallers proves records, labels, and descriptors handed
// to or returned from the repository never alias its stored state, in either direction.
func TestRepository_StoredStateIsIsolatedFromCallers(t *testing.T) {
	ctx := context.Background()
	ref := artifact.Ref{Name: testName, Version: "1.0.0"}
	tests := []struct {
		name   string
		tamper func(t *testing.T, repo *artifacttest.Repository, pinned artifact.PinnedReference)
	}{
		{"inspect record labels", func(t *testing.T, repo *artifacttest.Repository, pinned artifact.PinnedReference) {
			inspection, err := repo.Inspect(ctx, pinned)
			require.NoError(t, err)
			inspection.Record.Metadata.Labels["suite"] = "tampered"
			inspection.Record.Inventory[0].Path = "tampered"
			inspection.Record.Documents = append(inspection.Record.Documents[:0], artifact.Document{Role: "tampered"})
		}},
		{"read record labels", func(t *testing.T, repo *artifacttest.Repository, pinned artifact.PinnedReference) {
			content, err := repo.Read(ctx, pinned)
			require.NoError(t, err)
			content.Record.Metadata.Labels["suite"] = "tampered"
			content.Representation.Payload[0].Digest = digest.FromString("tampered")
		}},
		{"list entry labels", func(t *testing.T, repo *artifacttest.Repository, _ artifact.PinnedReference) {
			page, err := repo.List(ctx, artifact.ListQuery{})
			require.NoError(t, err)
			require.NotEmpty(t, page.Entries)
			page.Entries[0].Labels["suite"] = "tampered"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := artifacttest.New(artifacttest.Options{})
			pinned := publishOne(t, repo)
			before, err := repo.Inspect(ctx, pinned)
			require.NoError(t, err)
			wantPath := before.Record.Inventory[0].Path
			wantDocs := len(before.Record.Documents)
			wantPayload := slices.Clone(before.Representation.Payload)

			tt.tamper(t, repo, pinned)

			after, err := repo.Inspect(ctx, pinned)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"suite": "isolation"}, after.Record.Metadata.Labels)
			assert.Equal(t, wantPath, after.Record.Inventory[0].Path)
			assert.Len(t, after.Record.Documents, wantDocs)
			assert.Equal(t, wantPayload, after.Representation.Payload)
			_, err = repo.Resolve(ctx, ref)
			require.NoError(t, err)
		})
	}
}

func TestRepository_PublishCopiesThePreparedRecord(t *testing.T) {
	ctx := context.Background()
	repo := artifacttest.New(artifacttest.Options{})
	src := t.TempDir()
	fixtures.WriteFile(t, src, "data.txt", "payload", testPerm)
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(ctx, artifact.PrepareRequest{
		Source: src, Name: testName, Version: "1.0.0", Packaging: artifact.PackagingTar,
		Metadata:    artifact.Metadata{Labels: map[string]string{"suite": "isolation"}},
		OperationID: "op", PreparedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	_, err = repo.Publish(ctx, prepared, artifact.PublishOptions{})
	require.NoError(t, err)

	prepared.Record.Metadata.Labels["suite"] = "tampered"
	prepared.Record.Inventory[0].Path = "tampered"

	pinned, err := repo.Resolve(ctx, artifact.Ref{Name: testName, Version: "1.0.0"})
	require.NoError(t, err)
	inspection, err := repo.Inspect(ctx, pinned)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"suite": "isolation"}, inspection.Record.Metadata.Labels, "the stored record is independent of the prepared one")
	assert.Equal(t, "data.txt", inspection.Record.Inventory[0].Path)
}

// TestRepository_SignDoesNotStoreEvidenceForARecordDeletedWhileSigning proves the subject is checked
// again once the signer returned: Sign releases the lock while the signer runs, so a concurrent
// Delete can remove the record in between.
func TestRepository_SignDoesNotStoreEvidenceForARecordDeletedWhileSigning(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.Delete = artifact.DeleteObject
	repo := artifacttest.New(artifacttest.Options{Capabilities: &caps})
	pinned := publishOne(t, repo)
	inspection, err := repo.Inspect(context.Background(), pinned)
	require.NoError(t, err)
	canonical, err := inspection.Record.Canonical()
	require.NoError(t, err)
	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digest.FromBytes(canonical), MediaType: artifact.MediaTypeConfig, Size: int64(len(canonical))}

	fake := &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}
	signer := artifact.NewMockSigner(gomock.NewController(t))
	signer.EXPECT().Formats().Return(fake.Formats()).AnyTimes()
	signer.EXPECT().Sign(gomock.Any(), subject).DoAndReturn(func(ctx context.Context, subject artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
		// The record disappears while the signer is still working.
		_, err := repo.Delete(ctx, pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: artifact.DeleteObject})
		require.NoError(t, err)
		return fake.Sign(ctx, subject)
	})

	evidence, err := repo.Sign(context.Background(), subject, signer, artifact.SignOptions{OperationID: "sign-op"})

	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	assert.Empty(t, evidence)
	assert.False(t, recordAlive(repo, pinned.RecordDigest))
	assert.False(t, evidenceAlive(t, repo, &pinned), "no evidence may be left for a deleted record")
}

// A repository that does not advertise the read capability rejects every read entrypoint, even when
// writes are enabled.
func TestRepository_ReadsAreRejectedWithoutTheReadCapability(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.Read = false
	repo := artifacttest.New(artifacttest.Options{Capabilities: &caps})
	ctx := context.Background()

	checks := map[string]func() error{
		"Resolve": func() error { _, err := repo.Resolve(ctx, artifact.Ref{Name: testName, Version: "1.0.0"}); return err },
		"Inspect": func() error { _, err := repo.Inspect(ctx, artifact.PinnedReference{}); return err },
		"OpenDocument": func() error {
			_, err := repo.OpenDocument(ctx, artifact.PinnedReference{}, artifact.DocumentRole("readme"))
			return err
		},
		"Read": func() error { _, err := repo.Read(ctx, artifact.PinnedReference{}); return err },
	}
	for name, call := range checks {
		t.Run(name, func(t *testing.T) {
			var capability *artifact.CapabilityError
			require.ErrorAs(t, call(), &capability)
			assert.Equal(t, "read", capability.Capability)
		})
	}
}

// A failure from the signer carries the signing sentinel and keeps its cause.
func TestRepository_SignClassifiesSignerFailures(t *testing.T) {
	repo := artifacttest.New(artifacttest.Options{})
	pinned := publishOne(t, repo)
	inspection, err := repo.Inspect(context.Background(), pinned)
	require.NoError(t, err)
	canonical, err := inspection.Record.Canonical()
	require.NoError(t, err)
	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digest.FromBytes(canonical), MediaType: artifact.MediaTypeConfig, Size: int64(len(canonical))}
	cause := errors.New("the signing service refused the request")
	fake := &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}
	signer := artifact.NewMockSigner(gomock.NewController(t))
	signer.EXPECT().Formats().Return(fake.Formats()).AnyTimes()
	signer.EXPECT().Sign(gomock.Any(), subject).Return(nil, cause)

	_, err = repo.Sign(context.Background(), subject, signer, artifact.SignOptions{OperationID: "sign-op"})

	require.ErrorIs(t, err, errUtils.ErrArtifactSigningFailed)
	assert.ErrorIs(t, err, cause)
}

func TestRepository_VerifyRejectsEvidenceIntegrityMismatch(t *testing.T) {
	source := artifacttest.New(artifacttest.Options{})
	pinned := publishOne(t, source)
	signEvidence(t, source, &pinned)
	valid, err := source.ReadEvidence(t.Context(), &pinned)
	require.NoError(t, err)
	require.Len(t, valid, 1)
	signer := &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}
	for _, mismatch := range []string{"size", "digest"} {
		t.Run(mismatch, func(t *testing.T) {
			repo := artifacttest.New(artifacttest.Options{})
			target := publishOne(t, repo)
			corrupt := valid[0]
			if mismatch == "size" {
				corrupt.Evidence.Descriptor.Size++
			} else {
				corrupt.Evidence.Descriptor.Digest = digest.FromString("different bytes")
			}
			_, err := repo.WriteEvidence(t.Context(), &target, []artifact.EvidenceBlob{corrupt}, nil)
			require.NoError(t, err)
			result, err := repo.Verify(t.Context(), valid[0].Evidence.Subject, signer.TrustedPolicy())
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
			assert.Empty(t, result.Claims)
			assert.Empty(t, result.EvidenceDigests)
			require.NotEmpty(t, result.Diagnostics)
			assert.Equal(t, "evidence-integrity", result.Diagnostics[0].Code)
			_, err = repo.WriteEvidence(t.Context(), &target, valid, nil)
			require.NoError(t, err)
			result, err = repo.Verify(t.Context(), valid[0].Evidence.Subject, signer.TrustedPolicy())
			require.NoError(t, err)
			assert.Len(t, result.Claims, 1)
			assert.Equal(t, []digest.Digest{valid[0].Evidence.Descriptor.Digest}, result.EvidenceDigests)
		})
	}
}
