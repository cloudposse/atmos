package deferred

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ auth.AuthManager = (*Manager)(nil)

// base constructs the ordinary manager only for a requested auth operation.
// Scope-aware evaluation uses Resolve and its invocation cache instead.
func (m *Manager) base() (auth.AuthManager, error) {
	m.baseMu.Lock()
	defer m.baseMu.Unlock()
	if m.disabled {
		return nil, errUtils.ErrAuthenticationUnavailable
	}
	if !m.baseReady {
		m.baseReady = true
		if m.config == nil {
			m.baseError = errUtils.ErrInvalidAuthConfig
		} else {
			m.baseManager, m.baseError = auth.NewDefaultManager(&m.config.Auth, m.config.CliConfigPath)
		}
	}
	return m.baseManager, m.baseError
}

// GetStackInfo never initializes or authenticates an unused manager.
func (m *Manager) GetStackInfo() *schema.ConfigAndStacksInfo {
	m.baseMu.Lock()
	defer m.baseMu.Unlock()
	if m.baseManager == nil {
		return nil
	}
	return m.baseManager.GetStackInfo()
}

// GetChain never initializes or authenticates an unused manager.
func (m *Manager) GetChain() []string {
	m.baseMu.Lock()
	defer m.baseMu.Unlock()
	if m.baseManager == nil {
		return nil
	}
	return m.baseManager.GetChain()
}

// GetCachedCredentials delegates an explicitly requested auth operation.
func (m *Manager) GetCachedCredentials(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.deferred.Manager.GetCachedCredentials")()

	delegate, err := m.base()
	if err != nil {
		return nil, err
	}
	return delegate.GetCachedCredentials(ctx, identityName)
}

// Authenticate delegates an explicitly requested auth operation.
func (m *Manager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.deferred.Manager.Authenticate")()

	delegate, err := m.base()
	if err != nil {
		return nil, err
	}
	return delegate.Authenticate(ctx, identityName)
}

// AuthenticateProvider delegates an explicitly requested auth operation.
func (m *Manager) AuthenticateProvider(ctx context.Context, providerName string) (*types.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.deferred.Manager.AuthenticateProvider")()

	delegate, err := m.base()
	if err != nil {
		return nil, err
	}
	return delegate.AuthenticateProvider(ctx, providerName)
}

// Whoami delegates an explicitly requested auth operation.
func (m *Manager) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.deferred.Manager.Whoami")()

	delegate, err := m.base()
	if err != nil {
		return nil, err
	}
	return delegate.Whoami(ctx, identityName)
}

// Validate delegates an explicitly requested auth operation.
func (m *Manager) Validate() error {
	defer perf.Track(nil, "auth.deferred.Manager.Validate")()

	delegate, err := m.base()
	if err != nil {
		return err
	}
	return delegate.Validate()
}

// GetDefaultIdentity delegates an explicitly requested auth operation.
func (m *Manager) GetDefaultIdentity(forceSelect bool) (string, error) {
	defer perf.Track(nil, "auth.deferred.Manager.GetDefaultIdentity")()

	delegate, err := m.base()
	if err != nil {
		return "", err
	}
	return delegate.GetDefaultIdentity(forceSelect)
}

// ListIdentities delegates an explicitly requested auth operation.
func (m *Manager) ListIdentities() []string {
	defer perf.Track(nil, "auth.deferred.Manager.ListIdentities")()

	delegate, err := m.base()
	if err != nil {
		return nil
	}
	return delegate.ListIdentities()
}

// GetProviderForIdentity delegates an explicitly requested auth operation.
func (m *Manager) GetProviderForIdentity(identityName string) string {
	defer perf.Track(nil, "auth.deferred.Manager.GetProviderForIdentity")()

	delegate, err := m.base()
	if err != nil {
		return ""
	}
	return delegate.GetProviderForIdentity(identityName)
}

// GetFilesDisplayPath delegates an explicitly requested auth operation.
func (m *Manager) GetFilesDisplayPath(providerName string) string {
	defer perf.Track(nil, "auth.deferred.Manager.GetFilesDisplayPath")()

	delegate, err := m.base()
	if err != nil {
		return ""
	}
	return delegate.GetFilesDisplayPath(providerName)
}

// GetProviderKindForIdentity delegates an explicitly requested auth operation.
func (m *Manager) GetProviderKindForIdentity(identityName string) (string, error) {
	defer perf.Track(nil, "auth.deferred.Manager.GetProviderKindForIdentity")()

	delegate, err := m.base()
	if err != nil {
		return "", err
	}
	return delegate.GetProviderKindForIdentity(identityName)
}

// GetRealm delegates an explicitly requested auth operation.
func (m *Manager) GetRealm() realm.RealmInfo {
	defer perf.Track(nil, "auth.deferred.Manager.GetRealm")()

	delegate, err := m.base()
	if err != nil {
		return realm.RealmInfo{}
	}
	return delegate.GetRealm()
}

// CredentialStoreType delegates an explicitly requested auth operation.
func (m *Manager) CredentialStoreType() string {
	defer perf.Track(nil, "auth.deferred.Manager.CredentialStoreType")()

	delegate, err := m.base()
	if err != nil {
		return ""
	}
	return delegate.CredentialStoreType()
}

// ListProviders delegates an explicitly requested auth operation.
func (m *Manager) ListProviders() []string {
	defer perf.Track(nil, "auth.deferred.Manager.ListProviders")()

	delegate, err := m.base()
	if err != nil {
		return nil
	}
	return delegate.ListProviders()
}

