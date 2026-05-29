package aws

// Reproduction + unit tests for the RFC 9449 DPoP proof attached to AWS
// signin /v1/token requests (issue #2542).

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustGenerateDPoPKey returns a fresh EC P-256 key for tests, failing the test
// on error.
func mustGenerateDPoPKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := generateDPoPKey()
	require.NoError(t, err)
	return key
}

// testEncodedDPoPKey returns a freshly generated DPoP key serialized for the
// refresh cache. Refresh-flow fixtures need a parseable key so the exchange is
// reached (issue #2542).
func testEncodedDPoPKey(t *testing.T) string {
	t.Helper()
	encoded, err := marshalDPoPKey(mustGenerateDPoPKey(t))
	require.NoError(t, err)
	return encoded
}

// tokenSuccessResponse returns an *http.Response carrying a minimal valid
// /v1/token success body so the exchange helpers parse credentials cleanly.
func tokenSuccessResponse() *http.Response {
	body, _ := json.Marshal(map[string]interface{}{
		"accessToken": map[string]string{
			"accessKeyId":     "AKIAIOSFODNN7EXAMPLE",
			"secretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"sessionToken":    "session-token",
		},
		"expiresIn":    900,
		"refreshToken": "refresh-token-value",
	})
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

// TestExchangeCodeForCredentials_SendsDPoPHeader reproduces issue #2542: the
// authorization-code exchange against AWS signin must carry an RFC 9449 DPoP
// proof header. On unpatched code no DPoP header is set and AWS rejects the
// request with HTTP 400, so this test fails until the proof is attached.
func TestExchangeCodeForCredentials_SendsDPoPHeader(t *testing.T) {
	var captured string
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			captured = req.Header.Get("DPoP")
			return tokenSuccessResponse(), nil
		},
	}

	_, err := exchangeCodeForCredentials(context.Background(), mockClient, exchangeCodeParams{
		region: "us-east-2", code: "auth-code", codeVerifier: "verifier", redirectURI: "http://127.0.0.1:8080/oauth/callback",
		dpopKey: mustGenerateDPoPKey(t),
	})
	require.NoError(t, err)
	require.NotEmpty(t, captured, "authorization-code token exchange must send a DPoP proof header")

	htm, htu := proofMethodAndURL(t, captured)
	require.Equal(t, http.MethodPost, htm)
	require.Equal(t, "https://us-east-2.signin.aws.amazon.com/v1/token", htu)
}

// TestExchangeRefreshToken_SendsDPoPHeader reproduces issue #2542 for the
// refresh-token grant. The refresh exchange also hits /v1/token and must carry
// a DPoP proof signed with the key the refresh token is bound to.
func TestExchangeRefreshToken_SendsDPoPHeader(t *testing.T) {
	var captured string
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			captured = req.Header.Get("DPoP")
			return tokenSuccessResponse(), nil
		},
	}

	_, err := exchangeRefreshToken(context.Background(), mockClient, "us-east-2", "my-refresh-token", mustGenerateDPoPKey(t))
	require.NoError(t, err)
	require.NotEmpty(t, captured, "refresh-token exchange must send a DPoP proof header")
}

// proofMethodAndURL decodes a DPoP proof JWT and returns its htm and htu claims.
func proofMethodAndURL(t *testing.T, proof string) (htm, htu string) {
	t.Helper()
	parts := strings.Split(proof, ".")
	require.Len(t, parts, 3, "DPoP proof must be a 3-segment JWS")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims dpopClaims
	require.NoError(t, json.Unmarshal(payload, &claims))
	return claims.HTM, claims.HTU
}

// TestNewDPoPProof_StructureAndSignature verifies the proof is a well-formed
// RFC 9449 dpop+jwt: correct header, claims, and a signature that verifies
// against the embedded JWK public key.
func TestNewDPoPProof_StructureAndSignature(t *testing.T) {
	key := mustGenerateDPoPKey(t)
	const htu = "https://us-east-2.signin.aws.amazon.com/v1/token"

	proof, err := newDPoPProof(key, http.MethodPost, htu)
	require.NoError(t, err)

	parts := strings.Split(proof, ".")
	require.Len(t, parts, 3, "proof must be a 3-segment compact JWS")

	// Header: typ, alg, and a P-256 EC JWK.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var header dpopHeader
	require.NoError(t, json.Unmarshal(headerJSON, &header))
	assert.Equal(t, "dpop+jwt", header.Typ)
	assert.Equal(t, "ES256", header.Alg)
	assert.Equal(t, "EC", header.JWK.Kty)
	assert.Equal(t, "P-256", header.JWK.Crv)
	assert.NotEmpty(t, header.JWK.X)
	assert.NotEmpty(t, header.JWK.Y)

	// Claims: htm, htu, non-empty jti, recent iat.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims dpopClaims
	require.NoError(t, json.Unmarshal(claimsJSON, &claims))
	assert.Equal(t, http.MethodPost, claims.HTM)
	assert.Equal(t, htu, claims.HTU)
	assert.NotEmpty(t, claims.JTI)
	assert.Positive(t, claims.IAT)

	// The embedded JWK coordinates must match the signing key (derived via
	// crypto/ecdh to avoid the deprecated big.Int X/Y fields).
	ecdhPub, err := key.PublicKey.ECDH()
	require.NoError(t, err)
	point := ecdhPub.Bytes() // 0x04 || X(32) || Y(32).
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(point[1:1+dpopCoordinateBytes]), header.JWK.X)
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(point[1+dpopCoordinateBytes:]), header.JWK.Y)

	// Signature must verify against the signing key's public half.
	sig := mustDecodeRawURL(t, parts[2])
	require.Len(t, sig, 2*dpopCoordinateBytes, "ES256 signature must be R‖S (64 bytes)")
	r := new(big.Int).SetBytes(sig[:dpopCoordinateBytes])
	s := new(big.Int).SetBytes(sig[dpopCoordinateBytes:])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	assert.True(t, ecdsa.Verify(&key.PublicKey, digest[:], r, s),
		"proof signature must verify against the embedded public key")
}

// TestNewDPoPProof_NilKey returns an error rather than panicking.
func TestNewDPoPProof_NilKey(t *testing.T) {
	_, err := newDPoPProof(nil, http.MethodPost, "https://example.com/v1/token")
	require.Error(t, err)
}

// TestMarshalParseDPoPKey_RoundTrip verifies a DPoP key survives serialization
// to the cache and back unchanged.
func TestMarshalParseDPoPKey_RoundTrip(t *testing.T) {
	key := mustGenerateDPoPKey(t)

	encoded, err := marshalDPoPKey(key)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := parseDPoPKey(encoded)
	require.NoError(t, err)
	assert.True(t, key.Equal(decoded), "decoded key must equal the original")
}

// TestParseDPoPKey_Invalid rejects malformed input.
func TestParseDPoPKey_Invalid(t *testing.T) {
	_, err := parseDPoPKey("not-base64!!")
	require.Error(t, err)

	_, err = parseDPoPKey(base64.StdEncoding.EncodeToString([]byte("not-a-key")))
	require.Error(t, err)
}

// mustDecodeRawURL base64url-decodes s, failing the test on error.
func mustDecodeRawURL(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	require.NoError(t, err)
	return b
}
