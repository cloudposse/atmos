package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// verifierProbe records how many evidence blobs reached the verifier.
type verifierProbe struct{ seen int }

// mockVerifier returns a generated verifier mock with a canned outcome. Its probe records the
// evidence count of the latest Verify call.
func mockVerifier(t *testing.T, result *artifact.VerificationResult, err error) (*artifact.MockVerifier, *verifierProbe) {
	t.Helper()
	probe := &verifierProbe{}
	verifier := artifact.NewMockVerifier(gomock.NewController(t))
	verifier.EXPECT().Formats().AnyTimes().Return([]artifact.EvidenceFormat{artifact.EvidenceSigstoreBundle})
	verifier.EXPECT().Verify(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, _ artifact.SignatureSubject, evidence []artifact.EvidenceBlob, _ artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
			probe.seen = len(evidence)
			return result, err
		})
	return verifier, probe
}

// mockSigner returns a generated signer mock that always signs data.
func mockSigner(t *testing.T, data []byte) *artifact.MockSigner {
	t.Helper()
	signer := artifact.NewMockSigner(gomock.NewController(t))
	signer.EXPECT().Formats().AnyTimes().Return([]artifact.EvidenceFormat{artifact.EvidenceSigstoreBundle})
	signer.EXPECT().Sign(gomock.Any(), gomock.Any()).AnyTimes().
		Return(&artifact.EvidencePayload{Format: artifact.EvidenceSigstoreBundle, MediaType: "application/json", Data: data}, nil)
	return signer
}

func signedWith(t *testing.T, verifier artifact.Verifier) (*Repository, artifact.SignatureSubject) {
	t.Helper()
	repo := newRepo(t, &repoSpec{opts: []Option{WithVerifier(verifier)}})
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	subject := recordSubject(t, repo)
	_, err := repo.Sign(context.Background(), subject, mockSigner(t, []byte(`{"bundle":1}`)), artifact.SignOptions{})
	require.NoError(t, err)
	return repo, subject
}

var trustedPolicy = artifact.VerificationPolicy{
	RequireSignature: true,
	TrustSets:        []artifact.TrustSet{{Identities: []artifact.TrustedIdentity{{Identity: "who", Issuer: "https://issuer.example.com"}}}},
}

func TestVerify_TranslatesVerifierOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		result     *artifact.VerificationResult
		err        error
		wantErr    error
		wantStatus artifact.VerificationStatus
	}{
		{"verified", &artifact.VerificationResult{Status: artifact.VerificationVerified}, nil, nil, artifact.VerificationVerified},
		{"unchecked is not verified", &artifact.VerificationResult{Status: artifact.VerificationUnchecked}, nil, errUtils.ErrArtifactVerificationFailed, artifact.VerificationUnchecked},
		{"failed status without an error still fails", &artifact.VerificationResult{Status: artifact.VerificationFailed}, nil, errUtils.ErrArtifactVerificationFailed, artifact.VerificationFailed},
		{"verifier error is wrapped as a verification failure", nil, errors.New("crypto exploded"), errUtils.ErrArtifactVerificationFailed, artifact.VerificationFailed},
		{"verification sentinel is kept", &artifact.VerificationResult{Status: artifact.VerificationFailed}, fmt.Errorf("%w: policy", errUtils.ErrArtifactVerificationFailed), errUtils.ErrArtifactVerificationFailed, artifact.VerificationFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub, probe := mockVerifier(t, tt.result, tt.err)
			repo, subject := signedWith(t, stub)
			result, err := repo.Verify(context.Background(), subject, trustedPolicy)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.NotNil(t, result)
			assert.Equal(t, tt.wantStatus, result.Status)
			assert.Equal(t, subject, result.Subject)
			assert.NotEmpty(t, result.PolicyFingerprint, "the applied policy is always fingerprinted")
			assert.Equal(t, 1, probe.seen)
		})
	}
}

