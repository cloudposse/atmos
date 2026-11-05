package signature

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signature/signaturetest"
)

func TestSignBlobAcceptsOnlyEvidenceBoundToTheSignedBytes(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("release index bytes")
	valid := authority.Sign(t, "builder@example.com", "https://issuer.example.com", artifact)
	otherArtifact := authority.Sign(t, "builder@example.com", "https://issuer.example.com", []byte("different bytes"))
	failure := errors.New("signing backend unavailable")

	for _, tc := range []struct {
		name   string
		bundle []byte
		err    error
		want   error
	}{
		{"matching bundle", valid, nil, nil},
		{"bundle for other bytes", otherArtifact, nil, errUtils.ErrSignatureInvalid},
		{"malformed bundle", []byte("not json"), nil, errUtils.ErrSignatureInvalid},
		{"empty bundle", nil, nil, errUtils.ErrSignatureInvalid},
		{"oversized bundle", make([]byte, MaxBundleSize+1), nil, errUtils.ErrSignatureInvalid},
		{"signer failure", nil, failure, failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signer := NewMockBlobSigner(gomock.NewController(t))
			signer.EXPECT().Sign(gomock.Any(), artifact).Return(tc.bundle, tc.err)
			got, err := SignBlob(t.Context(), signer, artifact)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.bundle, got)
		})
	}
}

func TestCheckBundleSubjectRejectsEnvelopeBundles(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("statement subject")
	statement := authority.SignStatement(t, "builder@example.com", "https://issuer.example.com", artifact)
	// DSSE statements are verified by Verifier; SignBlob only vouches for message signatures.
	require.ErrorIs(t, CheckBundleSubject(statement, artifact), errUtils.ErrSignatureInvalid)
}
