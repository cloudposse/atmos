// Package oidc reads GitHub Actions OIDC token claims (repository, environment, ref) for runtime
// context detection. It is a stdlib-only leaf package (no Atmos imports) so low-level packages
// like pkg/store can use it without creating an import cycle through pkg/schema.
package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// requestTimeout bounds the OIDC token request to avoid hanging a runner with a stuck network.
const requestTimeout = 10 * time.Second

var (
	// ErrInvalidRequestURL indicates ACTIONS_ID_TOKEN_REQUEST_URL is malformed or unsafe.
	ErrInvalidRequestURL = errors.New("invalid ACTIONS_ID_TOKEN_REQUEST_URL")
	// ErrTokenRequest indicates the OIDC token endpoint request failed.
	ErrTokenRequest = errors.New("failed to request GitHub OIDC token")
	// ErrTokenDecode indicates the OIDC JWT could not be decoded into claims.
	ErrTokenDecode = errors.New("failed to decode GitHub OIDC token")
)

// Claims holds the subset of GitHub Actions OIDC token claims used for runtime context checks.
type Claims struct {
	Repository  string `json:"repository"`
	Environment string `json:"environment"`
	Ref         string `json:"ref"`
	Subject     string `json:"sub"`
}

// RequestClaims mints (or reads) the GitHub Actions OIDC token and returns its claims.
//
// available is false (with a nil error) when the token is unobtainable because the process is not
// a GitHub Actions runner, or the job lacks `id-token: write` permission — callers treat that as
// "unknown context", not a failure. A non-nil error means the token was obtainable but the
// request or decoding failed.
func RequestClaims(ctx context.Context) (claims *Claims, available bool, err error) {
	jwt, available, err := requestToken(ctx)
	if err != nil || !available {
		return nil, available, err
	}
	c, err := decodeClaims(jwt)
	if err != nil {
		return nil, true, err
	}
	return c, true, nil
}

// requestToken returns the OIDC JWT. It prefers a pre-injected ACTIONS_ID_TOKEN, otherwise mints
// one from the request token/URL. available reflects whether a token could be obtained at all.
func requestToken(ctx context.Context) (jwt string, available bool, err error) {
	if getenv("GITHUB_ACTIONS") != "true" {
		return "", false, nil
	}
	if t := strings.TrimSpace(getenv("ACTIONS_ID_TOKEN")); t != "" {
		return t, true, nil
	}

	requestTokenValue := strings.TrimSpace(getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"))
	requestURL := strings.TrimSpace(getenv("ACTIONS_ID_TOKEN_REQUEST_URL"))
	if requestTokenValue == "" || requestURL == "" {
		// The job did not grant `id-token: write`, so no token can be minted.
		return "", false, nil
	}
	if err := validateRequestURL(requestURL); err != nil {
		return "", true, err
	}

	jwt, err = fetchToken(ctx, requestURL, requestTokenValue)
	if err != nil {
		return "", true, err
	}
	return jwt, true, nil
}

// validateRequestURL guards against SSRF: the URL must be HTTPS with a non-empty host.
func validateRequestURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRequestURL, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: scheme must be https, got %q", ErrInvalidRequestURL, u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("%w: empty host", ErrInvalidRequestURL)
	}
	return nil
}

// fetchToken requests the OIDC JWT from the GitHub Actions token endpoint.
func fetchToken(ctx context.Context, requestURL, requestToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrTokenRequest, err)
	}
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: requestTimeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrTokenRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %s", ErrTokenRequest, resp.Status)
	}

	var out struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("%w: %w", ErrTokenRequest, err)
	}
	if out.Value == "" {
		return "", fmt.Errorf("%w: empty token in response", ErrTokenRequest)
	}
	return out.Value, nil
}

// decodeClaims extracts the claims from a JWT payload (the middle segment). The token is GitHub's
// own, read here only to learn the current runtime context, so the signature is not verified.
func decodeClaims(jwt string) (*Claims, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: malformed JWT", ErrTokenDecode)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTokenDecode, err)
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTokenDecode, err)
	}
	return &c, nil
}

// getenv reads a GitHub Actions runtime environment variable.
func getenv(name string) string {
	// These are external GitHub Actions runtime variables, not Atmos configuration.
	//nolint:forbidigo // GitHub Actions OIDC env vars are external CI signals, not Atmos config.
	return os.Getenv(name)
}
