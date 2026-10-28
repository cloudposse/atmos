package oci

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func fixture(t *testing.T) *Identity {
	t.Helper()
	value, err := NewIdentity("registry-bot", &schema.Identity{Kind: types.IdentityKindOCIUser, Spec: map[string]any{"registry": "GHCR.IO", "username": "company-bot", "password_env": "TEST_OCI_PASSWORD"}})
	require.NoError(t, err)
	return value.(*Identity)
}

func TestEnvironmentAuthenticationHasExplicitSource(t *testing.T) {
	identity := fixture(t)
	t.Setenv("TEST_OCI_PASSWORD", "private-token")
	t.Setenv("GITHUB_TOKEN", "unrelated-token")
	identity.prompt = func(types.CredentialPromptSpec) (map[string]string, error) {
		t.Fatal("configured environment source must not prompt")
		return nil, nil
	}
	value, err := identity.Authenticate(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, &types.OCICredentials{Registry: "ghcr.io", Username: "company-bot", Password: "private-token"}, value)
	t.Setenv("TEST_OCI_PASSWORD", "")
	_, err = identity.AuthenticateStandalone(t.Context())
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	require.NotContains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), "unrelated-token")
	require.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), "TEST_OCI_PASSWORD")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = identity.AuthenticateStandalone(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPromptMasksSecretAndKeepsConfiguredHostAndUser(t *testing.T) {
	identity := fixture(t)
	identity.passwordEnv = ""
	identity.prompt = func(spec types.CredentialPromptSpec) (map[string]string, error) {
		require.Equal(t, "registry-bot", spec.IdentityName)
		require.Len(t, spec.Fields, 3)
		require.Equal(t, "ghcr.io", spec.Fields[0].Default)
		require.Equal(t, "company-bot", spec.Fields[1].Default)
		require.Equal(t, "password", spec.Fields[2].Name)
		require.True(t, spec.Fields[2].Secret)
		require.True(t, spec.Fields[2].Required)
		return map[string]string{"password": "entered-secret", "registry": "evil.example", "username": "other"}, nil
	}
	value, err := identity.AuthenticateStandalone(t.Context())
	require.NoError(t, err)
	require.Equal(t, &types.OCICredentials{Registry: "ghcr.io", Username: "company-bot", Password: "entered-secret"}, value)
	identity.prompt = func(types.CredentialPromptSpec) (map[string]string, error) {
		return nil, errUtils.ErrAuthPromptUnavailable
	}
	_, err = identity.AuthenticateStandalone(t.Context())
	require.ErrorIs(t, err, errUtils.ErrAuthPromptUnavailable)
	require.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), "spec.password_env")
	identity.prompt = func(types.CredentialPromptSpec) (map[string]string, error) { return map[string]string{}, nil }
	_, err = identity.AuthenticateStandalone(t.Context())
	require.ErrorIs(t, err, errUtils.ErrMissingInput)
}

func TestIdentityRejectsAmbiguousOrUnsafeConfig(t *testing.T) {
	cases := []*schema.Identity{
		nil,
		{Kind: "other"},
		{Kind: types.IdentityKindOCIUser, Via: &schema.IdentityVia{Provider: "other"}},
		{Kind: types.IdentityKindOCIUser},
		{Kind: types.IdentityKindOCIUser, Spec: map[string]any{"registry": "ghcr.io", "username": "bot", "password": "inline-secret"}},
		{Kind: types.IdentityKindOCIUser, Spec: map[string]any{"registry": 42}},
		{Kind: types.IdentityKindOCIUser, Spec: map[string]any{"registry": "ghcr.io", "username": "bot", "password_env": "INVALID NAME"}},
	}
	for _, config := range cases {
		_, err := NewIdentity("registry", config)
		require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
		require.NotContains(t, err.Error(), "inline-secret")
	}
	for _, registry := range []string{"https://ghcr.io", "ghcr.io/repository", "user@ghcr.io", "ghcr.io?query", "ghcr.io\n", "ghcr.io\t", "registry:invalid-port", "[broken"} {
		_, err := NewIdentity("registry", &schema.Identity{Kind: types.IdentityKindOCIUser, Spec: map[string]any{"registry": registry, "username": "bot"}})
		require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig, "%q", registry)
	}
	_, err := NewIdentity("local", &schema.Identity{Kind: types.IdentityKindOCIUser, Spec: map[string]any{"registry": "localhost:15555", "username": "bot"}})
	require.NoError(t, err)
}

func TestIdentityDoesNotExposeSecretsInEnvironmentOrFiles(t *testing.T) {
	identity := fixture(t)
	require.Equal(t, types.IdentityKindOCIUser, identity.Kind())
	require.True(t, identity.IsStandalone())
	root, err := identity.GetProviderName()
	require.NoError(t, err)
	require.Equal(t, "registry-bot", root)
	environment, err := identity.Environment()
	require.NoError(t, err)
	require.Empty(t, environment)
	paths, err := identity.Paths()
	require.NoError(t, err)
	require.Empty(t, paths)
	original := map[string]string{"EXISTING": "value"}
	result, err := identity.PrepareEnvironment(t.Context(), original)
	require.NoError(t, err)
	result["EXISTING"] = "changed"
	require.Equal(t, "value", original["EXISTING"])
	original["EXISTING"] = "source-change"
	require.Equal(t, "changed", result["EXISTING"])
	require.NoError(t, identity.PostAuthenticate(t.Context(), nil))
	exists, err := identity.CredentialsExist()
	require.NoError(t, err)
	require.False(t, exists)
	value, err := identity.LoadCredentials(t.Context())
	require.NoError(t, err)
	require.Nil(t, value)
}

func TestLogoutRemovesOnlyIdentityInCurrentRealm(t *testing.T) {
	identity := fixture(t)
	require.NoError(t, identity.Logout(t.Context()))
	store := credentials.NewCredentialStoreWithConfig(&schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "memory"}})
	identity.SetCredentialStore(store)
	identity.SetRealm("project-one")
	secret := &types.OCICredentials{Registry: "ghcr.io", Username: "bot", Password: "secret"}
	for _, realm := range []string{"project-one", "project-two"} {
		require.NoError(t, store.Store("registry-bot", secret, realm))
		require.NoError(t, store.Store("other-bot", secret, realm))
	}
	require.NoError(t, identity.Logout(t.Context()))
	require.NoError(t, identity.Logout(t.Context()), "logout is idempotent")
	_, err := store.Retrieve("registry-bot", "project-one")
	require.ErrorIs(t, err, credentials.ErrCredentialsNotFound)
	other, err := store.Retrieve("other-bot", "project-one")
	require.NoError(t, err)
	require.Equal(t, secret, other)
	other, err = store.Retrieve("registry-bot", "project-two")
	require.NoError(t, err)
	require.Equal(t, secret, other)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, identity.Logout(ctx), context.Canceled)
}
