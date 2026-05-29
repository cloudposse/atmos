package types

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeJWT builds a minimal unsigned JWT with the given exp claim (0 = omit).
func makeJWT(t *testing.T, exp int64) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	claims := map[string]any{}
	if exp != 0 {
		claims["exp"] = exp
	}
	payloadBytes, err := json.Marshal(claims)
	require.NoError(t, err)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return fmt.Sprintf("%s.%s.sig", header, payload)
}

func TestProCredentials_Expiration(t *testing.T) {
	future := time.Now().Add(time.Hour).Unix()
	c := &ProCredentials{Token: makeJWT(t, future)}

	exp, err := c.GetExpiration()
	require.NoError(t, err)
	require.NotNil(t, exp)
	assert.WithinDuration(t, time.Unix(future, 0), *exp, time.Second)
	assert.False(t, c.IsExpired())
}

func TestProCredentials_Expired(t *testing.T) {
	past := time.Now().Add(-time.Hour).Unix()
	c := &ProCredentials{Token: makeJWT(t, past)}
	assert.True(t, c.IsExpired())
}

func TestProCredentials_NoExpiration(t *testing.T) {
	// A token without an exp claim is treated as non-expiring.
	c := &ProCredentials{Token: makeJWT(t, 0)}
	exp, err := c.GetExpiration()
	require.NoError(t, err)
	assert.Nil(t, exp)
	assert.False(t, c.IsExpired())

	// A non-JWT token also yields no expiration (not an error).
	c2 := &ProCredentials{Token: "not-a-jwt"}
	exp2, err := c2.GetExpiration()
	require.NoError(t, err)
	assert.Nil(t, exp2)
}

func TestProCredentials_BuildWhoamiInfo(t *testing.T) {
	c := &ProCredentials{Token: makeJWT(t, time.Now().Add(time.Hour).Unix()), WorkspaceID: "ws-99"}
	info := &WhoamiInfo{}
	c.BuildWhoamiInfo(info)
	assert.Equal(t, "ws-99", info.Account)
	assert.NotNil(t, info.Expiration)
}
