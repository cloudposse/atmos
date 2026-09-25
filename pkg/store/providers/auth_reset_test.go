package providers

import (
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/store"
)

func assertAuthReset(t *testing.T, identity string, resolver store.AuthContextResolver, client any, initErr error, once *sync.Once) {
	t.Helper()
	assert.Empty(t, identity)
	assert.Nil(t, resolver)
	assert.Nil(t, client)
	assert.NoError(t, initErr)
	reinitialized := false
	once.Do(func() { reinitialized = true })
	assert.True(t, reinitialized, "reset must allow a new client initialization")
}

func TestSSMStoreResetAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := &SSMStore{
		identityName: "inherited", authResolver: store.NewMockAuthContextResolver(ctrl),
		client: NewMockSSMClient(ctrl), awsConfig: &aws.Config{}, initErr: store.ErrAuthContextNotAvailable,
	}
	s.initOnce.Do(func() {})
	s.ResetAuthContext()
	assertAuthReset(t, s.identityName, s.authResolver, s.client, s.initErr, &s.initOnce)
	assert.Nil(t, s.awsConfig)
}

func TestSecretsManagerStoreResetAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := &SecretsManagerStore{
		identityName: "inherited", authResolver: store.NewMockAuthContextResolver(ctrl),
		client: &secretsmanager.Client{}, initErr: store.ErrAuthContextNotAvailable,
	}
	s.initOnce.Do(func() {})
	s.ResetAuthContext()
	assertAuthReset(t, s.identityName, s.authResolver, s.client, s.initErr, &s.initOnce)
}

func TestAzureKeyVaultStoreResetAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := &AzureKeyVaultStore{
		identityName: "inherited", authResolver: store.NewMockAuthContextResolver(ctrl),
		client: &azsecrets.Client{}, initErr: store.ErrAuthContextNotAvailable,
	}
	s.initOnce.Do(func() {})
	s.ResetAuthContext()
	assertAuthReset(t, s.identityName, s.authResolver, s.client, s.initErr, &s.initOnce)
}

func TestGSMStoreResetAuthContext(t *testing.T) {
	for _, closeErr := range []error{nil, store.ErrCreateClient} {
		t.Run("closes previous client", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockGSMClient(ctrl)
			client.EXPECT().Close().Return(closeErr)
			s := &GSMStore{
				identityName: "inherited", authResolver: store.NewMockAuthContextResolver(ctrl),
				client: client, initErr: store.ErrAuthContextNotAvailable,
			}
			s.initOnce.Do(func() {})
			s.ResetAuthContext()
			assertAuthReset(t, s.identityName, s.authResolver, s.client, s.initErr, &s.initOnce)
			// A second reset must not close the same client again.
			s.ResetAuthContext()
		})
	}
}

func TestVaultStoreResetAuthContextPreservesTokenClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := &vaultKVv2Client{}
	s := &VaultStore{client: client, identityName: "inherited", authResolver: store.NewMockAuthContextResolver(ctrl)}
	s.ResetAuthContext()
	assert.Empty(t, s.identityName)
	assert.Nil(t, s.authResolver)
	assert.Same(t, client, s.client, "Vault token authentication does not use the Atmos cloud identity")
}
