package eks

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// restoreEKSSeams snapshots the package seams and restores them on cleanup.
func restoreEKSSeams(t *testing.T) {
	t.Helper()
	origInit, origMgr := initCliConfig, newAuthManager
	t.Cleanup(func() { initCliConfig, newAuthManager = origInit, origMgr })
}

// okEKSInitConfig is a no-op config loader seam.
func okEKSInitConfig(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
	return schema.AtmosConfiguration{}, nil
}

var errEKSBoom = errors.New("boom")

// fakeEKSAuthManager embeds the (nil) AuthManager interface and overrides only
// the two methods these tests exercise, so we can drive the command without a
// live AWS account.
type fakeEKSAuthManager struct {
	types.AuthManager
	gotName      string
	gotOverrides types.IntegrationOverrides
	execErr      error
	whoami       *types.WhoamiInfo
	authErr      error
}

func (f *fakeEKSAuthManager) ExecuteIntegration(_ context.Context, name string, opts ...types.IntegrationOption) error {
	f.gotName = name
	for _, opt := range opts {
		opt(&f.gotOverrides)
	}
	return f.execErr
}

func (f *fakeEKSAuthManager) Authenticate(_ context.Context, name string) (*types.WhoamiInfo, error) {
	f.gotName = name
	return f.whoami, f.authErr
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_ForwardsOverrides is the
// regression guard for issue #3386 on the AWS side: --kubeconfig/--alias passed
// alongside --integration must reach ExecuteIntegration as overrides instead of
// being silently dropped.
func TestExecuteEKSUpdateKubeconfigViaIntegration_ForwardsOverrides(t *testing.T) {
	restoreEKSSeams(t)

	initCliConfig = okEKSInitConfig
	fake := &fakeEKSAuthManager{}
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return fake, nil
	}

	err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "/home/dev/.config/atmos/kube/prod", "prod-eks")
	require.NoError(t, err)

	assert.Equal(t, "prod/eks", fake.gotName)
	assert.Equal(t, "/home/dev/.config/atmos/kube/prod", fake.gotOverrides.KubeconfigPath)
	assert.Equal(t, "prod-eks", fake.gotOverrides.Alias)
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_EmptyOverrides confirms unset
// flags forward as empty overrides (no-ops), so the integration's configured
// values still win.
func TestExecuteEKSUpdateKubeconfigViaIntegration_EmptyOverrides(t *testing.T) {
	restoreEKSSeams(t)

	initCliConfig = okEKSInitConfig
	fake := &fakeEKSAuthManager{}
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return fake, nil
	}

	err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "", "")
	require.NoError(t, err)

	assert.Equal(t, "prod/eks", fake.gotName)
	assert.Empty(t, fake.gotOverrides.KubeconfigPath)
	assert.Empty(t, fake.gotOverrides.Alias)
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_PropagatesError confirms an
// integration execution error is surfaced.
func TestExecuteEKSUpdateKubeconfigViaIntegration_PropagatesError(t *testing.T) {
	restoreEKSSeams(t)

	initCliConfig = okEKSInitConfig
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return &fakeEKSAuthManager{execErr: errEKSBoom}, nil
	}

	err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errEKSBoom)
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_SetupErrors covers the config-load
// and auth-manager-construction failure branches.
func TestExecuteEKSUpdateKubeconfigViaIntegration_SetupErrors(t *testing.T) {
	tests := []struct {
		name      string
		initErr   error
		mgrErr    error
		wantErrIs error
	}{
		{name: "config init fails", initErr: errEKSBoom, wantErrIs: errUtils.ErrFailedToInitConfig},
		{name: "manager create fails", mgrErr: errEKSBoom, wantErrIs: errUtils.ErrFailedToInitializeAuthManager},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreEKSSeams(t)

			initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, tt.initErr
			}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return &fakeEKSAuthManager{}, tt.mgrErr
			}

			err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "", "")
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErrIs)
		})
	}
}

// TestExecuteEKSUpdateKubeconfigDirect_Errors drives the direct (non-integration)
// path's failure branches - including the nil-whoami guard that must not panic.
func TestExecuteEKSUpdateKubeconfigDirect_Errors(t *testing.T) {
	tests := []struct {
		name      string
		initErr   error
		mgrErr    error
		whoami    *types.WhoamiInfo
		authErr   error
		wantErrIs []error
	}{
		{name: "config init fails", initErr: errEKSBoom, wantErrIs: []error{errUtils.ErrFailedToInitConfig, errEKSBoom}},
		{name: "manager create fails", mgrErr: errEKSBoom, wantErrIs: []error{errUtils.ErrFailedToInitializeAuthManager, errEKSBoom}},
		{name: "authenticate fails", authErr: errEKSBoom, wantErrIs: []error{errUtils.ErrIdentityAuthFailed, errEKSBoom}},
		{name: "nil credentials", whoami: &types.WhoamiInfo{Credentials: nil}, wantErrIs: []error{errUtils.ErrIdentityAuthFailed}},
		// Authenticate returning (nil, nil) must be handled without a panic.
		{name: "nil whoami result", whoami: nil, wantErrIs: []error{errUtils.ErrIdentityAuthFailed}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreEKSSeams(t)

			initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, tt.initErr
			}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return &fakeEKSAuthManager{whoami: tt.whoami, authErr: tt.authErr}, tt.mgrErr
			}

			err := executeEKSUpdateKubeconfigDirect("dev-cluster", "us-east-2", "", "", "dev-admin")
			require.Error(t, err)
			for _, target := range tt.wantErrIs {
				assert.ErrorIs(t, err, target)
			}
		})
	}
}

// TestEKSSeamsDefaultToRealImplementations guards against an accidental nil seam.
func TestEKSSeamsDefaultToRealImplementations(t *testing.T) {
	assert.NotNil(t, initCliConfig)
	assert.NotNil(t, newAuthManager)
}
