package service_test

import (
	"context"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

// An Atmos package on OCI is addressed by its manifest digest, while the signed subject is the
// portable Record. Sign must accept the digest users see (the manifest or envelope digest) and the
// Record digest, and always sign the Record.
func TestSign_AcceptsTheResolvedOrTheRecordDigestAndSignsTheRecord(t *testing.T) {
	record, recordDigest := sealedRecord(t)
	manifest := digest.FromString("manifest")
	caps := artifacttest.DefaultCapabilities()

	tests := []struct {
		name       string
		named      digest.Digest
		wantNotice bool
	}{
		{"record digest", recordDigest, false},
		{"resolved manifest digest", manifest, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pinned := artifact.PinnedReference{Ref: artifact.Ref{Name: "tools/example", Digest: tt.named}, ResolvedDigest: manifest, RecordDigest: recordDigest}
			m := newMockSetup(t, &caps, service.WithSigner(releaseSigner))
			m.expectLock()
			m.repo.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(pinned, nil)
			m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned, Record: &record}, nil)
			var signed artifact.SignatureSubject
			m.repo.EXPECT().Sign(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, subject artifact.SignatureSubject, _ artifact.Signer, _ artifact.SignOptions) ([]artifact.SignatureEvidence, error) {
					signed = subject
					return []artifact.SignatureEvidence{{Subject: subject}}, nil
				})

			result, err := m.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: tt.named}})
			require.NoError(t, err)
			assert.Equal(t, artifact.SubjectRecord, signed.Kind)
			assert.Equal(t, recordDigest, signed.Digest, "the Record is always the signed subject")
			assert.Equal(t, recordDigest, result.Subject.Digest)
			assert.Equal(t, manifest, result.Operation.ResolvedDigest, "the result exposes the resolved digest")
			assert.Equal(t, recordDigest, result.Operation.RecordDigest, "and the Record digest")
			var notice bool
			for _, diagnostic := range result.Operation.Diagnostics {
				notice = notice || diagnostic.Code == "signed-subject"
			}
			assert.Equal(t, tt.wantNotice, notice, "naming the resolved digest explains which subject was signed")
		})
	}
}

func TestSign_AForeignGraphIsSignedByItsOriginalRoot(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.SubjectKinds = []artifact.SubjectKind{artifact.SubjectOCIRoot, artifact.SubjectRecord}
	root := digest.FromString("original-index")
	pinned := artifact.PinnedReference{Ref: artifact.Ref{Name: "tools/example", Digest: root}, ResolvedDigest: root}
	inspection := &artifact.Inspection{Pinned: pinned, Representation: artifact.Representation{OriginalRoot: &ocispec.Descriptor{Digest: root, MediaType: "application/vnd.oci.image.index.v1+json", Size: 10}}}

	m := newMockSetup(t, &caps, service.WithSigner(releaseSigner))
	m.expectLock()
	m.repo.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(pinned, nil)
	m.repo.EXPECT().Inspect(gomock.Any(), pinned).Return(inspection, nil)
	m.repo.EXPECT().Sign(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, subject artifact.SignatureSubject, _ artifact.Signer, _ artifact.SignOptions) ([]artifact.SignatureEvidence, error) {
			return []artifact.SignatureEvidence{{Subject: subject}}, nil
		})

	result, err := m.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: root}})
	require.NoError(t, err)
	assert.Equal(t, artifact.SubjectOCIRoot, result.Subject.Kind)
	assert.Equal(t, root, result.Subject.Digest)
}