// GetIdentities delegates an explicitly requested auth operation.
func (m *Manager) GetIdentities() map[string]schema.Identity {
	defer perf.Track(nil, "auth.deferred.Manager.GetIdentities")()

	delegate, err := m.base()
	if err != nil {
		return nil
	}
	return delegate.GetIdentities()
}

// GetProviders delegates an explicitly requested auth operation.
func (m *Manager) GetProviders() map[string]schema.Provider {
	defer perf.Track(nil, "auth.deferred.Manager.GetProviders")()

	delegate, err := m.base()
	if err != nil {
		return nil
	}
	return delegate.GetProviders()
}

// Logout delegates an explicitly requested auth operation.
func (m *Manager) Logout(ctx context.Context, identityName string, deleteKeychain bool) error {
	defer perf.Track(nil, "auth.deferred.Manager.Logout")()

	delegate, err := m.base()
	if err != nil {
		return err
	}
	return delegate.Logout(ctx, identityName, deleteKeychain)
}

// LogoutProvider delegates an explicitly requested auth operation.
func (m *Manager) LogoutProvider(ctx context.Context, providerName string, deleteKeychain bool) error {
	defer perf.Track(nil, "auth.deferred.Manager.LogoutProvider")()

	delegate, err := m.base()
	if err != nil {
		return err
	}
	return delegate.LogoutProvider(ctx, providerName, deleteKeychain)
}

// LogoutAll delegates an explicitly requested auth operation.
func (m *Manager) LogoutAll(ctx context.Context, deleteKeychain bool) error {
	defer perf.Track(nil, "auth.deferred.Manager.LogoutAll")()

	delegate, err := m.base()
	if err != nil {
		return err
	}
	return delegate.LogoutAll(ctx, deleteKeychain)
}

// GetEnvironmentVariables delegates an explicitly requested auth operation.
func (m *Manager) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	defer perf.Track(nil, "auth.deferred.Manager.GetEnvironmentVariables")()

	delegate, err := m.base()
	if err != nil {
		return nil, err
	}
	return delegate.GetEnvironmentVariables(identityName)
}

// PrepareShellEnvironment delegates an explicitly requested auth operation.
func (m *Manager) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	defer perf.Track(nil, "auth.deferred.Manager.PrepareShellEnvironment")()

	delegate, err := m.base()
	if err != nil {
		return nil, err
	}
	return delegate.PrepareShellEnvironment(ctx, identityName, currentEnv)
}

// EnsureIdentityEnvironment delegates an explicitly requested auth operation.
func (m *Manager) EnsureIdentityEnvironment(ctx context.Context, identityName string) (map[string]string, error) {
	defer perf.Track(nil, "auth.deferred.Manager.EnsureIdentityEnvironment")()

	delegate, err := m.base()
	if err != nil {
		return nil, err
	}
	return delegate.EnsureIdentityEnvironment(ctx, identityName)
}

// ExecuteIntegration delegates an explicitly requested auth operation.
func (m *Manager) ExecuteIntegration(ctx context.Context, integrationName string) error {
	defer perf.Track(nil, "auth.deferred.Manager.ExecuteIntegration")()

	delegate, err := m.base()
	if err != nil {
		return err
	}
	return delegate.ExecuteIntegration(ctx, integrationName)
}

// ExecuteIdentityIntegrations delegates an explicitly requested auth operation.
func (m *Manager) ExecuteIdentityIntegrations(ctx context.Context, identityName string) error {
	defer perf.Track(nil, "auth.deferred.Manager.ExecuteIdentityIntegrations")()

	delegate, err := m.base()
	if err != nil {
		return err
	}
	return delegate.ExecuteIdentityIntegrations(ctx, identityName)
}

// GetIntegration delegates an explicitly requested auth operation.
func (m *Manager) GetIntegration(integrationName string) (*schema.Integration, error) {
	defer perf.Track(nil, "auth.deferred.Manager.GetIntegration")()

	delegate, err := m.base()
	if err != nil {
		return nil, err
	}
	return delegate.GetIntegration(integrationName)
}

// RevokeEphemeralIntegrations delegates an explicitly requested auth operation.
func (m *Manager) RevokeEphemeralIntegrations(ctx context.Context, identityName string, globalDefault *bool) error {
	defer perf.Track(nil, "auth.deferred.Manager.RevokeEphemeralIntegrations")()

	delegate, err := m.base()
	if err != nil {
		return err
	}
	return delegate.RevokeEphemeralIntegrations(ctx, identityName, globalDefault)
}

// ResolvePrincipalSetting delegates an explicitly requested auth operation.
func (m *Manager) ResolvePrincipalSetting(identityName, key string) (interface{}, bool) {
	defer perf.Track(nil, "auth.deferred.Manager.ResolvePrincipalSetting")()

	delegate, err := m.base()
	if err != nil {
		return nil, false
	}
	return delegate.ResolvePrincipalSetting(identityName, key)
}

// ResolveProviderConfig delegates an explicitly requested auth operation.
func (m *Manager) ResolveProviderConfig(identityName string) (*schema.Provider, bool) {
	defer perf.Track(nil, "auth.deferred.Manager.ResolveProviderConfig")()

	delegate, err := m.base()
	if err != nil {
		return nil, false
	}
	return delegate.ResolveProviderConfig(identityName)
}

// MaybeOfferAnyProfileFallback delegates an explicitly requested auth operation.
func (m *Manager) MaybeOfferAnyProfileFallback(ctx context.Context) error {
	defer perf.Track(nil, "auth.deferred.Manager.MaybeOfferAnyProfileFallback")()

	delegate, err := m.base()
	if err != nil {
		return err
	}
	return delegate.MaybeOfferAnyProfileFallback(ctx)
}
