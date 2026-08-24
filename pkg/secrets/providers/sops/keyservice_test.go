package sops

import (
	"context"
	"errors"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/getsops/sops/v3"
	sopsage "github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/keyservice"
	"github.com/getsops/sops/v3/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/sopsauth"
)

// noopResolver satisfies store.AuthContextResolver without doing anything; keyClient only needs a
// non-nil resolver to decide to wrap — it does not call it.
type noopResolver struct{}

func (noopResolver) ResolveAWSAuthContext(context.Context, string) (*store.AWSAuthConfig, error) {
	return &store.AWSAuthConfig{}, nil
}

func (noopResolver) ResolveAzureAuthContext(context.Context, string) (*store.AzureAuthConfig, error) {
	return &store.AzureAuthConfig{}, nil
}

func (noopResolver) ResolveGCPAuthContext(context.Context, string) (*store.GCPAuthConfig, error) {
	return &store.GCPAuthConfig{}, nil
}

// fakeBuilder is a sopsauth.Builder whose AWSKMS resolution returns a sentinel error, letting tests
// assert that a request was dispatched to the AWS cloud handler (and not the fallback).
type fakeBuilder struct{ awsErr error }

func (f fakeBuilder) AWSKMS(context.Context, string) (sopsauth.KMSApplier, error) {
	return nil, f.awsErr
}
func (fakeBuilder) GCPKMS(context.Context, string) (sopsauth.GCPApplier, error) { return nil, nil }
func (fakeBuilder) AzureKV(context.Context, string) (sopsauth.AzureApplier, error) {
	return nil, nil
}

// recordingKeyService is a fallback keyservice that records whether it was invoked.
type recordingKeyService struct{ decrypted, encrypted bool }

func (r *recordingKeyService) Decrypt(_ context.Context, _ *keyservice.DecryptRequest, _ ...grpc.CallOption) (*keyservice.DecryptResponse, error) {
	r.decrypted = true
	return &keyservice.DecryptResponse{Plaintext: []byte("fallback")}, nil
}

func (r *recordingKeyService) Encrypt(_ context.Context, _ *keyservice.EncryptRequest, _ ...grpc.CallOption) (*keyservice.EncryptResponse, error) {
	r.encrypted = true
	return &keyservice.EncryptResponse{Ciphertext: []byte("fallback")}, nil
}

// TestKeyClientSelection is the regression guard for #2637: a SOPS provider with a resolvable
// identity must wrap the base key service so cloud-KMS keys authenticate via that identity; without
// an identity it must return the plain base client (preserving ambient-credential behavior).
func TestKeyClientSelection(t *testing.T) {
	t.Run("identity present wraps with sopsKeyServiceClient", func(t *testing.T) {
		p := &sopsProvider{kind: "sops/aws-kms", authResolver: noopResolver{}, effectiveIdentity: "acme-prod"}
		client, err := p.keyClient()
		require.NoError(t, err)
		_, ok := client.(*sopsKeyServiceClient)
		assert.True(t, ok, "with an identity, keyClient must wrap in sopsKeyServiceClient")
	})

	t.Run("no identity returns base client (ambient fallback)", func(t *testing.T) {
		p := &sopsProvider{kind: "sops/aws-kms"} // no resolver / identity
		client, err := p.keyClient()
		require.NoError(t, err)
		_, ok := client.(*sopsKeyServiceClient)
		assert.False(t, ok, "without an identity, keyClient must NOT wrap (ambient-credential behavior)")
	})
}

// TestSopsKeyServiceClient_Dispatch verifies cloud-KMS key types route to their registered handler
// (surfaced via the fake builder's sentinel error) while non-cloud key types (age) fall through to
// the fallback key service.
func TestSopsKeyServiceClient_Dispatch(t *testing.T) {
	sentinel := errors.New("aws builder invoked")
	fallback := &recordingKeyService{}
	c := &sopsKeyServiceClient{builder: fakeBuilder{awsErr: sentinel}, identity: "id", fallback: fallback}

	t.Run("kms key dispatches to aws handler", func(t *testing.T) {
		req := &keyservice.DecryptRequest{Key: &keyservice.Key{KeyType: &keyservice.Key_KmsKey{KmsKey: &keyservice.KmsKey{Arn: "arn:aws:kms:us-east-1:0:key/abc"}}}}
		_, err := c.Decrypt(context.Background(), req)
		require.ErrorIs(t, err, sentinel, "a KMS key must be dispatched to the AWS cloud handler")
		assert.False(t, fallback.decrypted, "fallback must not be used for a cloud-KMS key")
	})

	t.Run("age key falls through to fallback", func(t *testing.T) {
		req := &keyservice.DecryptRequest{Key: &keyservice.Key{KeyType: &keyservice.Key_AgeKey{AgeKey: &keyservice.AgeKey{Recipient: "age1xxx"}}}}
		_, err := c.Decrypt(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, fallback.decrypted, "a non-cloud key must delegate to the fallback")
	})
}

