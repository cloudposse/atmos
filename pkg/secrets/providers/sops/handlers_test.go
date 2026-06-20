package sops

import (
	"context"
	"errors"
	"testing"

	"github.com/getsops/sops/v3/keyservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store/sopsauth"
)

// errBuilder is a sopsauth.Builder whose per-cloud resolution returns a configurable error, letting
// tests exercise the credential-resolution failure branch of each cloud handler without real KMS.
type errBuilder struct {
	awsErr, gcpErr, azureErr error
}

func (e errBuilder) AWSKMS(context.Context, string) (sopsauth.KMSApplier, error) {
	return nil, e.awsErr
}

func (e errBuilder) GCPKMS(context.Context, string) (sopsauth.GCPApplier, error) {
	return nil, e.gcpErr
}

func (e errBuilder) AzureKV(context.Context, string) (sopsauth.AzureApplier, error) {
	return nil, e.azureErr
}

// TestCloudKeyHandlers_BuilderError verifies that when an identity's credentials cannot be resolved,
// each cloud handler surfaces that error from both decrypt and encrypt instead of proceeding.
func TestCloudKeyHandlers_BuilderError(t *testing.T) {
	awsErr := errors.New("aws creds failed")
	gcpErr := errors.New("gcp creds failed")
	azureErr := errors.New("azure creds failed")
	b := errBuilder{awsErr: awsErr, gcpErr: gcpErr, azureErr: azureErr}
	ctx := context.Background()

	kmsDecrypt := &keyservice.DecryptRequest{Key: &keyservice.Key{KeyType: &keyservice.Key_KmsKey{KmsKey: &keyservice.KmsKey{}}}}
	kmsEncrypt := &keyservice.EncryptRequest{Key: &keyservice.Key{KeyType: &keyservice.Key_KmsKey{KmsKey: &keyservice.KmsKey{}}}, Plaintext: []byte("x")}
	gcpDecrypt := &keyservice.DecryptRequest{Key: &keyservice.Key{KeyType: &keyservice.Key_GcpKmsKey{GcpKmsKey: &keyservice.GcpKmsKey{}}}}
	gcpEncrypt := &keyservice.EncryptRequest{Key: &keyservice.Key{KeyType: &keyservice.Key_GcpKmsKey{GcpKmsKey: &keyservice.GcpKmsKey{}}}, Plaintext: []byte("x")}
	azureDecrypt := &keyservice.DecryptRequest{Key: &keyservice.Key{KeyType: &keyservice.Key_AzureKeyvaultKey{AzureKeyvaultKey: &keyservice.AzureKeyVaultKey{}}}}
	azureEncrypt := &keyservice.EncryptRequest{Key: &keyservice.Key{KeyType: &keyservice.Key_AzureKeyvaultKey{AzureKeyvaultKey: &keyservice.AzureKeyVaultKey{}}}, Plaintext: []byte("x")}

	t.Run("aws decrypt surfaces builder error", func(t *testing.T) {
		_, err := awsKeyHandler{}.decrypt(ctx, b, "id", kmsDecrypt)
		assert.ErrorIs(t, err, awsErr)
	})
	t.Run("aws encrypt surfaces builder error", func(t *testing.T) {
		_, err := awsKeyHandler{}.encrypt(ctx, b, "id", kmsEncrypt)
		assert.ErrorIs(t, err, awsErr)
	})
	t.Run("gcp decrypt surfaces builder error", func(t *testing.T) {
		_, err := gcpKeyHandler{}.decrypt(ctx, b, "id", gcpDecrypt)
		assert.ErrorIs(t, err, gcpErr)
	})
	t.Run("gcp encrypt surfaces builder error", func(t *testing.T) {
		_, err := gcpKeyHandler{}.encrypt(ctx, b, "id", gcpEncrypt)
		assert.ErrorIs(t, err, gcpErr)
	})
	t.Run("azure decrypt surfaces builder error", func(t *testing.T) {
		_, err := azureKeyHandler{}.decrypt(ctx, b, "id", azureDecrypt)
		assert.ErrorIs(t, err, azureErr)
	})
	t.Run("azure encrypt surfaces builder error", func(t *testing.T) {
		_, err := azureKeyHandler{}.encrypt(ctx, b, "id", azureEncrypt)
		assert.ErrorIs(t, err, azureErr)
	})
}

// TestCloudKeyHandlers_KeyTypeID confirms each handler advertises the getsops key-type it serves.
func TestCloudKeyHandlers_KeyTypeID(t *testing.T) {
	assert.Equal(t, keyTypeAWSKMS, awsKeyHandler{}.keyTypeID())
	assert.Equal(t, keyTypeGCPKMS, gcpKeyHandler{}.keyTypeID())
	assert.Equal(t, keyTypeAzureKV, azureKeyHandler{}.keyTypeID())
}

