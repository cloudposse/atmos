package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

func signedSpec(spec *repoSpec) *repoSpec {
	return spec.withOpts(WithVerifier(&artifacttest.FakeVerifier{}))
}

// recordSubject builds the Record signature subject the service uses for the default reference.
func recordSubject(t *testing.T, repo *Repository) artifact.SignatureSubject {
	t.Helper()
	inspection, err := repo.Inspect(context.Background(), resolve(t, repo, defaultRef()))
	require.NoError(t, err)
	canonical, err := inspection.Record.Canonical()
	require.NoError(t, err)
	return artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digest.FromBytes(canonical), MediaType: artifact.MediaTypeConfig, Size: int64(len(canonical))}
}

// snapshot reads every file below dir into a map for before/after comparison.
func snapshot(t *testing.T, dir string, skip func(rel string) bool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rel := range listFiles(t, dir) {
		if skip != nil && skip(rel) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		require.NoError(t, err)
		out[rel] = string(data)
	}
	return out
}

func notEvidence(rel string) bool {
	return len(rel) >= len(layout.ManagedRoot+"/signatures") && rel[:len(layout.ManagedRoot+"/signatures")] == layout.ManagedRoot+"/signatures"
}

func TestSignature_StorageNeverMutatesTheSignedRecord(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			repo := newRepo(t, signedSpec(spec))
			result, _ := publishFilePackage(t, repo)
			pinned := resolve(t, repo, defaultRef())
			subject := recordSubject(t, repo)
			assert.Equal(t, pinned.RecordDigest, subject.Digest)
			before := snapshot(t, repo.root, nil)

			signer := newSigner()
			evidence, err := repo.Sign(context.Background(), subject, signer, artifact.SignOptions{OperationID: "sign-op"})
			require.NoError(t, err)
			require.Len(t, evidence, 1)
			assert.Equal(t, layout.SignaturePath(artifact.SubjectRecord, subject.Digest, evidence[0].Descriptor.Digest), evidence[0].Locator)
			assert.FileExists(t, filepath.Join(repo.root, filepath.FromSlash(evidence[0].Locator)))

			after := snapshot(t, repo.root, nil)
			added := 0
			for rel, content := range after {
				if notEvidence(rel) {
					added++
					continue
				}
				assert.Equal(t, before[rel], content, "%s must be byte-identical after signing", rel)
			}
			assert.Equal(t, 1, added, "exactly one detached evidence file was added")
			assert.Len(t, after, len(before)+1)

			again := resolve(t, repo, defaultRef())
			assert.Equal(t, pinned.RecordDigest, again.RecordDigest, "the signed Record, and so its digest, did not change")
			assert.Equal(t, result.Pinned.RecordDigest, again.RecordDigest)
			assertIntact(t, repo, defaultRef())

			inspection, err := repo.Inspect(context.Background(), pinned)
			require.NoError(t, err)
			require.Len(t, inspection.Evidence, 1)
			assert.Equal(t, evidence[0].Descriptor.Digest, inspection.Evidence[0].Descriptor.Digest)
			pin, err := repo.Pin(context.Background(), defaultRef())
			require.NoError(t, err)
			require.Len(t, pin.Evidence, 1, "a pin carries the evidence descriptors known now")

			verified, err := repo.Verify(context.Background(), subject, signer.TrustedPolicy())
			require.NoError(t, err)
			assert.Equal(t, artifact.VerificationVerified, verified.Status)
			assert.NotEmpty(t, verified.PolicyFingerprint)
			_, err = repo.Verify(context.Background(), subject, signer.UntrustedPolicy())
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		})
	}
}

// Signing an archive after publication leaves the archive bytes untouched: the evidence covers the
// Record that describes it, and no claim is made about the original archive bytes.
func TestSignature_ArchiveBytesAreNeverRewritten(t *testing.T) {
	repo := newRepo(t, signedSpec(archiveSpec(artifact.PackagingTgz)))
	result, _ := publishFilePackage(t, repo)
	archiveBefore, err := os.ReadFile(result.Location)
	require.NoError(t, err)

	_, err = repo.Sign(context.Background(), recordSubject(t, repo), newSigner(), artifact.SignOptions{})
	require.NoError(t, err)

	archiveAfter, err := os.ReadFile(result.Location)
	require.NoError(t, err)
	assert.Equal(t, archiveBefore, archiveAfter)
	assert.Equal(t, digest.FromBytes(archiveBefore), result.Representation.RepresentationDigest, "the representation checksum still describes the unsigned archive")
}

