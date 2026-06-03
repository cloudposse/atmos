package auth

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFindIntegrationsForIdentity_ViaProvider(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"pro-id":   {Kind: "atmos/pro", Via: &schema.IdentityVia{Provider: "atmos-pro"}},
				"other-id": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "sso"}},
			},
			Integrations: map[string]schema.Integration{
				"gh-provider": {Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}},
				"gh-identity": {Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Identity: "pro-id"}},
				"ecr-other":   {Kind: integrations.KindAWSECR, Via: &schema.IntegrationVia{Identity: "other-id"}},
			},
		},
	}

	t.Run("matches via.provider by root provider and via.identity", func(t *testing.T) {
		got := m.findIntegrationsForIdentity("pro-id", false)
		sort.Strings(got)
		assert.Equal(t, []string{"gh-identity", "gh-provider"}, got)
	})

	t.Run("does not match a provider-bound integration for an unrelated identity", func(t *testing.T) {
		got := m.findIntegrationsForIdentity("other-id", false)
		assert.Equal(t, []string{"ecr-other"}, got)
	})
}

func TestResolveRevokeOnExit(t *testing.T) {
	tr, fa := true, false
	tests := []struct {
		name   string
		spec   *schema.IntegrationSpec
		global *bool
		want   bool
	}{
		{"spec true overrides global false", &schema.IntegrationSpec{RevokeOnExit: &tr}, &fa, true},
		{"spec false overrides global true", &schema.IntegrationSpec{RevokeOnExit: &fa}, &tr, false},
		{"global used when spec unset", &schema.IntegrationSpec{}, &fa, false},
		{"default true when both unset", nil, nil, true},
		{"default true when spec nil and global nil", nil, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &schema.Integration{Kind: integrations.KindGitHubSTS, Spec: tc.spec}
			assert.Equal(t, tc.want, resolveRevokeOnExit(cfg, tc.global))
		})
	}
}

func TestRevokeEphemeralIntegrations_KindFilterAndNoState(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"pro-id": {Kind: "atmos/pro", Via: &schema.IdentityVia{Provider: "atmos-pro"}},
			},
			Integrations: map[string]schema.Integration{
				// A malformed aws/ecr integration: it must be skipped by kind before Create,
				// so it never errors here.
				"ecr-bad": {Kind: integrations.KindAWSECR, Via: &schema.IntegrationVia{Identity: "pro-id"}},
				// github/sts with no minted state — Cleanup is an idempotent no-op (no HTTP).
				"gh": {Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}},
			},
		},
	}

	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	require.NoError(t, m.RevokeEphemeralIntegrations(context.Background(), "pro-id", nil))
}
