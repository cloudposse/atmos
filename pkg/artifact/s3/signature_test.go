package s3

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

func signedSpec(t *testing.T, spec *rspec) *rspec {
	t.Helper()
	return spec.fresh(t).withOpts(WithVerifier(&artifacttest.FakeVerifier{}))
}

// objectSnapshot copies every object of the fake for before/after comparison.
func objectSnapshot(t *testing.T, fake *s3test.Server) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, key := range fake.Keys() {
		object, ok := fake.Object(key)
		require.True(t, ok)
		out[key] = string(object.Data)
	}
	return out
}

func isEvidenceKey(key string) bool {
	return len(key) > len(layout.ManagedRoot+"/signatures") && key[:len(layout.ManagedRoot+"/signatures")] == layout.ManagedRoot+"/signatures"
}

// Evidence is a digest-named object beside the Record; signing changes no other object.
func TestSignature_StorageNeverMutatesTheSignedRecord(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = signedSpec(t, spec)
			repo := newRepo(t, spec)
			prep, _ := packageSpec(t)
			result := publish(t, repo, prep, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			subject := recordSubject(t, repo, &pinned)
			assert.Equal(t, pinned.RecordDigest, subject.Digest)
			before := objectSnapshot(t, spec.fake)

			signer := newSigner()
			evidence, err := repo.Sign(context.Background(), subject, signer, artifact.SignOptions{OperationID: "sign-op"})
			require.NoError(t, err)
			require.Len(t, evidence, 1)
			evidenceKey := layout.SignaturePath(artifact.SubjectRecord, subject.Digest, evidence[0].Descriptor.Digest)
			assert.Equal(t, "s3://"+spec.fake.Bucket()+"/"+evidenceKey, evidence[0].Locator)

			after := objectSnapshot(t, spec.fake)
			added := 0
			for key, content := range after {
				if isEvidenceKey(key) {
					added++
					continue
				}
				assert.Equal(t, before[key], content, "%s must be byte-identical after signing", key)
			}
			assert.Equal(t, 1, added, "exactly one detached evidence object was added")
			assert.Len(t, after, len(before)+1)

			again := resolve(t, repo, defaultRef())
			assert.Equal(t, result.Pinned.RecordDigest, again.RecordDigest, "the signed Record, and so its digest, did not change")
			assertIntact(t, repo, defaultRef())

			inspection, err := repo.Inspect(context.Background(), pinned)
			require.NoError(t, err)
			require.Len(t, inspection.Evidence, 1)
			pin, err := repo.Pin(context.Background(), defaultRef())
			require.NoError(t, err)
			require.Len(t, pin.Evidence, 1, "a pin carries the evidence descriptors known now")
			assert.Equal(t, pinned.RecordDigest, pin.RecordDigest, "the pin is the canonical Record digest")

			verified, err := repo.Verify(context.Background(), subject, signer.TrustedPolicy())
			require.NoError(t, err)
			assert.Equal(t, artifact.VerificationVerified, verified.Status)
			_, err = repo.Verify(context.Background(), subject, signer.UntrustedPolicy())
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		})
	}
}

// Verification fails closed: a store that cannot be listed is an error, never "no evidence".
func TestSignature_VerifyFailsClosed(t *testing.T) {
	spec := signedSpec(t, noneSpec())
	repo := newRepo(t, spec)
	pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
	subject := recordSubject(t, repo, &pinned)
	signer := newSigner()
	_, err := repo.Sign(context.Background(), subject, signer, artifact.SignOptions{})
	require.NoError(t, err)

	t.Run("listing failure", func(t *testing.T) {
		spec.fake.Inject(s3test.Fault{Op: s3test.OpListObjectsV2, Status: 500, Times: -1})
		defer spec.fake.ClearFaults()
		_, err := repo.Verify(context.Background(), subject, signer.TrustedPolicy())
		require.Error(t, err)
		assert.NotErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "a store failure is reported as such, and verification did not pass")
		_, err = repo.Pin(context.Background(), defaultRef())
		require.Error(t, err)
	})
	t.Run("malformed and foreign evidence is reported and ignored", func(t *testing.T) {
		dir := layout.SignatureDir(artifact.SubjectRecord, subject.Digest)
		spec.fake.Seed(dir+"/"+"0000000000000000000000000000000000000000000000000000000000000000.json", []byte("not json"), nil)
		result, err := repo.Verify(context.Background(), subject, signer.TrustedPolicy())
		require.NoError(t, err, "valid evidence still verifies")
		var malformed bool
		for _, diagnostic := range result.Diagnostics {
			malformed = malformed || diagnostic.Code == "malformed-evidence"
		}
		assert.True(t, malformed, "the malformed object is reported: %v", result.Diagnostics)
	})
	t.Run("no evidence never verifies", func(t *testing.T) {
		bare := newRepo(t, signedSpec(t, noneSpec()))
		pinned := publish(t, bare, &prepareSpec{}, artifact.PublishOptions{}).Pinned
		_, err := bare.Verify(context.Background(), recordSubject(t, bare, &pinned), signer.TrustedPolicy())
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	})
}