func TestSignature_VerifyFailsClosed(t *testing.T) {
	repo := newRepo(t, signedSpec(dirSpec()))
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	subject := recordSubject(t, repo)
	signer := newSigner()

	result, err := repo.Verify(context.Background(), subject, signer.TrustedPolicy())
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "no evidence is never verified")
	require.NotNil(t, result)
	assert.Equal(t, artifact.VerificationFailed, result.Status)

	_, err = repo.Sign(context.Background(), subject, signer, artifact.SignOptions{})
	require.NoError(t, err)

	// A malformed file and a file bound to another subject are reported and ignored; they can
	// neither break nor override the valid evidence.
	dir := filepath.Join(repo.root, filepath.FromSlash(layout.SignatureDir(artifact.SubjectRecord, subject.Digest)))
	junk := digest.FromString("junk")
	require.NoError(t, os.WriteFile(filepath.Join(dir, junk.Encoded()+".json"), []byte("not json"), 0o644))
	foreign := digest.FromString("foreign subject")
	evidence, err := layout.NewEvidence(artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: foreign}, &artifact.EvidencePayload{Format: artifact.EvidenceSigstoreBundle, MediaType: "x", Data: []byte("{}")})
	require.NoError(t, err)
	raw, err := evidence.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, evidence.Digest().Encoded()+".json"), raw, 0o644))

	verified, err := repo.Verify(context.Background(), subject, signer.TrustedPolicy())
	require.NoError(t, err)
	assert.Equal(t, artifact.VerificationVerified, verified.Status)
	codes := map[string]bool{}
	for _, diagnostic := range verified.Diagnostics {
		codes[diagnostic.Code] = true
	}
	assert.True(t, codes["malformed-evidence"], "malformed evidence is bounded and reported: %v", verified.Diagnostics)
	assert.True(t, codes["subject-mismatch"], "foreign evidence is reported: %v", verified.Diagnostics)

	// Evidence whose bytes were edited no longer matches its file name and is ignored.
	files := listFiles(t, dir)
	for _, name := range files {
		if name != junk.Encoded()+".json" && name != evidence.Digest().Encoded()+".json" {
			tamper(t, dir, name, appendJunk)
		}
	}
	_, err = repo.Verify(context.Background(), subject, signer.TrustedPolicy())
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "tampered evidence cannot verify")
}

