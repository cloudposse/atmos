package sops

import (
	"context"

	"github.com/getsops/sops/v3/gcpkms"
	"github.com/getsops/sops/v3/keyservice"

	"github.com/cloudposse/atmos/pkg/store/sopsauth"
)

// init registers the GCP KMS handler so the key service dispatches "gcp_kms" master keys to it.
func init() {
	registerCloudKeyHandler(gcpKeyHandler{})
}

// gcpKeyHandler injects an Atmos identity's GCP credentials into GCP KMS SOPS master keys.
type gcpKeyHandler struct{}

func (gcpKeyHandler) keyTypeID() string { return keyTypeGCPKMS }

func (gcpKeyHandler) decrypt(ctx context.Context, b sopsauth.Builder, identity string, req *keyservice.DecryptRequest) ([]byte, error) {
	applier, err := b.GCPKMS(ctx, identity)
	if err != nil {
		return nil, err
	}
	mk := gcpkms.MasterKey{ResourceID: req.Key.GetGcpKmsKey().ResourceId}
	applier.ApplyToMasterKey(&mk)
	mk.EncryptedKey = string(req.Ciphertext)
	return mk.Decrypt()
}

func (gcpKeyHandler) encrypt(ctx context.Context, b sopsauth.Builder, identity string, req *keyservice.EncryptRequest) ([]byte, error) {
	applier, err := b.GCPKMS(ctx, identity)
	if err != nil {
		return nil, err
	}
	mk := gcpkms.MasterKey{ResourceID: req.Key.GetGcpKmsKey().ResourceId}
	applier.ApplyToMasterKey(&mk)
	if err := mk.Encrypt(req.Plaintext); err != nil {
		return nil, err
	}
	return []byte(mk.EncryptedKey), nil
}
