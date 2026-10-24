package aws

// RFC 9449 DPoP (Demonstrating Proof-of-Possession) support for the AWS signin
// OAuth2 flow. AWS signin's /v1/token endpoint requires a DPoP proof JWT on
// every token request (authorization-code and refresh-token grants); without
// it the endpoint returns HTTP 400 INVALID_REQUEST (issue #2542).
//
// The proof is a compact JWS with header {typ: dpop+jwt, alg: ES256, jwk: <EC
// public key>} and claims {jti, htm, htu, iat}, signed with an ephemeral EC
// P-256 key. Because the AWS OAuth client is public, the refresh token is bound
// to the DPoP key, so the same key must be reused on refresh — callers persist
// it alongside the refresh token (see webflow_cache.go).

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// The dpopJTIBytes constant is the byte length of the random DPoP proof identifier (jti).
	dpopJTIBytes = 16
	// The dpopCoordinateBytes constant is the fixed byte length of a P-256 affine coordinate.
	dpopCoordinateBytes = 32
	// The dpopUncompressedPointLen constant is the length of an uncompressed P-256 point: 0x04 || X(32) || Y(32).
	dpopUncompressedPointLen = 1 + 2*dpopCoordinateBytes
)

// dpopHeader is the JOSE header of a DPoP proof JWT.
type dpopHeader struct {
	Typ string  `json:"typ"`
	Alg string  `json:"alg"`
	JWK dpopJWK `json:"jwk"`
}

// dpopJWK is the public EC key embedded in the DPoP proof header (RFC 7517).
type dpopJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// dpopClaims are the registered claims of a DPoP proof JWT (RFC 9449 §4.2).
type dpopClaims struct {
	JTI string `json:"jti"`
	HTM string `json:"htm"`
	HTU string `json:"htu"`
	IAT int64  `json:"iat"`
}

// generateDPoPKey creates an ephemeral EC P-256 private key for DPoP proofs.
func generateDPoPKey() (*ecdsa.PrivateKey, error) {
	defer perf.Track(nil, "aws.generateDPoPKey")()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate key: %w", errUtils.ErrWebflowDPoP, err)
	}
	return key, nil
}

// newDPoPProof builds and signs an RFC 9449 DPoP proof JWT for the given HTTP
// method (htm) and target URL (htu). The htu MUST be the token endpoint URL
// without query or fragment components.
func newDPoPProof(key *ecdsa.PrivateKey, htm, htu string) (string, error) {
	defer perf.Track(nil, "aws.newDPoPProof")()

	if key == nil {
		return "", fmt.Errorf("%w: nil DPoP key", errUtils.ErrWebflowDPoP)
	}

	jti, err := randomBase64URL(dpopJTIBytes)
	if err != nil {
		return "", fmt.Errorf("%w: failed to generate jti: %w", errUtils.ErrWebflowDPoP, err)
	}

	jwk, err := publicKeyToJWK(&key.PublicKey)
	if err != nil {
		return "", err
	}
	header := dpopHeader{
		Typ: "dpop+jwt",
		Alg: "ES256",
		JWK: jwk,
	}
	claims := dpopClaims{
		JTI: jti,
		HTM: htm,
		HTU: htu,
		IAT: time.Now().Unix(),
	}

	headerSegment, err := marshalSegment(header)
	if err != nil {
		return "", fmt.Errorf("%w: failed to encode header: %w", errUtils.ErrWebflowDPoP, err)
	}
	claimsSegment, err := marshalSegment(claims)
	if err != nil {
		return "", fmt.Errorf("%w: failed to encode claims: %w", errUtils.ErrWebflowDPoP, err)
	}

	signingInput := headerSegment + "." + claimsSegment
	signature, err := signES256(key, signingInput)
	if err != nil {
		return "", err
	}

	return signingInput + "." + signature, nil
}

// publicKeyToJWK converts an EC P-256 public key to its JWK representation with
// fixed-width, base64url-encoded affine coordinates. It derives the coordinates
// from the uncompressed point encoding via crypto/ecdh rather than reading the
// deprecated big.Int X/Y fields directly (Go 1.26).
func publicKeyToJWK(pub *ecdsa.PublicKey) (dpopJWK, error) {
	ecdhPub, err := pub.ECDH()
	if err != nil {
		return dpopJWK{}, fmt.Errorf("%w: failed to convert public key: %w", errUtils.ErrWebflowDPoP, err)
	}
	// Uncompressed point: 0x04 || X(32) || Y(32).
	point := ecdhPub.Bytes()
	if len(point) != dpopUncompressedPointLen {
		return dpopJWK{}, fmt.Errorf("%w: unexpected public key length %d", errUtils.ErrWebflowDPoP, len(point))
	}
	return dpopJWK{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(point[1 : 1+dpopCoordinateBytes]),
		Y:   base64.RawURLEncoding.EncodeToString(point[1+dpopCoordinateBytes:]),
	}, nil
}

// signES256 computes the base64url-encoded ES256 (ECDSA P-256 + SHA-256)
// signature over signingInput, using the fixed-width R‖S encoding required by
// JWS (RFC 7518 §3.4).
func signES256(key *ecdsa.PrivateKey, signingInput string) (string, error) {
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("%w: failed to sign proof: %w", errUtils.ErrWebflowDPoP, err)
	}

	sig := make([]byte, 2*dpopCoordinateBytes)
	copy(sig[:dpopCoordinateBytes], leftPadCoordinate(r.Bytes()))
	copy(sig[dpopCoordinateBytes:], leftPadCoordinate(s.Bytes()))
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// marshalSegment JSON-encodes v and base64url-encodes it without padding.
func marshalSegment(v interface{}) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// randomBase64URL returns n random bytes encoded as base64url without padding.
func randomBase64URL(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// leftPadCoordinate returns b left-padded with zero bytes to dpopCoordinateBytes.
// If b is already that long, it is returned unchanged.
func leftPadCoordinate(b []byte) []byte {
	if len(b) >= dpopCoordinateBytes {
		return b
	}
	padded := make([]byte, dpopCoordinateBytes)
	copy(padded[dpopCoordinateBytes-len(b):], b)
	return padded
}

// marshalDPoPKey serializes a DPoP private key to a base64-encoded PKCS#8 DER
// string suitable for persisting in the refresh-token cache.
func marshalDPoPKey(key *ecdsa.PrivateKey) (string, error) {
	defer perf.Track(nil, "aws.marshalDPoPKey")()

	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal key: %w", errUtils.ErrWebflowDPoP, err)
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// parseDPoPKey reverses marshalDPoPKey, returning the EC private key encoded in
// the given base64 PKCS#8 DER string.
func parseDPoPKey(encoded string) (*ecdsa.PrivateKey, error) {
	defer perf.Track(nil, "aws.parseDPoPKey")()

	der, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode key: %w", errUtils.ErrWebflowDPoP, err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse key: %w", errUtils.ErrWebflowDPoP, err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: unexpected key type %T", errUtils.ErrWebflowDPoP, parsed)
	}
	return key, nil
}
