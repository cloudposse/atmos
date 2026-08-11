package secrets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDetectSopsCollisions_OK proves distinct instance files and a consistently-shared stack file
// (the derive-in-code default) pass.
func TestDetectSopsCollisions_OK(t *testing.T) {
	placements := []SopsPlacement{
		{Stack: "prod", Component: "api", Secret: "K", Scope: ScopeInstance, File: "secrets/prod.api.enc.yaml"},
		{Stack: "prod", Component: "web", Secret: "K", Scope: ScopeInstance, File: "secrets/prod.web.enc.yaml"},
		// A stack-scoped secret resolves to the same shared file from every instance's vantage point.
		{Stack: "prod", Component: "api", Secret: "SHARED", Scope: ScopeStack, File: "secrets/prod.enc.yaml"},
		{Stack: "prod", Component: "web", Secret: "SHARED", Scope: ScopeStack, File: "secrets/prod.enc.yaml"},
	}
	require.NoError(t, DetectSopsCollisions(placements))
}

// TestDetectSopsCollisions_InstanceShareFile proves two distinct instances resolving to the same
// file (a non-discriminating spec.file template) is rejected.
func TestDetectSopsCollisions_InstanceShareFile(t *testing.T) {
	placements := []SopsPlacement{
		{Stack: "prod", Component: "api", Secret: "K", Scope: ScopeInstance, File: "secrets/prod.enc.yaml"},
		{Stack: "prod", Component: "web", Secret: "K", Scope: ScopeInstance, File: "secrets/prod.enc.yaml"},
	}
	require.ErrorIs(t, DetectSopsCollisions(placements), ErrSopsCollision)
}

// TestDetectSopsCollisions_StackNotShared proves a stack-scoped secret resolving to per-component
// files (so it would not actually be shared) is rejected.
func TestDetectSopsCollisions_StackNotShared(t *testing.T) {
	placements := []SopsPlacement{
		{Stack: "prod", Component: "api", Secret: "SHARED", Scope: ScopeStack, File: "secrets/prod.api.enc.yaml"},
		{Stack: "prod", Component: "web", Secret: "SHARED", Scope: ScopeStack, File: "secrets/prod.web.enc.yaml"},
	}
	require.ErrorIs(t, DetectSopsCollisions(placements), ErrSopsCollision)
}