func TestSignature_WithoutVerifierIsACapabilityError(t *testing.T) {
	repo := newRepo(t, noneSpec().fresh(t))
	pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
	subject := recordSubject(t, repo, &pinned)
	_, err := repo.Sign(context.Background(), subject, newSigner(), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	_, err = repo.Verify(context.Background(), subject, newSigner().TrustedPolicy())
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.Empty(t, repo.Capabilities().EvidenceFormats)
}

func TestSignature_SubjectBinding(t *testing.T) {
	repo := newRepo(t, signedSpec(t, noneSpec()))
	pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
	subject := recordSubject(t, repo, &pinned)

	wrongSize := subject
	wrongSize.Size++
	_, err := repo.Sign(context.Background(), wrongSize, newSigner(), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)

	badKind := subject
	badKind.Kind = artifact.SubjectOCIRoot
	_, err = repo.Sign(context.Background(), badKind, newSigner(), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)

	_, err = repo.Sign(context.Background(), subject, nil, artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

// Evidence exported from one S3 repository imports into another that holds the same Record.
func TestSignature_ExportAndImportEvidenceAcrossRepositories(t *testing.T) {
	source := newRepo(t, signedSpec(t, archiveSpec(artifact.PackagingTgz)))
	publish(t, source, &prepareSpec{}, artifact.PublishOptions{})
	sourcePinned := resolve(t, source, defaultRef())
	subject := recordSubject(t, source, &sourcePinned)
	signer := newSigner()
	_, err := source.Sign(context.Background(), subject, signer, artifact.SignOptions{})
	require.NoError(t, err)

	blobs, err := source.ReadEvidence(context.Background(), &sourcePinned)
	require.NoError(t, err)
	require.Len(t, blobs, 1)

	destination := newRepo(t, signedSpec(t, noneSpec()))
	publish(t, destination, &prepareSpec{}, artifact.PublishOptions{})
	destPinned := resolve(t, destination, defaultRef())
	assert.Equal(t, subject.Digest, destPinned.RecordDigest, "packaging does not change Record identity")
	_, err = destination.WriteEvidence(context.Background(), &destPinned, blobs, nil)
	require.NoError(t, err)
	verified, err := destination.Verify(context.Background(), recordSubject(t, destination, &destPinned), signer.TrustedPolicy())
	require.NoError(t, err)
	assert.Equal(t, artifact.VerificationVerified, verified.Status)

	different := newRepo(t, signedSpec(t, noneSpec()))
	publish(t, different, &prepareSpec{files: map[string]string{"data.txt": "other content"}}, artifact.PublishOptions{})
	differentPinned := resolve(t, different, defaultRef())
	_, err = different.WriteEvidence(context.Background(), &differentPinned, blobs, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid, "evidence for another Record is never imported")

	imported, err := source.WriteEvidence(context.Background(), &sourcePinned, blobs, nil)
	require.NoError(t, err)
	require.Len(t, imported, 1)
	assert.Equal(t, blobs[0].Evidence.Descriptor.Digest, imported[0].Descriptor.Digest, "re-import is idempotent")

	corrupted := blobs[0]
	corrupted.Data = append([]byte(nil), corrupted.Data...)
	corrupted.Data[0] ^= 0xff
	_, err = source.WriteEvidence(context.Background(), &sourcePinned, []artifact.EvidenceBlob{corrupted}, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
}

func TestSignature_EvidenceFollowsTheRecordLifecycle(t *testing.T) {
	spec := signedSpec(t, noneSpec())
	repo := newRepo(t, spec)
	first := publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v1"}}, artifact.PublishOptions{})
	oldSubject := recordSubject(t, repo, &first.Pinned)
	_, err := repo.Sign(context.Background(), oldSubject, newSigner(), artifact.SignOptions{})
	require.NoError(t, err)
	oldDir := layout.SignatureDir(artifact.SubjectRecord, oldSubject.Digest)
	assert.NotEmpty(t, managedKeys(spec.fake, "", oldDir))

	second := publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v2"}, opID: "op-2"}, artifact.PublishOptions{Force: true, ExpectedPrior: &first.Pinned})
	assert.Empty(t, managedKeys(spec.fake, "", oldDir), "evidence of a retired Record is retired with it")
	_, err = repo.Sign(context.Background(), oldSubject, newSigner(), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "a retired Record cannot be signed")

	currentSubject := recordSubject(t, repo, &second.Pinned)
	_, err = repo.Sign(context.Background(), currentSubject, newSigner(), artifact.SignOptions{})
	require.NoError(t, err)
	currentDir := layout.SignatureDir(artifact.SubjectRecord, currentSubject.Digest)
	assert.NotEmpty(t, managedKeys(spec.fake, "", currentDir))
	_, err = repo.Delete(context.Background(), second.Pinned, artifact.DeleteOptions{Scope: artifact.DeleteObject})
	require.NoError(t, err)
	assert.Empty(t, managedKeys(spec.fake, "", currentDir), "delete removes the evidence the artifact exclusively owns")
}
