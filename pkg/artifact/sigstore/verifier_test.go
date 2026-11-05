package sigstore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

var _ = artifact.SignatureSubject{Kind: artifact.SubjectRecord}

func TestVerify_Matrix(t *testing.T) {
	tests := []struct {
		name       string
		policy     func(f *fixture) artifact.VerificationPolicy
		evidence   func(f *fixture) []artifact.EvidenceBlob
		opts       func(f *fixture) []VerifierOption
		wantStatus artifact.VerificationStatus
		wantCodes  []string
		wantClaim  artifact.SignerClaim
	}{
		{
			name:   "keyless message signature over the subject digest",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}
			},
			wantStatus: artifact.VerificationVerified,
			wantClaim:  artifact.SignerClaim{Identity: testIdentity, Issuer: testIssuer},
		},
		{
			name:   "keyless statement form from the Signer",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.keylessStatement(testIdentity, testIssuer)}
			},
			wantStatus: artifact.VerificationVerified,
			wantClaim:  artifact.SignerClaim{Identity: testIdentity, Issuer: testIssuer},
		},
		{
			name:   "DSSE in-toto statement naming the subject digest",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.blob(artifact.EvidenceSigstoreBundle, f.auth.SignStatement(f.t, testIdentity, testIssuer, f.data))}
			},
			wantStatus: artifact.VerificationVerified,
			wantClaim:  artifact.SignerClaim{Identity: testIdentity, Issuer: testIssuer},
		},
		{
			name: "identity wildcard",
			policy: func(*fixture) artifact.VerificationPolicy {
				return identityPolicy("https://github.com/acme/*", testIssuer)
			},
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}
			},
			wantStatus: artifact.VerificationVerified,
			wantClaim:  artifact.SignerClaim{Identity: testIdentity, Issuer: testIssuer},
		},
		{
			name:       "no evidence",
			policy:     func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence:   func(*fixture) []artifact.EvidenceBlob { return nil },
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"no-permitted-signature"},
		},
		{
			name:   "wrong issuer",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.keyless(testIdentity, "https://evil.example.com")}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"no-permitted-signature", "verification-failed"},
		},
		{
			name: "policy issuer differs from the signed issuer",
			policy: func(*fixture) artifact.VerificationPolicy {
				return identityPolicy(testIdentity, "https://other.example.com")
			},
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"no-permitted-signature", "verification-failed"},
		},
		{
			name:   "wrong identity",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.keyless("https://github.com/evil/repo/.github/workflows/x.yml@refs/heads/main", testIssuer)}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"no-permitted-signature", "verification-failed"},
		},
		{
			name:   "signature over different bytes is wrong for the subject",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.blob(artifact.EvidenceSigstoreBundle, f.auth.Sign(f.t, testIdentity, testIssuer, []byte("another record")))}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"no-permitted-signature", "verification-failed"},
		},
		{
			name:   "tampered bundle",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				tampered := tamperSignature(f.t, f.auth.Sign(f.t, testIdentity, testIssuer, f.data))
				return []artifact.EvidenceBlob{f.blob(artifact.EvidenceSigstoreBundle, tampered)}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"no-permitted-signature", "verification-failed"},
		},
		{
			name:   "evidence bound to another subject is ignored",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				other := f.keyless(testIdentity, testIssuer)
				other.Evidence.Subject.Digest = digest.FromString("another subject")
				return []artifact.EvidenceBlob{other}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"subject-mismatch", "no-permitted-signature"},
		},
		{
			name:   "evidence of another subject kind is ignored",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				other := f.keyless(testIdentity, testIssuer)
				other.Evidence.Subject.Kind = artifact.SubjectOCIRoot
				return []artifact.EvidenceBlob{other}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"subject-mismatch", "no-permitted-signature"},
		},
		{
			name:   "declared format that disagrees with the verification material",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				return []artifact.EvidenceBlob{f.blob(artifact.EvidencePublicKeyBundle, f.auth.Sign(f.t, testIdentity, testIssuer, f.data))}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"format-mismatch", "no-permitted-signature"},
		},
		{
			name:   "descriptor digest that does not match the bytes",
			policy: func(*fixture) artifact.VerificationPolicy { return identityPolicy(testIdentity, testIssuer) },
			evidence: func(f *fixture) []artifact.EvidenceBlob {
				item := f.keyless(testIdentity, testIssuer)
				item.Evidence.Descriptor.Digest = digest.FromString("something else")
				return []artifact.EvidenceBlob{item}
			},
			wantStatus: artifact.VerificationFailed,
			wantCodes:  []string{"malformed-evidence", "no-permitted-signature"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			var opts []VerifierOption
			if tt.opts != nil {
				opts = tt.opts(f)
			}
			result, err := f.verifier(opts...).Verify(context.Background(), f.subject, tt.evidence(f), tt.policy(f))
			require.NotNil(t, result)
			assert.Equal(t, tt.wantStatus, result.Status)
			assert.NotEmpty(t, result.PolicyFingerprint)
			assert.Equal(t, f.subject, result.Subject)
			if tt.wantStatus == artifact.VerificationVerified {
				require.NoError(t, err)
				require.Len(t, result.Claims, 1)
				assert.Equal(t, tt.wantClaim, result.Claims[0])
				assert.Len(t, result.EvidenceDigests, 1)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
			assert.Empty(t, result.Claims)
			for _, code := range tt.wantCodes {
				assert.Contains(t, codes(result), code)
			}
		})
	}
}

