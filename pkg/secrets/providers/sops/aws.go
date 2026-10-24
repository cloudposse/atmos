package sops

import (
	"context"

	"github.com/getsops/sops/v3/keyservice"
	"github.com/getsops/sops/v3/kms"

	"github.com/cloudposse/atmos/pkg/store/sopsauth"
)

// init registers the AWS KMS handler so the key service dispatches "kms" master keys to it.
func init() {
	registerCloudKeyHandler(awsKeyHandler{})
}

// awsKeyHandler injects an Atmos identity's AWS credentials into AWS KMS SOPS master keys.
type awsKeyHandler struct{}

func (awsKeyHandler) keyTypeID() string { return keyTypeAWSKMS }

func (awsKeyHandler) decrypt(ctx context.Context, b sopsauth.Builder, identity string, req *keyservice.DecryptRequest) ([]byte, error) {
	applier, err := b.AWSKMS(ctx, identity)
	if err != nil {
		return nil, err
	}
	mk := kmsKeyToMasterKey(req.Key.GetKmsKey())
	applier.ApplyToMasterKey(&mk)
	mk.EncryptedKey = string(req.Ciphertext)
	return mk.Decrypt()
}

func (awsKeyHandler) encrypt(ctx context.Context, b sopsauth.Builder, identity string, req *keyservice.EncryptRequest) ([]byte, error) {
	applier, err := b.AWSKMS(ctx, identity)
	if err != nil {
		return nil, err
	}
	mk := kmsKeyToMasterKey(req.Key.GetKmsKey())
	applier.ApplyToMasterKey(&mk)
	if err := mk.Encrypt(req.Plaintext); err != nil {
		return nil, err
	}
	return []byte(mk.EncryptedKey), nil
}

// kmsKeyToMasterKey reconstructs an AWS KMS master key from a keyservice request, replicating
// getsops' own keyservice server conversion (Arn, Role, EncryptionContext, AwsProfile).
func kmsKeyToMasterKey(key *keyservice.KmsKey) kms.MasterKey {
	ctx := make(map[string]*string, len(key.Context))
	for k, v := range key.Context {
		value := v
		ctx[k] = &value
	}
	return kms.MasterKey{
		Arn:               key.Arn,
		Role:              key.Role,
		EncryptionContext: ctx,
		AwsProfile:        key.AwsProfile,
	}
}
