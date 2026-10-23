package types

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestOCICredentialsMetadataAndExpiration(t *testing.T) {
	credentials := &OCICredentials{Registry: "ghcr.io", Username: "company-bot", Password: "private-password", IdentityToken: "private-identity-token", RegistryToken: "private-registry-token"}
	require.Equal(t, "ghcr.io", credentials.RegistryHost())
	require.NotContains(t, fmt.Sprint(credentials), "private-")
	require.Contains(t, fmt.Sprint(credentials), "ghcr.io")
	var absent *OCICredentials
	require.Equal(t, "OCICredentials(nil)", absent.String())
	username, password, identityToken, registryToken := credentials.RegistryAuth()
	require.Equal(t, "company-bot", username)
	require.Equal(t, "private-password", password)
	require.Equal(t, "private-identity-token", identityToken)
	require.Equal(t, "private-registry-token", registryToken)
	require.False(t, credentials.IsExpired())
	expiration, err := credentials.GetExpiration()
	require.NoError(t, err)
	require.Nil(t, expiration)
	past := time.Now().Add(-time.Hour)
	credentials.Expiration = &past
	require.True(t, credentials.IsExpired())
	future := time.Now().Add(time.Hour)
	credentials.Expiration = &future
	require.False(t, credentials.IsExpired())
	require.True(t, (&OCICredentials{}).IsExpired())
	credentials.BuildWhoamiInfo(nil)
	info := &WhoamiInfo{Credentials: credentials}
	credentials.BuildWhoamiInfo(info)
	require.Equal(t, "company-bot", info.Principal)
	require.Equal(t, "ghcr.io", info.Account)
	for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
		raw, err := marshal(info)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "private-")
	}
	_, err = credentials.Validate(t.Context())
	require.ErrorIs(t, err, errUtils.ErrNotImplemented)
}

func TestPluginCredentialsExpirationAndRedactedMetadata(t *testing.T) {
	for name, expiration := range map[string]time.Time{"absent": {}, "expired": time.Now().Add(-time.Hour), "valid": time.Now().Add(time.Hour)} {
		t.Run(name, func(t *testing.T) {
			credentials := &PluginCredentials{Type: "custom/v1", ExpiresAt: expiration, Data: map[string]any{"subject": "example-user", "refresh_token": "private-token"}}
			require.Equal(t, name != "valid", credentials.IsExpired())
			actual, err := credentials.GetExpiration()
			if expiration.IsZero() {
				require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
				require.Nil(t, actual)
			} else {
				require.NoError(t, err)
				require.Equal(t, expiration, *actual)
				*actual = time.Time{}
				require.Equal(t, expiration, credentials.ExpiresAt, "expiration values must not alias stored credentials")
			}
			credentials.BuildWhoamiInfo(nil)
			info := &WhoamiInfo{Credentials: credentials}
			credentials.BuildWhoamiInfo(info)
			require.Equal(t, "example-user", info.Principal)
			for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
				raw, err := marshal(info)
				require.NoError(t, err)
				require.NotContains(t, string(raw), "private-token")
			}
			require.NotContains(t, fmt.Sprint(credentials), "private-token")
			_, err = credentials.Validate(t.Context())
			require.ErrorIs(t, err, errUtils.ErrNotImplemented)
		})
	}
}