func TestVerify_PublicKeyPath(t *testing.T) {
	t.Run("configured key verifies and never satisfies an identity policy", func(t *testing.T) {
		f := newFixture(t)
		item, signed := f.keyed()
		verifier := f.verifier(WithTrustedKey(keyHint, signed.PublicKey))

		result, err := verifier.Verify(context.Background(), f.subject, []artifact.EvidenceBlob{item}, keyPolicy(keyHint))
		require.NoError(t, err)
		assert.Equal(t, artifact.VerificationVerified, result.Status)
		require.Len(t, result.Claims, 1)
		assert.Empty(t, result.Claims[0].Identity, "a key never produces an identity claim")
		assert.NotEmpty(t, result.Claims[0].KeyID)

		result, err = verifier.Verify(context.Background(), f.subject, []artifact.EvidenceBlob{item}, identityPolicy(testIdentity, testIssuer))
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		assert.Equal(t, artifact.VerificationFailed, result.Status)
	})

	t.Run("a key-only identity cannot be forged by naming it in the identity", func(t *testing.T) {
		f := newFixture(t)
		item, signed := f.keyed()
		verifier := f.verifier(WithTrustedKey(keyHint, signed.PublicKey))
		_, err := verifier.Verify(context.Background(), f.subject, []artifact.EvidenceBlob{item}, identityPolicy(keyHint, testIssuer))
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	})

	t.Run("a keyless bundle never satisfies a key policy", func(t *testing.T) {
		f := newFixture(t)
		_, signed := f.keyed()
		verifier := f.verifier(WithTrustedKey(keyHint, signed.PublicKey))
		result, err := verifier.Verify(context.Background(), f.subject, []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}, keyPolicy(keyHint))
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		assert.Equal(t, artifact.VerificationFailed, result.Status)
	})

	t.Run("an untrusted key fails", func(t *testing.T) {
		f := newFixture(t)
		item, _ := f.keyed()
		_, trusted := f.keyed()
		verifier := f.verifier(WithTrustedKey(keyHint, trusted.PublicKey))
		result, err := verifier.Verify(context.Background(), f.subject, []artifact.EvidenceBlob{item}, keyPolicy(keyHint))
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		assert.Equal(t, artifact.VerificationFailed, result.Status)
	})

	t.Run("an unregistered key reference fails with the reason", func(t *testing.T) {
		f := newFixture(t)
		item, _ := f.keyed()
		reader := func(string) ([]byte, error) { return nil, os.ErrNotExist }
		result, err := f.verifier(WithKeyFileReader(reader)).Verify(context.Background(), f.subject, []artifact.EvidenceBlob{item}, keyPolicy("missing.pub"))
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		assert.Contains(t, fmt.Sprint(result.Diagnostics), "missing.pub")
	})

	t.Run("public key file loaded from disk by the default reader", func(t *testing.T) {
		f := newFixture(t)
		item, signed := f.keyed()
		encoded, fingerprint := pemFor(t, signed)
		path := filepath.Join(t.TempDir(), "trusted.pub")
		require.NoError(t, os.WriteFile(path, []byte(encoded), 0o600))
		// Bundles from real signers identify the key by its fingerprint.
		item.Data = bytes.Replace(item.Data, []byte(keyHint), []byte(fingerprint), 1)
		item.Evidence.Descriptor.Digest = digest.FromBytes(item.Data)

		result, err := f.verifier().Verify(context.Background(), f.subject, []artifact.EvidenceBlob{item}, keyPolicy(path))
		require.NoError(t, err)
		assert.Equal(t, artifact.VerificationVerified, result.Status)
		assert.Equal(t, fingerprint, result.Claims[0].KeyID)
	})

	t.Run("malformed key file is reported", func(t *testing.T) {
		f := newFixture(t)
		item, _ := f.keyed()
		path := filepath.Join(t.TempDir(), "bad.pub")
		require.NoError(t, os.WriteFile(path, []byte("not pem"), 0o600))
		result, err := f.verifier().Verify(context.Background(), f.subject, []artifact.EvidenceBlob{item}, keyPolicy(path))
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		assert.Contains(t, fmt.Sprint(result.Diagnostics), "not a PEM PUBLIC KEY")
	})
}

