package auth

//go:generate go run go.uber.org/mock/mockgen -destination mock_integration_test.go -package auth github.com/cloudposse/atmos/pkg/auth/integrations Integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema fields the logout fixture configures.
var _ = schema.Integration{Kind: "", Via: &schema.IntegrationVia{Identity: ""}}

// testCleanupIntegrationKind is a kind only these tests register, so no real integration is replaced.
const testCleanupIntegrationKind = "test/logout-cleanup"

// registerCleanupIntegration registers a mocked integration whose Cleanup returns cleanupErr,
// links it to identityName in the fixture's config, and returns the mock.
func registerCleanupIntegration(t *testing.T, f *logoutKeyringFixture, identityName string, cleanupErr error) {
	t.Helper()
	integration := NewMockIntegration(gomock.NewController(t))
	integration.EXPECT().Cleanup(gomock.Any()).Return(cleanupErr).Times(1)
	integrations.Register(testCleanupIntegrationKind, func(*integrations.IntegrationConfig) (integrations.Integration, error) {
		return integration, nil
	})
	f.m.config.Integrations = map[string]schema.Integration{
		"registry": {Kind: testCleanupIntegrationKind, Via: &schema.IntegrationVia{Identity: identityName}},
	}
}

// TestManager_Logout_IntegrationCleanupFailureIsReported reproduces the 1.235.0 upgrade
// regression where `atmos auth logout` reported success although the ECR/ACR credential could not
// be removed from Docker's config.json. A failed integration cleanup now makes the logout partial
// and carries the cleanup error, so the user is told what was left behind.
func TestManager_Logout_IntegrationCleanupFailureIsReported(t *testing.T) {
	f := newLogoutKeyringFixture(t, "dev-user")
	registerCleanupIntegration(t, f, "dev-user", errUtils.ErrDockerConfigWrite)

	err := f.m.Logout(context.Background(), "dev-user", false)

	require.ErrorIs(t, err, errUtils.ErrPartialLogout)
	require.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.Contains(t, err.Error(), "registry", "the error names the integration")
}

// TestManager_Logout_IntegrationCleanupSuccessIsComplete is the negative path: a cleanup that
// succeeds keeps the logout complete.
func TestManager_Logout_IntegrationCleanupSuccessIsComplete(t *testing.T) {
	f := newLogoutKeyringFixture(t, "dev-user")
	registerCleanupIntegration(t, f, "dev-user", nil)

	require.NoError(t, f.m.Logout(context.Background(), "dev-user", false))
}