func TestVerify_ReportsDiagnosticsAlongsideTheVerifier(t *testing.T) {
	stub, _ := mockVerifier(t, &artifact.VerificationResult{
		Status: artifact.VerificationVerified, PolicyFingerprint: "verifier-fingerprint",
		Diagnostics: []artifact.Diagnostic{{Severity: artifact.SeverityInfo, Code: "from-verifier", Message: "ok"}},
	}, nil)
	repo, subject := signedWith(t, stub)
	dir := filepath.Join(repo.root, filepath.FromSlash(layout.SignatureDir(artifact.SubjectRecord, subject.Digest)))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zz.json"), []byte("{}"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.json"), 0o755))

	result, err := repo.Verify(context.Background(), subject, trustedPolicy)
	require.NoError(t, err)
	assert.Equal(t, "verifier-fingerprint", result.PolicyFingerprint, "the verifier's own fingerprint is kept")
	codes := map[string]bool{}
	for _, diagnostic := range result.Diagnostics {
		codes[diagnostic.Code] = true
	}
	assert.True(t, codes["from-verifier"])
	assert.True(t, codes["malformed-evidence"], "a file whose name is not an evidence digest is reported, not trusted")
}

func TestVerify_ReadsAllValidEvidence(t *testing.T) {
	stub, probe := mockVerifier(t, &artifact.VerificationResult{Status: artifact.VerificationVerified}, nil)
	repo, subject := signedWith(t, stub)
	for index := range maxMalformedEvidence + 5 {
		evidence, err := layout.NewEvidence(subject, &artifact.EvidencePayload{Format: artifact.EvidenceSigstoreBundle, MediaType: "x", Data: []byte(fmt.Sprintf("bundle-%d", index))})
		require.NoError(t, err)
		raw, err := evidence.Marshal()
		require.NoError(t, err)
		path := filepath.Join(repo.root, filepath.FromSlash(layout.SignaturePath(artifact.SubjectRecord, subject.Digest, evidence.Digest())))
		require.NoError(t, os.WriteFile(path, raw, 0o644))
	}
	result, err := repo.Verify(context.Background(), subject, trustedPolicy)
	require.NoError(t, err)
	assert.Equal(t, maxMalformedEvidence+6, probe.seen, "all valid evidence reaches the verifier")
	assert.Equal(t, artifact.VerificationVerified, result.Status)
}

func TestSign_RejectsOversizeEvidence(t *testing.T) {
	verifier, _ := mockVerifier(t, nil, nil)
	repo := newRepo(t, &repoSpec{opts: []Option{WithVerifier(verifier)}})
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	subject := recordSubject(t, repo)
	_, err := repo.Sign(context.Background(), subject, mockSigner(t, make([]byte, artifact.MaxEvidenceSize+1)), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
	_, err = repo.Sign(context.Background(), subject, mockSigner(t, nil), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, "empty evidence is invalid")
}

func TestVerifyRejectsUnreadableEvidenceDirectory(t *testing.T) {
	stub, probe := mockVerifier(t, &artifact.VerificationResult{Status: artifact.VerificationVerified}, nil)
	repo, subject := signedWith(t, stub)
	dir := filepath.Join(repo.root, filepath.FromSlash(layout.SignatureDir(subject.Kind, subject.Digest)))
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("not a directory"), 0o644))
	result, err := repo.Verify(context.Background(), subject, trustedPolicy)
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	require.Contains(t, result.Diagnostics[0].Code, "evidence-read-failed")
	assert.Zero(t, probe.seen)
}

func TestVerifyFailsClosedOnTooManyMalformedEvidenceFiles(t *testing.T) {
	stub, probe := mockVerifier(t, &artifact.VerificationResult{Status: artifact.VerificationVerified}, nil)
	repo, subject := signedWith(t, stub)
	dir := filepath.Join(repo.root, filepath.FromSlash(layout.SignatureDir(subject.Kind, subject.Digest)))
	for i := range maxMalformedEvidence + 1 {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("bad-%03d.json", i)), []byte("bad"), 0o644))
	}
	_, err := repo.Verify(context.Background(), subject, trustedPolicy)
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	require.ErrorIs(t, err, errTooManyMalformedEvidence)
	assert.Zero(t, probe.seen)
}
