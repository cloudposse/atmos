package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func TestRevokeEphemeralAuthExec(t *testing.T) {
	t.Run("nil execCtx is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() { revokeEphemeralAuthExec(nil) })
	})

	t.Run("nil auth manager is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() { revokeEphemeralAuthExec(&authExecContext{identity: "id"}) })
	})

	t.Run("empty identity is a no-op", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		m := authTypes.NewMockAuthManager(ctrl)
		// No EXPECT: RevokeEphemeralIntegrations must not be called.
		revokeEphemeralAuthExec(&authExecContext{authManager: m, identity: ""})
	})

	t.Run("not in CI is a no-op", func(t *testing.T) {
		preserved := telemetry.PreserveCIEnvVars()
		t.Cleanup(func() { telemetry.RestoreCIEnvVars(preserved) })

		ctrl := gomock.NewController(t)
		m := authTypes.NewMockAuthManager(ctrl)
		// No EXPECT: outside CI the broker must not revoke.
		revokeEphemeralAuthExec(&authExecContext{authManager: m, identity: "id"})
	})

	t.Run("in CI revokes", func(t *testing.T) {
		t.Setenv("CI", "true")
		ctrl := gomock.NewController(t)
		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().RevokeEphemeralIntegrations(gomock.Any(), "id", gomock.Any()).Return(nil)

		revokeEphemeralAuthExec(&authExecContext{authManager: m, identity: "id"})
	})

	t.Run("in CI a revoke error is swallowed", func(t *testing.T) {
		t.Setenv("CI", "true")
		ctrl := gomock.NewController(t)
		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().RevokeEphemeralIntegrations(gomock.Any(), "id", gomock.Any()).Return(errors.New("boom"))

		assert.NotPanics(t, func() { revokeEphemeralAuthExec(&authExecContext{authManager: m, identity: "id"}) })
	})
}
