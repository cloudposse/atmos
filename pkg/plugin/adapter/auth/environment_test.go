package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEnvironmentMaterializerLifecycleAndIsolation(t *testing.T) {
	m := EnvironmentMaterializer("identity", nil)
	_, err := m.Environment()
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	_, err = m.Load(t.Context())
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	_, err = m.PrepareEnvironment(nil)
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	credentials := &types.PluginCredentials{Type: "environment/v1", ExpiresAt: time.Now().Add(time.Hour), Data: map[string]any{"TOKEN": "private-token"}, SensitiveKeys: []string{"TOKEN"}}
	authContext := &schema.AuthContext{}
	require.NoError(t, m.Apply(t.Context(), &types.PostAuthenticateParams{AuthContext: authContext, Credentials: credentials}))
	require.Equal(t, map[string]string{"TOKEN": "private-token"}, authContext.Environment)
	credentials.Data["TOKEN"] = "mutated"
	credentials.SensitiveKeys[0] = "changed"
	require.Equal(t, "private-token", authContext.Environment["TOKEN"], "auth context must not alias source credentials")
	values, err := m.Environment()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "private-token"}, values)
	authContext.Environment["TOKEN"] = "context mutation"
	values, err = m.Environment()
	require.NoError(t, err)
	require.Equal(t, "private-token", values["TOKEN"], "materializer must not alias auth context")
	values["TOKEN"] = "output mutation"
	loaded, err := m.Load(t.Context())
	require.NoError(t, err)
	copy := loaded.(*types.PluginCredentials)
	require.Equal(t, "private-token", copy.Data["TOKEN"])
	require.Equal(t, []string{"TOKEN"}, copy.SensitiveKeys)
	copy.Data["TOKEN"] = "load mutation"
	copy.SensitiveKeys[0] = "load mutation"
	base := map[string]string{"token": "old", "PATH": "unchanged"}
	merged, err := m.PrepareEnvironment(base)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "private-token", "PATH": "unchanged"}, merged)
	require.Equal(t, map[string]string{"token": "old", "PATH": "unchanged"}, base)
	merged["PATH"] = "output mutation"
	require.Equal(t, "unchanged", base["PATH"])
	base["PATH"] = "source mutation"
	require.Equal(t, "output mutation", merged["PATH"])
	merged, err = m.PrepareEnvironment(nil)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "private-token"}, merged)
	require.NoError(t, m.Logout(t.Context()))
	_, err = m.Environment()
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	_, err = m.Load(t.Context())
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestEnvironmentMaterializerRejectsInvalidCredentials(t *testing.T) {
	for _, tt := range []struct {
		name   string
		params *types.PostAuthenticateParams
	}{
		{"nil params", nil},
		{"missing credentials", &types.PostAuthenticateParams{}},
		{"wrong type", &types.PostAuthenticateParams{Credentials: &types.PluginCredentials{Type: "secret-values/v1", ExpiresAt: time.Now().Add(time.Hour)}}},
		{"expired", &types.PostAuthenticateParams{Credentials: &types.PluginCredentials{Type: "environment/v1", ExpiresAt: time.Now().Add(-time.Hour)}}},
		{"non string", &types.PostAuthenticateParams{Credentials: &types.PluginCredentials{Type: "environment/v1", ExpiresAt: time.Now().Add(time.Hour), Data: map[string]any{"TOKEN": 1}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := EnvironmentMaterializer("identity", nil)
			require.ErrorIs(t, m.Apply(t.Context(), tt.params), errUtils.ErrPluginProtocol)
			_, err := m.Environment()
			require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, EnvironmentMaterializer("identity", nil).Apply(ctx, nil), context.Canceled)
}

func TestEnvironmentMaterializerMasksComposedCredentials(t *testing.T) {
	m := EnvironmentMaterializer("identity", nil)
	credentials := &types.PluginCredentials{
		Type:      "environment/v1",
		ExpiresAt: time.Now().Add(time.Hour),
		Data: map[string]any{
			"ACCESS":        "private-access",
			"SECRET":        "private-secret",
			"AUTHORIZATION": "Bearer private-access:private-secret",
		},
		SensitiveKeys: []string{"ACCESS", "SECRET", "AUTHORIZATION"},
	}
	require.NoError(t, m.Apply(t.Context(), &types.PostAuthenticateParams{Credentials: credentials}))
	require.Equal(t, "authorization="+atmosio.MaskReplacement,
		atmosio.MaskString("authorization=Bearer private-access:private-secret"),
		"the whole composed credential must be masked, including its non-secret prefix")
	require.Equal(t, "access="+atmosio.MaskReplacement+" secret="+atmosio.MaskReplacement,
		atmosio.MaskString("access=private-access secret=private-secret"))
}