// TestKeyTypeID maps getsops oneof key types to their identifiers (empty for non-cloud types).
func TestKeyTypeID(t *testing.T) {
	cases := map[string]struct {
		key  *keyservice.Key
		want string
	}{
		"kms":   {&keyservice.Key{KeyType: &keyservice.Key_KmsKey{KmsKey: &keyservice.KmsKey{}}}, keyTypeAWSKMS},
		"gcp":   {&keyservice.Key{KeyType: &keyservice.Key_GcpKmsKey{GcpKmsKey: &keyservice.GcpKmsKey{}}}, keyTypeGCPKMS},
		"azure": {&keyservice.Key{KeyType: &keyservice.Key_AzureKeyvaultKey{AzureKeyvaultKey: &keyservice.AzureKeyVaultKey{}}}, keyTypeAzureKV},
		"age":   {&keyservice.Key{KeyType: &keyservice.Key_AgeKey{AgeKey: &keyservice.AgeKey{}}}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, keyTypeID(tc.key))
		})
	}
}

// TestCloudKeyHandlersRegistered confirms the per-cloud handlers self-registered by key type.
func TestCloudKeyHandlersRegistered(t *testing.T) {
	for _, id := range []string{keyTypeAWSKMS, keyTypeGCPKMS, keyTypeAzureKV} {
		_, ok := cloudKeyHandlers[id]
		assert.Truef(t, ok, "a cloud key handler must be registered for %q", id)
	}
}

// TestIdentityPrecedence verifies per-provider `identity` wins over the injected default identity.
func TestIdentityPrecedence(t *testing.T) {
	secretsAuth := &store.SecretsAuthContext{Resolver: noopResolver{}, DefaultIdentity: "stack-default"}

	t.Run("per-provider identity wins", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{SecretsAuth: secretsAuth}
		section := map[string]any{"v": map[string]any{"kind": "sops/aws-kms", "identity": "provider-id", "spec": map[string]any{"file": "f.enc.yaml"}}}
		prov, err := New(cfg, "v", section)
		require.NoError(t, err)
		assert.Equal(t, "provider-id", prov.(*sopsProvider).effectiveIdentity)
	})

	t.Run("falls back to default identity", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{SecretsAuth: secretsAuth}
		section := map[string]any{"v": map[string]any{"kind": "sops/aws-kms", "spec": map[string]any{"file": "f.enc.yaml"}}}
		prov, err := New(cfg, "v", section)
		require.NoError(t, err)
		assert.Equal(t, "stack-default", prov.(*sopsProvider).effectiveIdentity)
	})
}

// TestDecryptErrHints verifies hints are derived from the file's actual key types: a KMS file gets
// identity hints (not age hints), and an age file gets age-key hints.
func TestDecryptErrHints(t *testing.T) {
	cause := errors.New("boom")

	t.Run("kms file yields identity hint, not age hint", func(t *testing.T) {
		p := &sopsProvider{name: "acme-sops", kind: "sops/aws-kms", effectiveIdentity: "acme-prod"}
		mk := kms.NewMasterKeyFromArn("arn:aws:kms:us-east-1:0:key/abc", nil, "")
		tree := &sops.Tree{Metadata: sops.Metadata{KeyGroups: []sops.KeyGroup{{mk}}}}
		hints := strings.Join(cockroachErrors.GetAllHints(p.decryptErr("f.enc.yaml", tree, cause)), "\n")
		assert.Contains(t, hints, "acme-prod", "a KMS file's hint should name the identity")
		assert.NotContains(t, hints, "age-keygen", "a KMS file must not get age-key hints")
	})

	t.Run("age file yields age-key hint", func(t *testing.T) {
		p := &sopsProvider{name: "dev-sops", kind: "sops/age"}
		mk := &sopsage.MasterKey{Recipient: "age1xxx"}
		tree := &sops.Tree{Metadata: sops.Metadata{KeyGroups: []sops.KeyGroup{{mk}}}}
		hints := strings.Join(cockroachErrors.GetAllHints(p.decryptErr("f.enc.yaml", tree, cause)), "\n")
		assert.Contains(t, hints, "SOPS_AGE_KEY", "an age file should get age-key hints")
	})
}
