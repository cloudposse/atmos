package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the provider configuration fields used below.
var _ = schema.Provider{Kind: types.ProviderKindGitLabOIDC, Spec: map[string]any{}}

// TestValidateGitLabOIDCProvider checks registry-backed configuration validation without requiring a runtime
// GitLab token.
func TestValidateGitLabOIDCProvider(t *testing.T) {
	t.Setenv("ATMOS_OIDC_TOKEN", "")
	tests := []struct {
		name string
		spec map[string]any
		fail bool
	}{
		{name: "default source without runtime token"},
		{name: "custom source", spec: map[string]any{"token_environment_variable": "GITLAB_ID_TOKEN"}},
		{name: "invalid source", spec: map[string]any{"token_environment_variable": false}, fail: true},
		{name: "invalid variable name", spec: map[string]any{"token_environment_variable": "TOKEN=NAME"}, fail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := NewValidator().ValidateProvider("gitlab", &schema.Provider{Kind: types.ProviderKindGitLabOIDC, Spec: tc.spec})
			if tc.fail {
				require.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
				return
			}
			require.NoError(t, err)
		})
	}
}
