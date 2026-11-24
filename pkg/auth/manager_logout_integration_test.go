package auth

//go:generate go run go.uber.org/mock/mockgen -destination mock_integration_test.go -package auth github.com/cloudposse/atmos/pkg/auth/integrations Integration

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema fields the logout fixture configures.
var _ = schema.Integration{Kind: "", Via: &schema.IntegrationVia{Identity: ""}}

// testCleanupIntegrationKind prefixes the kinds only these tests register, so no real
// integration is replaced.
const testCleanupIntegrationKind = "test/logout-cleanup"

// registerCleanupIntegration registers, under its own kind, a mocked integration named name whose
// Cleanup must run exactly once and returns cleanupErr, and links it to identityName in the
// fixture's config next to the integrations already linked.
func registerCleanupIntegration(t *testing.T, f *logoutKeyringFixture, identityName, name string, cleanupErr error) {
	t.Helper()
	integration := NewMockIntegration(gomock.NewController(t))
	integration.EXPECT().Cleanup(gomock.Any()).Return(cleanupErr).Times(1)
	kind := testCleanupIntegrationKind + "/" + name
	integrations.Register(kind, func(*integrations.IntegrationConfig) (integrations.Integration, error) {
		return integration, nil
	})
	if f.m.config.Integrations == nil {
		f.m.config.Integrations = map[string]schema.Integration{}
	}
	f.m.config.Integrations[name] = schema.Integration{Kind: kind, Via: &schema.IntegrationVia{Identity: identityName}}
}

// expectSingleIdentityLogout replaces the fixture's identity, whose Logout may run any number of
// times, with one whose Logout must run exactly once.
func expectSingleIdentityLogout(t *testing.T, f *logoutKeyringFixture, identityName string) {
	t.Helper()
	identity := types.NewMockIdentity(gomock.NewController(t))
	identity.EXPECT().Logout(gomock.Any()).Return(nil).Times(1)
	f.m.identities[identityName] = identity
}

// errSecondCleanup is the cleanup failure of the second integration in these tests.
var errSecondCleanup = errors.New("second cleanup failed")

// TestManager_Logout_IntegrationCleanupFailureIsReported reproduces the 1.235.0 upgrade
// regression where `atmos auth logout` reported success although the ECR/ACR credential could not
// be removed from Docker's config.json. A failed integration cleanup now makes the logout partial
// and carries the cleanup error, so the user is told what was left behind. A failure does not stop
// the logout: every linked integration is still cleaned up once, the identity is logged out once,
// and every failure is reported.
func TestManager_Logout_IntegrationCleanupFailureIsReported(t *testing.T) {
	f := newLogoutKeyringFixture(t, "dev-user")
	expectSingleIdentityLogout(t, f, "dev-user")
	registerCleanupIntegration(t, f, "dev-user", "registry", errUtils.ErrDockerConfigWrite)
	registerCleanupIntegration(t, f, "dev-user", "cluster", errSecondCleanup)

	err := f.m.Logout(context.Background(), "dev-user", false)

	require.ErrorIs(t, err, errUtils.ErrPartialLogout)
	require.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	require.ErrorIs(t, err, errSecondCleanup)
	assert.Contains(t, err.Error(), "registry", "the error names the integration")
	assert.Contains(t, err.Error(), "cluster")
}

// TestManager_Logout_IntegrationCleanupSuccessIsComplete is the negative path: a cleanup that
// succeeds keeps the logout complete.
func TestManager_Logout_IntegrationCleanupSuccessIsComplete(t *testing.T) {
	f := newLogoutKeyringFixture(t, "dev-user")
	expectSingleIdentityLogout(t, f, "dev-user")
	registerCleanupIntegration(t, f, "dev-user", "registry", nil)

	require.NoError(t, f.m.Logout(context.Background(), "dev-user", false))
}
