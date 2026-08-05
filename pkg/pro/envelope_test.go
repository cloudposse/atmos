package pro

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stsPayload mirrors the shape of the github/sts response payload (tokens nested under
// data). It lives in the test so the decoder stays decoupled from any one caller.
type stsPayload struct {
	Tokens []struct {
		Owner string `json:"owner"`
		Token string `json:"token"`
	} `json:"tokens"`
}

// TestDecodeEnvelope_UnwrapsData verifies the canonical envelope is unwrapped so the
// typed payload is read from `data`, not the top level.
func TestDecodeEnvelope_UnwrapsData(t *testing.T) {
	const body = `{"success":true,"status":200,"data":{"tokens":[{"owner":"o","token":"t"}]}}`

	env, err := DecodeEnvelope[stsPayload](strings.NewReader(body))
	require.NoError(t, err)
	assert.True(t, env.Success)
	require.Len(t, env.Data.Tokens, 1)
	assert.Equal(t, "o", env.Data.Tokens[0].Owner)
	assert.Equal(t, "t", env.Data.Tokens[0].Token)
}

// TestDecodeEnvelope_FlatPayloadDropsData is the canary for the exact bug class: a flat,
// top-level `{tokens:...}` body (the shape the server NEVER sends) decodes to an EMPTY
// Data. This documents why routing every Pro response through the envelope is mandatory —
// a flat decode silently drops a data-nested result and surfaces no error.
func TestDecodeEnvelope_FlatPayloadDropsData(t *testing.T) {
	const flat = `{"tokens":[{"owner":"o","token":"t"}]}`

	env, err := DecodeEnvelope[stsPayload](strings.NewReader(flat))
	require.NoError(t, err)
	assert.Empty(t, env.Data.Tokens, "top-level tokens must NOT populate data — that is the bug the envelope prevents")
}

// TestDecodeEnvelope_SurfacesError verifies success=false responses expose the
// server-side message via EffectiveErrorMessage, tolerating both the current
// `errorMessage` field and the legacy `error` field.
func TestDecodeEnvelope_SurfacesError(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"errorMessage field", `{"success":false,"status":403,"errorMessage":"not entitled"}`, "not entitled"},
		{"legacy error field", `{"success":false,"status":403,"error":"legacy boom"}`, "legacy boom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, err := DecodeEnvelope[stsPayload](strings.NewReader(tc.body))
			require.NoError(t, err)
			assert.False(t, env.Success)
			assert.Equal(t, tc.want, env.EffectiveErrorMessage())
		})
	}
}

// TestDecodeEnvelope_GenericOverPayloadType verifies the decoder is payload-agnostic by
// instantiating it with a different typed payload.
func TestDecodeEnvelope_GenericOverPayloadType(t *testing.T) {
	type tokenData struct {
		Token string `json:"token"`
	}
	const body = `{"success":true,"status":200,"data":{"token":"abc123"}}`

	env, err := DecodeEnvelope[tokenData](strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, "abc123", env.Data.Token)
}

// TestDecodeEnvelope_InvalidJSON verifies a malformed body is returned as an error.
func TestDecodeEnvelope_InvalidJSON(t *testing.T) {
	_, err := DecodeEnvelope[stsPayload](strings.NewReader("not-json"))
	require.Error(t, err)
}
