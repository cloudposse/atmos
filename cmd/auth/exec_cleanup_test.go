package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the provider field used by cleanup's configuration lookup.
var _ = schema.Provider{Kind: types.ProviderKindGitLabOIDC}

// TestCleanupGitLabAuthExecScope verifies provider isolation and best-effort
// cleanup of every identity in the chain, without requiring a CI environment.
func TestCleanupGitLabAuthExecScope(t *testing.T) {
	tests := []struct {
		name      string
		chain     []string
		kind      string
		wantCalls []string
		fail      bool
	}{
		{name: "no chain"},
		{name: "standalone identity", chain: []string{"user"}},
		{name: "GitHub OIDC unchanged", chain: []string{"provider", "role"}, kind: "github/oidc"},
		{name: "SSO unchanged", chain: []string{"provider", "role"}, kind: "aws/iam-identity-center"},
		{name: "GitLab direct role", chain: []string{"provider", "role"}, kind: types.ProviderKindGitLabOIDC, wantCalls: []string{"role"}},
		{name: "GitLab chain continues after cleanup error", chain: []string{"provider", "runner", "role"}, kind: types.ProviderKindGitLabOIDC, wantCalls: []string{"role", "runner"}, fail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CI", "false")
			manager := types.NewMockAuthManager(gomock.NewController(t))
			manager.EXPECT().GetChain().Return(tc.chain)
			if len(tc.chain) >= 2 {
				manager.EXPECT().GetProviders().Return(map[string]schema.Provider{"provider": {Kind: tc.kind}})
			}
			var calls []any
			for i, identity := range tc.wantCalls {
				var logoutErr error
				if tc.fail && i == 0 {
					logoutErr = errUtils.ErrPartialLogout
				}
				calls = append(calls, manager.EXPECT().Logout(gomock.Any(), identity, false).Return(logoutErr))
			}
			gomock.InOrder(calls...)

			err := cleanupGitLabAuthExec(&authExecContext{authManager: manager, identity: "role"})

			if tc.fail {
				require.ErrorIs(t, err, errUtils.ErrPartialLogout)
				assert.ErrorContains(t, err, "failed to clean up GitLab OIDC identity")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCleanupGitLabAuthExecEmptyContext verifies that incomplete setup has no
// cleanup side effects when no authentication manager or identity is available.
func TestCleanupGitLabAuthExecEmptyContext(t *testing.T) {
	manager := types.NewMockAuthManager(gomock.NewController(t))
	for _, execCtx := range []*authExecContext{nil, {}, {identity: "role"}, {authManager: manager}} {
		require.NoError(t, cleanupGitLabAuthExec(execCtx))
	}
}