// TestKmsKeyToMasterKey verifies the keyservice→master-key conversion preserves identity fields and
// deep-copies the encryption context (the values must be independent pointers, not aliases).
func TestKmsKeyToMasterKey(t *testing.T) {
	key := &keyservice.KmsKey{
		Arn:        "arn:aws:kms:us-east-1:0:key/abc",
		Role:       "arn:aws:iam::0:role/r",
		AwsProfile: "prof",
		Context:    map[string]string{"app": "atmos", "env": "prod"},
	}

	mk := kmsKeyToMasterKey(key)

	assert.Equal(t, key.Arn, mk.Arn)
	assert.Equal(t, key.Role, mk.Role)
	assert.Equal(t, key.AwsProfile, mk.AwsProfile)
	require.Len(t, mk.EncryptionContext, 2)
	require.Contains(t, mk.EncryptionContext, "app")
	require.Contains(t, mk.EncryptionContext, "env")
	assert.Equal(t, "atmos", *mk.EncryptionContext["app"])
	assert.Equal(t, "prod", *mk.EncryptionContext["env"])

	// Mutating the source map must not affect the already-converted master key.
	key.Context["app"] = "mutated"
	assert.Equal(t, "atmos", *mk.EncryptionContext["app"], "encryption context must be deep-copied")
}

// TestAzureMasterKey verifies the keyservice→master-key conversion for Azure Key Vault keys.
func TestAzureMasterKey(t *testing.T) {
	mk := azureMasterKey(&keyservice.AzureKeyVaultKey{
		VaultUrl: "https://vault.vault.azure.net",
		Name:     "my-key",
		Version:  "v1",
	})
	assert.Equal(t, "https://vault.vault.azure.net", mk.VaultURL)
	assert.Equal(t, "my-key", mk.Name)
	assert.Equal(t, "v1", mk.Version)
}

// TestCloudKeyTypeName maps key-type identifiers to human-readable cloud names for error hints,
// including the default fallback for unknown identifiers.
func TestCloudKeyTypeName(t *testing.T) {
	cases := map[string]string{
		keyTypeAWSKMS:  "AWS KMS",
		keyTypeGCPKMS:  "GCP KMS",
		keyTypeAzureKV: "Azure Key Vault",
		"something":    "cloud KMS",
		"":             "cloud KMS",
	}
	for id, want := range cases {
		assert.Equalf(t, want, cloudKeyTypeName(id), "cloudKeyTypeName(%q)", id)
	}
}

// TestRegisterCloudKeyHandler_Duplicate ensures a second handler claiming an already-registered key
// type panics — the panic fires before mutating the registry, so global state is unaffected.
func TestRegisterCloudKeyHandler_Duplicate(t *testing.T) {
	assert.PanicsWithValue(
		t,
		`duplicate SOPS cloud key handler for "kms"`,
		func() { registerCloudKeyHandler(awsKeyHandler{}) },
		"re-registering an existing key type must panic",
	)
}

// TestSopsKeyServiceClient_EncryptDispatch mirrors the Decrypt dispatch test for the Encrypt path:
// cloud-KMS keys route to their handler (surfaced via the fake builder's sentinel) and non-cloud
// keys fall through to the fallback key service.
func TestSopsKeyServiceClient_EncryptDispatch(t *testing.T) {
	sentinel := errors.New("aws builder invoked")
	fallback := &recordingKeyService{}
	c := &sopsKeyServiceClient{builder: fakeBuilder{awsErr: sentinel}, identity: "id", fallback: fallback}

	t.Run("kms key dispatches to aws handler", func(t *testing.T) {
		req := &keyservice.EncryptRequest{
			Key:       &keyservice.Key{KeyType: &keyservice.Key_KmsKey{KmsKey: &keyservice.KmsKey{Arn: "arn:aws:kms:us-east-1:0:key/abc"}}},
			Plaintext: []byte("secret"),
		}
		_, err := c.Encrypt(context.Background(), req)
		require.ErrorIs(t, err, sentinel, "a KMS key must be dispatched to the AWS cloud handler")
		assert.False(t, fallback.encrypted, "fallback must not be used for a cloud-KMS key")
	})

	t.Run("age key falls through to fallback", func(t *testing.T) {
		req := &keyservice.EncryptRequest{
			Key:       &keyservice.Key{KeyType: &keyservice.Key_AgeKey{AgeKey: &keyservice.AgeKey{Recipient: "age1xxx"}}},
			Plaintext: []byte("secret"),
		}
		_, err := c.Encrypt(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, fallback.encrypted, "a non-cloud key must delegate to the fallback")
	})
}
