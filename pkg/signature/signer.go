package signature

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"

	"github.com/sigstore/sigstore-go/pkg/bundle"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// BlobSigner produces a standard Sigstore bundle for the exact supplied bytes. Implementations
// (for example the managed Cosign authoring path) own signing identity; registry credentials are
// unrelated to it.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock_signer.go -package=signature . BlobSigner
type BlobSigner interface {
	Sign(ctx context.Context, artifact []byte) ([]byte, error)
}

// SignBlob signs artifact and refuses output that is oversized, malformed, or bound to different bytes.
func SignBlob(ctx context.Context, signer BlobSigner, artifact []byte) ([]byte, error) {
	defer perf.Track(nil, "signature.SignBlob")()

	signed, err := signer.Sign(ctx, artifact)
	if err != nil {
		return nil, err
	}
	if err := CheckBundleSubject(signed, artifact); err != nil {
		return nil, err
	}
	return signed, nil
}

// CheckBundleSubject confirms a message-signature bundle commits to the SHA-256 of artifact.
// This is a structural check on freshly produced evidence; verification still goes through a Verifier.
func CheckBundleSubject(raw, artifact []byte) error {
	defer perf.Track(nil, "signature.CheckBundleSubject")()

	if len(raw) == 0 || len(raw) > MaxBundleSize {
		return fail(errUtils.ErrSignatureInvalid, nil)
	}
	var signed bundle.Bundle
	if err := json.Unmarshal(raw, &signed); err != nil {
		return fail(errUtils.ErrSignatureInvalid, err)
	}
	digest := sha256.Sum256(artifact)
	message := signed.GetMessageSignature()
	if message == nil || message.MessageDigest == nil || message.MessageDigest.Algorithm.String() != "SHA2_256" || !bytes.Equal(message.MessageDigest.Digest, digest[:]) {
		return fail(errUtils.ErrSignatureInvalid, nil)
	}
	return nil
}
