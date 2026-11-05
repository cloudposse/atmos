package auth

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compose generated mocks so auth methods remain strict, unexpected calls.
type (
	contractProvider struct {
		*types.MockProvider
		*types.MockCredentialContractProvider
	}
	contractIdentity struct {
		*types.MockIdentity
		*types.MockCredentialContractProvider
	}
)

func TestCredentialContractMismatchFailsBeforeAuthentication(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)
	controller := gomock.NewController(t)
	provider := &contractProvider{types.NewMockProvider(controller), types.NewMockCredentialContractProvider(controller)}
	identity := &contractIdentity{types.NewMockIdentity(controller), types.NewMockCredentialContractProvider(controller)}
	identity.MockIdentity.EXPECT().Kind().Return("fixture")
	identity.MockIdentity.EXPECT().Validate().Return(nil)
	provider.MockCredentialContractProvider.EXPECT().CredentialContract().Return(types.CredentialContract{Produces: []string{"oidc/v1"}})
	identity.MockCredentialContractProvider.EXPECT().CredentialContract().Return(types.CredentialContract{Accepts: []string{"aws/v1"}, Produces: []string{"aws/v1"}})
	manager := newPersistenceTestManager(&testStore{data: map[string]any{}}, map[string]types.Identity{"target": identity})
	manager.providers["source"] = provider
	manager.chain = []string{"source", "target"}
	_, err := manager.authenticateChain(context.Background(), "target")
	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
	require.Contains(t, err.Error(), "does not accept oidc/v1")
}

func TestCredentialContractsAllowCompatibleAndLegacySteps(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	provider := &contractProvider{types.NewMockProvider(controller), types.NewMockCredentialContractProvider(controller)}
	identity := &contractIdentity{types.NewMockIdentity(controller), types.NewMockCredentialContractProvider(controller)}
	provider.MockCredentialContractProvider.EXPECT().CredentialContract().Return(types.CredentialContract{Produces: []string{"oidc/v1"}})
	identity.MockCredentialContractProvider.EXPECT().CredentialContract().Return(types.CredentialContract{Accepts: []string{"oidc/v1"}, Produces: []string{"aws/v1"}})
	manager := &manager{chain: []string{"source", "broker", "legacy"}, providers: map[string]types.Provider{"source": provider}, identities: map[string]types.Identity{"broker": identity, "legacy": types.NewMockIdentity(controller)}}
	require.NoError(t, manager.validateChainCredentialContracts())
}

// Compose generated mocks to verify the optional transaction contract without implementing it in a fake.
type transactionProvider struct {
	*types.MockProvider
	*types.MockCredentialTransaction
}

type logoutTransactionKey struct{}

func TestLogoutAllKeepsProviderCleanupInsideTransaction(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	controller := gomock.NewController(t)
	provider := &transactionProvider{types.NewMockProvider(controller), types.NewMockCredentialTransaction(controller)}
	store := types.NewMockCredentialStore(controller)
	m := &manager{config: &schema.AuthConfig{}, providers: map[string]types.Provider{"source": provider}, credentialStore: store}
	writer, err := types.NewProvisioningWriter()
	require.NoError(t, err)
	cache, err := writer.Write(&types.ProvisioningResult{Provider: "source"})
	require.NoError(t, err)
	active := false
	provider.MockCredentialTransaction.EXPECT().WithCredentialTransaction(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, operation func(context.Context) error) error {
		active = true
		defer func() { active = false }()
		err := operation(context.WithValue(ctx, logoutTransactionKey{}, true))
		require.NoFileExists(t, cache, "provisioned cache cleanup must complete before releasing the transaction")
		return err
	})
	store.EXPECT().Delete("source", "").DoAndReturn(func(string, string) error {
		require.True(t, active, "keyring cleanup must hold the provider transaction")
		return nil
	})
	provider.MockProvider.EXPECT().Logout(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		require.True(t, active)
		require.Equal(t, true, ctx.Value(logoutTransactionKey{}), "provider logout must receive the transaction lease context")
		require.FileExists(t, cache)
		return nil
	})
	require.NoError(t, m.LogoutAll(t.Context(), true))
	require.False(t, active)
}

func TestLogoutAllPreservesTransactionAndCleanupErrors(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	controller := gomock.NewController(t)
	unavailable := &transactionProvider{types.NewMockProvider(controller), types.NewMockCredentialTransaction(controller)}
	failing := &transactionProvider{types.NewMockProvider(controller), types.NewMockCredentialTransaction(controller)}
	store := types.NewMockCredentialStore(controller)
	m := &manager{config: &schema.AuthConfig{}, providers: map[string]types.Provider{"unavailable": unavailable, "failing": failing}, credentialStore: store}
	// An acquisition failure must skip that provider's cleanup and remain visible to callers.
	unavailable.MockCredentialTransaction.EXPECT().WithCredentialTransaction(gomock.Any(), gomock.Any()).Return(context.DeadlineExceeded)
	failing.MockCredentialTransaction.EXPECT().WithCredentialTransaction(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, operation func(context.Context) error) error { return operation(ctx) })
	store.EXPECT().Delete("failing", "").Return(os.ErrPermission)
	failing.MockProvider.EXPECT().Logout(gomock.Any()).Return(io.ErrClosedPipe)
	err := m.LogoutAll(t.Context(), true)
	require.ErrorIs(t, err, errUtils.ErrLogoutFailed)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, errUtils.ErrKeyringDeletion)
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorIs(t, err, errUtils.ErrProviderLogout)
	require.ErrorIs(t, err, io.ErrClosedPipe)
}
