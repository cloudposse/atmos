package sigstore

import (
	"context"
	"errors"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/signature"
)

func TestSigner_SignRoundTripsThroughVerifier(t *testing.T) {
	f := newFixture(t)
	signer := NewSigner(&authoritySigner{t: t, auth: f.auth, identity: testIdentity, issuer: testIssuer})
	for _, kind := range []artifact.SubjectKind{artifact.SubjectRecord, artifact.SubjectOCIRoot} {
		t.Run(string(kind), func(t *testing.T) {
			subject := f.subject
			subject.Kind = kind
			payload, err := signer.Sign(context.Background(), subject)
			require.NoError(t, err)
			assert.Equal(t, artifact.EvidenceSigstoreBundle, payload.Format)
			assert.Equal(t, "application/vnd.dev.sigstore.bundle.v0.3+json", payload.MediaType)

			evidence := artifact.EvidenceBlob{
				Evidence: artifact.SignatureEvidence{Subject: subject, Format: payload.Format},
				Data:     payload.Data,
			}
			result, err := f.verifier().Verify(context.Background(), subject, []artifact.EvidenceBlob{evidence}, identityPolicy(testIdentity, testIssuer))
			require.NoError(t, err)
			assert.Equal(t, artifact.VerificationVerified, result.Status)

			// The statement binds kind, so the same bundle does not verify a different subject.
			other := subject
			other.MediaType = "application/octet-stream"
			evidence.Evidence.Subject = other
			_, err = f.verifier().Verify(context.Background(), other, []artifact.EvidenceBlob{evidence}, identityPolicy(testIdentity, testIssuer))
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		})
	}
}

func TestSigner_KeyBundleIsAPublicKeyBundle(t *testing.T) {
	f := newFixture(t)
	statement := SubjectStatement(f.subject)
	signed := f.auth.SignWithKey(t, statement)
	ctrl := gomock.NewController(t)
	blob := signature.NewMockBlobSigner(ctrl)
	blob.EXPECT().Sign(gomock.Any(), statement).Return(signed.Bundle, nil)

	payload, err := NewSigner(blob).Sign(context.Background(), f.subject)
	require.NoError(t, err)
	assert.Equal(t, artifact.EvidencePublicKeyBundle, payload.Format)

	verifier := f.verifier(WithTrustedKey(keyHint, signed.PublicKey))
	result, err := verifier.Verify(context.Background(), f.subject, []artifact.EvidenceBlob{{
		Evidence: artifact.SignatureEvidence{Subject: f.subject, Format: payload.Format},
		Data:     payload.Data,
	}}, keyPolicy(keyHint))
	require.NoError(t, err)
	assert.Equal(t, artifact.VerificationVerified, result.Status)
}

func TestSigner_Failures(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name    string
		signer  *Signer
		subject artifact.SignatureSubject
		ctx     func() context.Context
		wantErr error
	}{
		{
			name:    "output bound to different bytes",
			signer:  NewSigner(&authoritySigner{t: t, auth: f.auth, identity: testIdentity, issuer: testIssuer, replace: []byte("other bytes")}),
			subject: f.subject,
			wantErr: errUtils.ErrArtifactVerificationFailed,
		},
		{
			name:    "invalid subject",
			signer:  NewSigner(&authoritySigner{t: t, auth: f.auth, identity: testIdentity, issuer: testIssuer}),
			subject: artifact.SignatureSubject{Kind: "bogus", Digest: digest.FromString("x")},
			wantErr: errUtils.ErrArtifactRefInvalid,
		},
		{
			name:    "no blob signer",
			signer:  NewSigner(nil),
			subject: f.subject,
			wantErr: errUtils.ErrArtifactUnsupportedCapability,
		},
		{
			name:    "canceled context",
			signer:  NewSigner(&authoritySigner{t: t, auth: f.auth, identity: testIdentity, issuer: testIssuer}),
			subject: f.subject,
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}
			payload, err := tt.signer.Sign(ctx, tt.subject)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, payload)
		})
	}

	t.Run("signer error is returned", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		blob := signature.NewMockBlobSigner(ctrl)
		boom := errors.New("cosign failed")
		blob.EXPECT().Sign(gomock.Any(), gomock.Any()).Return(nil, boom)
		_, err := NewSigner(blob).Sign(context.Background(), f.subject)
		require.ErrorIs(t, err, boom)
	})
}

func TestSigner_Formats(t *testing.T) {
	assert.ElementsMatch(t, NewVerifier().Formats(), NewSigner(nil).Formats())
}

func TestSubjectStatement_IsDeterministicAndBindsEveryField(t *testing.T) {
	subject := artifact.SignatureSubject{
		Kind:      artifact.SubjectRecord,
		Digest:    digest.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		MediaType: artifact.MediaTypeConfig,
		Size:      42,
	}
	want := `{"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","kind":"record","media_type":"application/vnd.atmos.artifact.config.v1+json","size":42,"version":1}`
	assert.Equal(t, want, string(SubjectStatement(subject)))
	assert.Equal(t, SubjectStatement(subject), SubjectStatement(subject))

	variants := map[string]artifact.SignatureSubject{}
	for name, mutate := range map[string]func(*artifact.SignatureSubject){
		"kind":   func(s *artifact.SignatureSubject) { s.Kind = artifact.SubjectOCIRoot },
		"digest": func(s *artifact.SignatureSubject) { s.Digest = digest.FromString("x") },
		"media":  func(s *artifact.SignatureSubject) { s.MediaType = "x" },
		"size":   func(s *artifact.SignatureSubject) { s.Size = 43 },
	} {
		changed := subject
		mutate(&changed)
		variants[name] = changed
		assert.NotEqual(t, SubjectStatement(subject), SubjectStatement(changed), name)
	}
}

func TestEvidenceFormatOf(t *testing.T) {
	tests := []struct {
		name      string
		bundle    string
		want      artifact.EvidenceFormat
		wantMedia string
		wantErr   bool
	}{
		{name: "certificate", bundle: `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","verificationMaterial":{"certificate":{"rawBytes":"AA=="}}}`, want: artifact.EvidenceSigstoreBundle, wantMedia: "application/vnd.dev.sigstore.bundle.v0.3+json"},
		{name: "certificate chain defaults the media type", bundle: `{"verificationMaterial":{"x509CertificateChain":{"certificates":[]}}}`, want: artifact.EvidenceSigstoreBundle, wantMedia: defaultBundleMediaType},
		{name: "public key", bundle: `{"mediaType":"application/vnd.dev.sigstore.bundle+json;version=0.2","verificationMaterial":{"publicKey":{"hint":"k"}}}`, want: artifact.EvidencePublicKeyBundle, wantMedia: "application/vnd.dev.sigstore.bundle+json;version=0.2"},
		{name: "both", bundle: `{"verificationMaterial":{"certificate":{},"publicKey":{}}}`, wantErr: true},
		{name: "neither", bundle: `{"verificationMaterial":{}}`, wantErr: true},
		{name: "null material", bundle: `{"verificationMaterial":{"certificate":null}}`, wantErr: true},
		{name: "not json", bundle: `nope`, wantErr: true},
		{name: "empty", bundle: ``, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, media, err := EvidenceFormatOf([]byte(tt.bundle))
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, format)
			assert.Equal(t, tt.wantMedia, media)
		})
	}
	assert.Contains(t, defaultBundleMediaType, BundleMediaTypePrefix)
}
