package azure

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func makeJWTWithClaims(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(body)
	return header + "." + payload + ".sig"
}

func TestExtractObjectIDFromToken(t *testing.T) {
	oid := "11111111-2222-3333-4444-555555555555"
	token := makeJWTWithClaims(map[string]any{"oid": oid, "upn": "user@example.com"})

	got, err := ExtractObjectIDFromToken(token)
	require.NoError(t, err)
	assert.Equal(t, oid, got)
}

func TestExtractObjectIDFromToken_Errors(t *testing.T) {
	t.Run("not a jwt", func(t *testing.T) {
		_, err := ExtractObjectIDFromToken("not-a-jwt")
		require.ErrorIs(t, err, errUtils.ErrAzureInvalidJWTFormat)
	})

	t.Run("missing oid claim", func(t *testing.T) {
		token := makeJWTWithClaims(map[string]any{"upn": "user@example.com"})
		_, err := ExtractObjectIDFromToken(token)
		require.ErrorIs(t, err, errUtils.ErrAzureOIDClaimNotFound)
	})
}