func TestSignature_SubjectBinding(t *testing.T) {
	repo := newRepo(t, signedSpec(dirSpec()))
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	subject := recordSubject(t, repo)
	signer := newSigner()

	t.Run("unsupported subject kind", func(t *testing.T) {
		other := subject
		other.Kind = artifact.SubjectOCIRoot
		_, err := repo.Sign(context.Background(), other, signer, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
		_, err = repo.Verify(context.Background(), other, signer.TrustedPolicy())
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	})
	t.Run("unknown record", func(t *testing.T) {
		other := subject
		other.Digest = digest.FromString("missing")
		_, err := repo.Sign(context.Background(), other, signer, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	})
	t.Run("wrong declared size", func(t *testing.T) {
		other := subject
		other.Size++
		_, err := repo.Sign(context.Background(), other, signer, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
	})
	t.Run("nil signer", func(t *testing.T) {
		_, err := repo.Sign(context.Background(), subject, nil, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
	t.Run("format the repository cannot verify", func(t *testing.T) {
		_, err := repo.Sign(context.Background(), subject, &foreignSigner{}, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	})
	t.Run("signer produces nothing", func(t *testing.T) {
		_, err := repo.Sign(context.Background(), subject, &nilSigner{}, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	})
}

type foreignSigner struct{}

func (*foreignSigner) Formats() []artifact.EvidenceFormat { return []artifact.EvidenceFormat{"other"} }

func (*foreignSigner) Sign(context.Context, artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	return &artifact.EvidencePayload{Format: "other", MediaType: "x", Data: []byte("x")}, nil
}

type nilSigner struct{ foreignSigner }

func (*nilSigner) Sign(context.Context, artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	return nil, nil
}

func TestSignature_WithoutVerifierIsACapabilityError(t *testing.T) {
	repo := newRepo(t, dirSpec())
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	subject := recordSubject(t, repo)
	_, err := repo.Sign(context.Background(), subject, newSigner(), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "no advertised evidence format means signing is unavailable, not a silent no-op")
	_, err = repo.Verify(context.Background(), subject, newSigner().TrustedPolicy())
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}

func TestSignature_ExportAndImportEvidenceAcrossRepositories(t *testing.T) {
	source := newRepo(t, signedSpec(archiveSpec(artifact.PackagingTgz)))
	publish(t, source, &prepareSpec{}, artifact.PublishOptions{})
	sourcePinned := resolve(t, source, defaultRef())
	subject := recordSubject(t, source)
	signer := newSigner()
	_, err := source.Sign(context.Background(), subject, signer, artifact.SignOptions{})
	require.NoError(t, err)

	blobs, err := source.ReadEvidence(context.Background(), &sourcePinned)
	require.NoError(t, err)
	require.Len(t, blobs, 1)

	// The destination stores the same Record in a different packaging. The Record, and so the
	// signed subject, is identical, so imported evidence verifies there without re-signing.
	destination := newRepo(t, signedSpec(dirSpec()))
	publish(t, destination, &prepareSpec{}, artifact.PublishOptions{})
	destPinned := resolve(t, destination, defaultRef())
	assert.Equal(t, subject.Digest, destPinned.RecordDigest, "packaging does not change Record identity")
	_, err = destination.WriteEvidence(context.Background(), &destPinned, blobs, nil)
	require.NoError(t, err)
	verified, err := destination.Verify(context.Background(), recordSubject(t, destination), signer.TrustedPolicy())
	require.NoError(t, err)
	assert.Equal(t, artifact.VerificationVerified, verified.Status)

	different := newRepo(t, signedSpec(dirSpec()))
	publish(t, different, &prepareSpec{files: map[string]string{"data.txt": "other content"}}, artifact.PublishOptions{})
	differentPinned := resolve(t, different, defaultRef())
	_, err = different.WriteEvidence(context.Background(), &differentPinned, blobs, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid, "evidence for another Record is never imported")

	imported, err := source.WriteEvidence(context.Background(), &sourcePinned, blobs, nil)
	require.NoError(t, err)
	require.Len(t, imported, 1)
	assert.Equal(t, blobs[0].Evidence.Descriptor.Digest, imported[0].Descriptor.Digest, "re-import is idempotent")
	again, err := source.ReadEvidence(context.Background(), &sourcePinned)
	require.NoError(t, err)
	assert.Len(t, again, 1)

	corrupted := blobs[0]
	corrupted.Data = append([]byte(nil), corrupted.Data...)
	corrupted.Data[0] ^= 0xff
	_, err = source.WriteEvidence(context.Background(), &sourcePinned, []artifact.EvidenceBlob{corrupted}, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
}

func TestSignature_EvidenceFollowsTheRecordLifecycle(t *testing.T) {
	repo := newRepo(t, signedSpec(dirSpec()))
	publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v1"}}, artifact.PublishOptions{})
	old := resolve(t, repo, defaultRef())
	oldSubject := recordSubject(t, repo)
	_, err := repo.Sign(context.Background(), oldSubject, newSigner(), artifact.SignOptions{})
	require.NoError(t, err)
	evidenceDir := filepath.Join(repo.root, filepath.FromSlash(layout.SignatureDir(artifact.SubjectRecord, oldSubject.Digest)))
	assert.DirExists(t, evidenceDir)

	publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v2"}, opID: "op-2"}, artifact.PublishOptions{Force: true, ExpectedPrior: &old})
	assert.NoDirExists(t, evidenceDir, "evidence of a retired Record is retired with it")
	_, err = repo.Sign(context.Background(), oldSubject, newSigner(), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "a retired Record cannot be signed")

	current := resolve(t, repo, defaultRef())
	currentSubject := recordSubject(t, repo)
	_, err = repo.Sign(context.Background(), currentSubject, newSigner(), artifact.SignOptions{})
	require.NoError(t, err)
	currentDir := filepath.Join(repo.root, filepath.FromSlash(layout.SignatureDir(artifact.SubjectRecord, currentSubject.Digest)))
	assert.DirExists(t, currentDir)
	_, err = repo.Delete(context.Background(), current, artifact.DeleteOptions{Scope: artifact.DeleteObject})
	require.NoError(t, err)
	assert.NoDirExists(t, currentDir, "delete removes the evidence the artifact exclusively owns")
}
