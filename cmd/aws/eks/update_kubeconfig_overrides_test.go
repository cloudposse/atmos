package eks

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeEKSAuthManager embeds the (nil) AuthManager interface and overrides only
// ExecuteIntegration so we can capture the overrides forwarded by the command
// without a live AWS account.
type fakeEKSAuthManager struct {
	types.AuthManager
	gotName      string
	gotOverrides types.IntegrationOverrides
	execErr      error
}

func (f *fakeEKSAuthManager) ExecuteIntegration(_ context.Context, name string, opts ...types.IntegrationOption) error {
	f.gotName = name
	for _, opt := range opts {
		opt(&f.gotOverrides)
	}
	return f.execErr
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_ForwardsOverrides is the
// regression guard for issue #3386 on the AWS side: --kubeconfig/--alias passed
// alongside --integration must reach ExecuteIntegration as overrides instead of
// being silently dropped.
func TestExecuteEKSUpdateKubeconfigViaIntegration_ForwardsOverrides(t *testing.T) {
	origInit, origMgr := initCliConfig, newAuthManager
	t.Cleanup(func() { initCliConfig, newAuthManager = origInit, origMgr })

	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
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
	origInit, origMgr := initCliConfig, newAuthManager
	t.Cleanup(func() { initCliConfig, newAuthManager = origInit, origMgr })

	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
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
	origInit, origMgr := initCliConfig, newAuthManager
	t.Cleanup(func() { initCliConfig, newAuthManager = origInit, origMgr })

	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	boom := errors.New("boom")
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return &fakeEKSAuthManager{execErr: boom}, nil
	}

	err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// TestEKSSeamsDefaultToRealImplementations guards against an accidental nil seam.
func TestEKSSeamsDefaultToRealImplementations(t *testing.T) {
	assert.NotNil(t, initCliConfig)
	assert.NotNil(t, newAuthManager)
}