func TestVerify_TrustSetsAreAllRequired(t *testing.T) {
	f := newFixture(t)
	item, signed := f.keyed()
	keyless := f.keyless(testIdentity, testIssuer)
	verifier := f.verifier(WithTrustedKey(keyHint, signed.PublicKey))
	policy := artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{
		{Identities: []artifact.TrustedIdentity{{Identity: testIdentity, Issuer: testIssuer}}},
		{Keys: []string{keyHint}},
	}}

	result, err := verifier.Verify(context.Background(), f.subject, []artifact.EvidenceBlob{keyless, item}, policy)
	require.NoError(t, err)
	assert.Equal(t, artifact.VerificationVerified, result.Status)
	assert.Len(t, result.Claims, 2)
	assert.Len(t, result.EvidenceDigests, 2)

	for name, held := range map[string][]artifact.EvidenceBlob{"only keyless": {keyless}, "only key": {item}} {
		t.Run(name, func(t *testing.T) {
			result, err := verifier.Verify(context.Background(), f.subject, held, policy)
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
			assert.Equal(t, artifact.VerificationFailed, result.Status)
		})
	}

	t.Run("one set with identities and keys accepts either signer", func(t *testing.T) {
		either := artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{{
			Identities: []artifact.TrustedIdentity{{Identity: testIdentity, Issuer: testIssuer}},
			Keys:       []string{keyHint},
		}}}
		for _, held := range [][]artifact.EvidenceBlob{{keyless}, {item}} {
			result, err := verifier.Verify(context.Background(), f.subject, held, either)
			require.NoError(t, err)
			assert.Equal(t, artifact.VerificationVerified, result.Status)
		}
	})
}

func TestVerify_TrustMaterial(t *testing.T) {
	tests := []struct {
		name    string
		loader  RootLoader
		offline bool
		message string
	}{
		{
			name:    "offline with no cached trust",
			loader:  func(context.Context, bool) (root.TrustedMaterial, error) { return nil, errUtils.ErrSignatureOffline },
			offline: true,
			message: errUtils.ErrSignatureOffline.Error(),
		},
		{
			name: "expired trust material",
			loader: func(context.Context, bool) (root.TrustedMaterial, error) {
				return nil, errors.New("trusted root metadata expired")
			},
			message: "trusted root metadata expired",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			policy := identityPolicy(testIdentity, testIssuer)
			policy.Offline = tt.offline
			result, err := NewVerifier(WithRootLoader(tt.loader)).Verify(context.Background(), f.subject, []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}, policy)
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
			assert.Equal(t, artifact.VerificationFailed, result.Status)
			assert.Contains(t, fmt.Sprint(result.Diagnostics), tt.message)
		})
	}

	t.Run("trust from a different authority does not verify", func(t *testing.T) {
		f := newFixture(t)
		other := newFixture(t)
		verifier := NewVerifier(WithRootLoader(f.loader(other.auth)))
		_, err := verifier.Verify(context.Background(), f.subject, []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}, identityPolicy(testIdentity, testIssuer))
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	})

	t.Run("the offline flag reaches the loader", func(t *testing.T) {
		f := newFixture(t)
		var seen []bool
		loader := func(_ context.Context, offline bool) (root.TrustedMaterial, error) {
			seen = append(seen, offline)
			return f.auth, nil
		}
		policy := identityPolicy(testIdentity, testIssuer)
		policy.Offline = true
		_, err := NewVerifier(WithRootLoader(loader)).Verify(context.Background(), f.subject, []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}, policy)
		require.NoError(t, err)
		require.NotEmpty(t, seen)
		for _, offline := range seen {
			assert.True(t, offline)
		}
	})
}

