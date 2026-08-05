package broker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	atmosproIdentities "github.com/cloudposse/atmos/pkg/auth/identities/atmospro"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	atmosproProviders "github.com/cloudposse/atmos/pkg/auth/providers/atmospro"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards: a rename of these kind constants breaks the build immediately.
var (
	_ = schema.Provider{Kind: atmosproProviders.Kind}
	_ = schema.Identity{Kind: atmosproIdentities.IdentityKind}
	_ = schema.Integration{Kind: integrations.KindGitHubSTS}
)

func boolPtr(b bool) *bool { return &b }

// proAuthConfig returns a baseline atmos/pro provider + identity, with the supplied integration.
func proAuthConfig(integration schema.Integration) schema.AuthConfig {
	return schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"atmos-pro": {Kind: atmosproProviders.Kind},
		},
		Identities: map[string]schema.Identity{
			"atmos-pro": {Kind: atmosproIdentities.IdentityKind, Via: &schema.IdentityVia{Provider: "atmos-pro"}},
		},
		Integrations: map[string]schema.Integration{
			"github-sts": integration,
		},
	}
}

func TestFindProGitHubSTSIdentity(t *testing.T) {
	tests := []struct {
		name string
		auth schema.AuthConfig
		want string
	}{
		{
			name: "via provider resolves to the atmos/pro identity",
			auth: proAuthConfig(schema.Integration{Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}}),
			want: "atmos-pro",
		},
		{
			name: "via identity resolves directly",
			auth: proAuthConfig(schema.Integration{Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Identity: "atmos-pro"}}),
			want: "atmos-pro",
		},
		{
			name: "auto_provision false is skipped",
			auth: proAuthConfig(schema.Integration{Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}, Spec: &schema.IntegrationSpec{AutoProvision: boolPtr(false)}}),
			want: "",
		},
		{
			name: "auto_provision explicit true is honored",
			auth: proAuthConfig(schema.Integration{Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}, Spec: &schema.IntegrationSpec{AutoProvision: boolPtr(true)}}),
			want: "atmos-pro",
		},
		{
			name: "no integrations configured",
			auth: schema.AuthConfig{},
			want: "",
		},
		{
			name: "non-github/sts integration is ignored",
			auth: proAuthConfig(schema.Integration{Kind: integrations.KindAWSECR, Via: &schema.IntegrationVia{Provider: "atmos-pro"}}),
			want: "",
		},
		{
			name: "via.identity pointing at a non-pro identity is rejected",
			auth: func() schema.AuthConfig {
				c := proAuthConfig(schema.Integration{Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Identity: "aws-admin"}})
				c.Identities["aws-admin"] = schema.Identity{Kind: "aws/assume-role"}
				return c
			}(),
			want: "",
		},
		{
			name: "via.provider pointing at a non-pro provider is rejected",
			auth: func() schema.AuthConfig {
				c := proAuthConfig(schema.Integration{Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "not-pro"}})
				c.Providers["not-pro"] = schema.Provider{Kind: "aws/iam-identity-center"}
				return c
			}(),
			want: "",
		},
		{
			name: "via.provider with no identity routing through it",
			auth: schema.AuthConfig{
				Providers:    map[string]schema.Provider{"atmos-pro": {Kind: atmosproProviders.Kind}},
				Integrations: map[string]schema.Integration{"github-sts": {Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}}},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findProGitHubSTSIdentity(&schema.AtmosConfiguration{Auth: tt.auth})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProBroker_Enabled(t *testing.T) {
	b := proBroker{}

	// Force CI so the gate depends only on configuration presence.
	t.Setenv("CI", "true")

	withSTS := &schema.AtmosConfiguration{
		Auth: proAuthConfig(schema.Integration{Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}}),
	}
	assert.True(t, b.Enabled(withSTS), "enabled in CI when atmos/pro + github/sts configured")

	// No github/sts configuration: disabled regardless of CI.
	assert.False(t, b.Enabled(&schema.AtmosConfiguration{}), "disabled without github/sts config")
}

func TestProBroker_Name(t *testing.T) {
	assert.Equal(t, "atmos-pro/github-sts", proBroker{}.Name())
}
