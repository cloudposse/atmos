package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSignaturePolicyRequiresScopedPublisherIdentities(t *testing.T) {
	valid := schema.PluginPublisher{Publisher: "acme", Identity: "https://github.com/acme/*", Issuer: "https://token.actions.githubusercontent.com"}
	for _, tc := range []struct {
		name      string
		publisher schema.PluginPublisher
	}{
		{"missing namespace", schema.PluginPublisher{Identity: valid.Identity, Issuer: valid.Issuer}},
		{"wildcard namespace", schema.PluginPublisher{Publisher: "*", Identity: valid.Identity, Issuer: valid.Issuer}},
		{"package instead of namespace", schema.PluginPublisher{Publisher: "acme/oidc", Identity: valid.Identity, Issuer: valid.Issuer}},
		{"missing identity", schema.PluginPublisher{Publisher: valid.Publisher, Issuer: valid.Issuer}},
		{"unrestricted identity", schema.PluginPublisher{Publisher: valid.Publisher, Identity: "*", Issuer: valid.Issuer}},
		{"missing issuer", schema.PluginPublisher{Publisher: valid.Publisher, Identity: valid.Identity}},
		{"wildcard issuer", schema.PluginPublisher{Publisher: valid.Publisher, Identity: valid.Identity, Issuer: "https://*.example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, Validate(&schema.PluginsConfig{Policy: schema.PluginPolicy{TrustedPublishers: []schema.PluginPublisher{tc.publisher}}}), errUtils.ErrPluginInvalidConfig)
			require.ErrorIs(t, Validate(&schema.PluginsConfig{Policy: schema.PluginPolicy{TrustedPublishers: []schema.PluginPublisher{valid}, PublisherConstraints: [][]schema.PluginPublisher{{tc.publisher}}}}), errUtils.ErrPluginInvalidConfig)
		})
	}
	require.ErrorIs(t, Validate(&schema.PluginsConfig{Policy: schema.PluginPolicy{RequireSignature: true}}), errUtils.ErrPluginInvalidConfig)
	require.NoError(t, Validate(&schema.PluginsConfig{Policy: schema.PluginPolicy{TrustedPublishers: []schema.PluginPublisher{valid}}}))
	require.NoError(t, Validate(&schema.PluginsConfig{Policy: schema.PluginPolicy{RequireSignature: true, PublisherConstraints: [][]schema.PluginPublisher{{valid}}}}))
}