func TestVerify_UnrelatedEvidenceNeverOverridesValidEvidence(t *testing.T) {
	f := newFixture(t)
	valid := f.keyless(testIdentity, testIssuer)
	malformed := f.blob(artifact.EvidenceSigstoreBundle, []byte("{not json"))
	unsupported := f.blob("cosign-tag-payload", []byte("legacy"))
	foreign := f.keyless("https://github.com/evil/repo/.github/workflows/x.yml@refs/heads/main", testIssuer)
	evidence := []artifact.EvidenceBlob{malformed, unsupported, foreign, valid}

	result, err := f.verifier().Verify(context.Background(), f.subject, evidence, identityPolicy(testIdentity, testIssuer))
	require.NoError(t, err)
	assert.Equal(t, artifact.VerificationVerified, result.Status)
	assert.Equal(t, []digestValue{valid.Evidence.Descriptor.Digest}, toValues(result.EvidenceDigests))
	assert.Contains(t, codes(result), "malformed-evidence")
	assert.Contains(t, codes(result), "unsupported-evidence")

	t.Run("unsupported-only evidence fails closed", func(t *testing.T) {
		result, err := f.verifier().Verify(context.Background(), f.subject, []artifact.EvidenceBlob{unsupported}, identityPolicy(testIdentity, testIssuer))
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		assert.Equal(t, artifact.VerificationFailed, result.Status)
	})

	t.Run("diagnostics are bounded", func(t *testing.T) {
		many := make([]artifact.EvidenceBlob, 0, 60)
		for index := range 59 {
			many = append(many, f.blob(artifact.EvidenceSigstoreBundle, fmt.Appendf(nil, "{bad %d", index)))
		}
		many = append(many, valid)
		result, err := f.verifier().Verify(context.Background(), f.subject, many, identityPolicy(testIdentity, testIssuer))
		require.NoError(t, err)
		assert.Equal(t, artifact.VerificationVerified, result.Status)
		assert.Len(t, result.Diagnostics, maxDiagnostics+1)
		assert.Equal(t, "diagnostics-truncated", result.Diagnostics[len(result.Diagnostics)-1].Code)
	})

	t.Run("oversize evidence is malformed", func(t *testing.T) {
		huge := f.blob(artifact.EvidenceSigstoreBundle, bytes.Repeat([]byte("x"), int(artifact.MaxEvidenceSize)+1))
		result, err := f.verifier().Verify(context.Background(), f.subject, []artifact.EvidenceBlob{huge, valid}, identityPolicy(testIdentity, testIssuer))
		require.NoError(t, err)
		assert.Contains(t, codes(result), "malformed-evidence")
	})
}

type digestValue = digest.Digest

func toValues(in []digest.Digest) []digestValue { return in }

func TestVerify_NoTrustConstraintsIsUnchecked(t *testing.T) {
	f := newFixture(t)
	for _, policy := range []artifact.VerificationPolicy{{}, {RequireSignature: true}, {TrustSets: []artifact.TrustSet{{}}}} {
		result, err := f.verifier().Verify(context.Background(), f.subject, []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}, policy)
		require.NoError(t, err)
		assert.Equal(t, artifact.VerificationUnchecked, result.Status, "nothing is claimed without trust configuration")
		assert.Empty(t, result.Claims)
	}
}

func TestVerify_CanceledContext(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := f.verifier().Verify(ctx, f.subject, []artifact.EvidenceBlob{f.keyless(testIdentity, testIssuer)}, identityPolicy(testIdentity, testIssuer))
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	assert.Equal(t, artifact.VerificationFailed, result.Status)
}

func TestVerifier_Formats(t *testing.T) {
	assert.ElementsMatch(t, []artifact.EvidenceFormat{artifact.EvidenceSigstoreBundle, artifact.EvidencePublicKeyBundle}, NewVerifier().Formats())
}

func TestNewVerifier_DefaultsAndNilOptions(t *testing.T) {
	verifier := NewVerifier(nil, WithRootLoader(nil), WithKeyFileReader(nil))
	require.NotNil(t, verifier.loadRoot)
	require.NotNil(t, verifier.readFile)
}

// tamperSignature flips one byte of the bundle's message signature and re-encodes the bundle.
func tamperSignature(t *testing.T, bundle []byte) []byte {
	t.Helper()
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(bundle, &parsed))
	message, ok := parsed["messageSignature"].(map[string]any)
	require.True(t, ok)
	raw, err := base64.StdEncoding.DecodeString(message["signature"].(string))
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0xff
	message["signature"] = base64.StdEncoding.EncodeToString(raw)
	out, err := json.Marshal(parsed)
	require.NoError(t, err)
	return out
}
