package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetIntegration(t *testing.T) {
	t.Run("nil integrations", func(t *testing.T) {
		m := &manager{config: &schema.AuthConfig{}}
		_, err := m.GetIntegration("x")
		require.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
	})
	t.Run("missing name", func(t *testing.T) {
		m := &manager{config: &schema.AuthConfig{Integrations: map[string]schema.Integration{}}}
		_, err := m.GetIntegration("x")
		require.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
	})
	t.Run("found", func(t *testing.T) {
		m := &manager{config: &schema.AuthConfig{Integrations: map[string]schema.Integration{
			"gh": {Kind: integrations.KindGitHubSTS},
		}}}
		got, err := m.GetIntegration("gh")
		require.NoError(t, err)
		assert.Equal(t, integrations.KindGitHubSTS, got.Kind)
	})
}

func TestExecuteIntegration_EarlyErrors(t *testing.T) {
	t.Run("nil integrations", func(t *testing.T) {
		m := &manager{config: &schema.AuthConfig{}}
		err := m.ExecuteIntegration(context.Background(), "x")
		require.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
	})
	t.Run("missing name", func(t *testing.T) {
		m := &manager{config: &schema.AuthConfig{Integrations: map[string]schema.Integration{}}}
		err := m.ExecuteIntegration(context.Background(), "x")
		require.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
	})
	t.Run("no via.identity", func(t *testing.T) {
		m := &manager{config: &schema.AuthConfig{Integrations: map[string]schema.Integration{
			// Provider-bound (no via.identity) — ExecuteIntegration requires an identity.
			"gh": {Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}},
		}}}
		err := m.ExecuteIntegration(context.Background(), "gh")
		require.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	})
}

func TestExecuteIdentityIntegrations_Errors(t *testing.T) {
	t.Run("unknown identity", func(t *testing.T) {
		m := &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{}}}
		err := m.ExecuteIdentityIntegrations(context.Background(), "nope")
		require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	})
	t.Run("no linked integrations", func(t *testing.T) {
		m := &manager{config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{"id": {Kind: "atmos/pro"}},
		}}
		err := m.ExecuteIdentityIntegrations(context.Background(), "id")
		require.ErrorIs(t, err, errUtils.ErrNoLinkedIntegrations)
	})
}

func TestFindIntegrationsForIdentity_NilConfig(t *testing.T) {
	m := &manager{config: &schema.AuthConfig{}}
	assert.Nil(t, m.findIntegrationsForIdentity("x", false))
}
