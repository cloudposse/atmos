package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/store"
)

// TestService_SopsPlacements resolves the SOPS file placement for every SOPS-backed declared
// secret. The two declarations in the fixture share one file and are instance-scoped (the default).
func TestService_SopsPlacements(t *testing.T) {
	cfg, section, file := newSopsServiceConfig(t, true)
	svc := NewService(cfg, "dev", "api", section)

	placements := svc.SopsPlacements()
	require.Len(t, placements, 2, "both SOPS-backed secrets produce a placement")
	for _, p := range placements {
		assert.Equal(t, file, p.File)
		assert.Equal(t, "dev", p.Stack)
		assert.Equal(t, "api", p.Component)
		assert.Equal(t, ScopeInstance, p.Scope, "no explicit scope defaults to instance")
		assert.Contains(t, []string{"DATADOG_API_KEY", "REDIS_URL"}, p.Secret)
	}
}

// TestService_SopsPlacements_SkipsStoreBacked proves store-backed (non-SOPS) declarations
// contribute no placements.
func TestService_SopsPlacements_SkipsStoreBacked(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg, section := serviceTestConfig(store.NewMockStore(ctrl))
	svc := NewService(cfg, "prod", "api", section)

	assert.Empty(t, svc.SopsPlacements(), "store-backed secrets are not SOPS placements")
}

// TestDetectSopsCollisions_EmptyAndSingle proves the trivial inputs (no placements, one placement)
// never report a collision.
func TestDetectSopsCollisions_EmptyAndSingle(t *testing.T) {
	require.NoError(t, DetectSopsCollisions(nil))
	require.NoError(t, DetectSopsCollisions([]SopsPlacement{
		{Stack: "prod", Component: "api", Secret: "K", Scope: ScopeInstance, File: "secrets/prod.api.enc.yaml"},
	}))
}
