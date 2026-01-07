package exec

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	auth_types "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authContextWrapper is a minimal AuthManager implementation that only provides
// GetStackInfo() for passing AuthContext to ExecuteDescribeComponent.
// Other methods panic if called since this wrapper is only for propagating existing auth context.
type authContextWrapper struct {
	stackInfo *schema.ConfigAndStacksInfo
}

func (a *authContextWrapper) GetStackInfo() *schema.ConfigAndStacksInfo {
	defer perf.Track(nil, "exec.authContextWrapper.GetStackInfo")()

	return a.stackInfo
}

// Stub methods to satisfy AuthManager interface (not used by ExecuteDescribeComponent).
func (a *authContextWrapper) GetCachedCredentials(ctx context.Context, identityName string) (*auth_types.WhoamiInfo, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetCachedCredentials")()

	panic("authContextWrapper.GetCachedCredentials should not be called")
}

func (a *authContextWrapper) Authenticate(ctx context.Context, identityName string) (*auth_types.WhoamiInfo, error) {
	defer perf.Track(nil, "exec.authContextWrapper.Authenticate")()

	panic("authContextWrapper.Authenticate should not be called")
}

func (a *authContextWrapper) AuthenticateProvider(ctx context.Context, providerName string) (*auth_types.WhoamiInfo, error) {
	defer perf.Track(nil, "exec.authContextWrapper.AuthenticateProvider")()

	return nil, fmt.Errorf("%w: authContextWrapper.AuthenticateProvider for template context", errUtils.ErrNotImplemented)
}

func (a *authContextWrapper) Whoami(ctx context.Context, identityName string) (*auth_types.WhoamiInfo, error) {
	defer perf.Track(nil, "exec.authContextWrapper.Whoami")()

	panic("authContextWrapper.Whoami should not be called")
}

func (a *authContextWrapper) Validate() error {
	defer perf.Track(nil, "exec.authContextWrapper.Validate")()

	panic("authContextWrapper.Validate should not be called")
}

func (a *authContextWrapper) GetDefaultIdentity(forceSelect bool) (string, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetDefaultIdentity")()

	panic("authContextWrapper.GetDefaultIdentity should not be called")
}

func (a *authContextWrapper) ListProviders() []string {
	defer perf.Track(nil, "exec.authContextWrapper.ListProviders")()

	panic("authContextWrapper.ListProviders should not be called")
}

func (a *authContextWrapper) Logout(ctx context.Context, identityName string, deleteKeychain bool) error {
	defer perf.Track(nil, "exec.authContextWrapper.Logout")()

	panic("authContextWrapper.Logout should not be called")
}

func (a *authContextWrapper) GetChain() []string {
	defer perf.Track(nil, "exec.authContextWrapper.GetChain")()

	// Return empty slice instead of panicking.
	// This wrapper doesn't track the authentication chain; it only propagates auth context.
	// When used in resolveAuthManagerForNestedComponent, an empty chain means
	// no inherited identity, so the component will use its own defaults.
	return []string{}
}

func (a *authContextWrapper) ListIdentities() []string {
	defer perf.Track(nil, "exec.authContextWrapper.ListIdentities")()

	panic("authContextWrapper.ListIdentities should not be called")
}

func (a *authContextWrapper) GetProviderForIdentity(identityName string) string {
	defer perf.Track(nil, "exec.authContextWrapper.GetProviderForIdentity")()

	panic("authContextWrapper.GetProviderForIdentity should not be called")
}

func (a *authContextWrapper) GetFilesDisplayPath(providerName string) string {
	defer perf.Track(nil, "exec.authContextWrapper.GetFilesDisplayPath")()

	panic("authContextWrapper.GetFilesDisplayPath should not be called")
}

func (a *authContextWrapper) GetProviderKindForIdentity(identityName string) (string, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetProviderKindForIdentity")()

	panic("authContextWrapper.GetProviderKindForIdentity should not be called")
}

func (a *authContextWrapper) GetIdentities() map[string]schema.Identity {
	defer perf.Track(nil, "exec.authContextWrapper.GetIdentities")()

	panic("authContextWrapper.GetIdentities should not be called")
}

func (a *authContextWrapper) GetProviders() map[string]schema.Provider {
	defer perf.Track(nil, "exec.authContextWrapper.GetProviders")()

	panic("authContextWrapper.GetProviders should not be called")
}

func (a *authContextWrapper) LogoutProvider(ctx context.Context, providerName string, deleteKeychain bool) error {
	defer perf.Track(nil, "exec.authContextWrapper.LogoutProvider")()

	panic("authContextWrapper.LogoutProvider should not be called")
}

func (a *authContextWrapper) LogoutAll(ctx context.Context, deleteKeychain bool) error {
	defer perf.Track(nil, "exec.authContextWrapper.LogoutAll")()

	panic("authContextWrapper.LogoutAll should not be called")
}

func (a *authContextWrapper) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetEnvironmentVariables")()

	panic("authContextWrapper.GetEnvironmentVariables should not be called")
}

func (a *authContextWrapper) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	defer perf.Track(nil, "exec.authContextWrapper.PrepareShellEnvironment")()

	panic("authContextWrapper.PrepareShellEnvironment should not be called")
}

func (a *authContextWrapper) ExecuteIntegration(ctx context.Context, integrationName string) error {
	defer perf.Track(nil, "exec.authContextWrapper.ExecuteIntegration")()

	panic("authContextWrapper.ExecuteIntegration should not be called")
}

func (a *authContextWrapper) ExecuteIdentityIntegrations(ctx context.Context, identityName string) error {
	defer perf.Track(nil, "exec.authContextWrapper.ExecuteIdentityIntegrations")()

	panic("authContextWrapper.ExecuteIdentityIntegrations should not be called")
}

func (a *authContextWrapper) GetIntegration(integrationName string) (*schema.Integration, error) {
	defer perf.Track(nil, "exec.authContextWrapper.GetIntegration")()

	panic("authContextWrapper.GetIntegration should not be called")
}

// newAuthContextWrapper creates an AuthManager wrapper that returns the given AuthContext.
func newAuthContextWrapper(authContext *schema.AuthContext) *authContextWrapper {
	if authContext == nil {
		return nil
	}
	return &authContextWrapper{
		stackInfo: &schema.ConfigAndStacksInfo{
			AuthContext: authContext,
		},
	}
}
