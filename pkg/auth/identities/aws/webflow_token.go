package aws

// Token endpoint HTTP: exchange authorization code or refresh token for
// AWS credentials, plus response parsing and conversion helpers.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// exchangeCodeForCredentials exchanges an authorization code for AWS credentials.
func exchangeCodeForCredentials(ctx context.Context, client HTTPClient, p exchangeCodeParams) (*webflowTokenResponse, error) {
	defer perf.Track(nil, "aws.exchangeCodeForCredentials")()

	body := url.Values{}
	body.Set("client_id", webflowOAuthClientID)
	body.Set(webflowGrantTypeKey, webflowGrantTypeAuthCode)
	body.Set("code", p.code)
	body.Set("code_verifier", p.codeVerifier)
	body.Set("redirect_uri", p.redirectURI)

	return callTokenEndpoint(ctx, client, p.region, body)
}

// exchangeRefreshToken exchanges a refresh token for new AWS credentials.
func exchangeRefreshToken(ctx context.Context, client HTTPClient, region, refreshToken string) (*webflowTokenResponse, error) {
	defer perf.Track(nil, "aws.exchangeRefreshToken")()

	body := url.Values{}
	body.Set("client_id", webflowOAuthClientID)
	body.Set(webflowGrantTypeKey, webflowGrantTypeRefresh)
	body.Set("refresh_token", refreshToken)

	return callTokenEndpoint(ctx, client, region, body)
}

// callTokenEndpoint makes a POST request to the AWS signin token endpoint.
func callTokenEndpoint(ctx context.Context, client HTTPClient, region string, body url.Values) (*webflowTokenResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/token", getSigninEndpoint(region))

	respBody, statusCode, err := doTokenRequest(ctx, client, endpoint, region, body)
	if err != nil {
		return nil, err
	}

	if statusCode != http.StatusOK {
		return nil, decodeTokenErrorResponse(respBody, statusCode, endpoint, body.Get(webflowGrantTypeKey))
	}

	return parseTokenSuccessResponse(respBody)
}

// doTokenRequest constructs and executes the HTTP request against the token
// endpoint and returns the raw body bytes plus HTTP status code.
//
// SECURITY: Debug logging here MUST NOT include the request or response body.
// The request body carries authorization codes, PKCE verifiers, and refresh
// tokens; the response body carries AWS STS credentials (access key, secret
// key, session token) and refresh tokens. Any body-level log becomes a secret
// exfiltration path for users running `atmos --log-level=debug auth login`.
// Only non-sensitive metadata (endpoint, grant_type, status code) may be
// logged. If body-level diagnostics are ever needed, redact first.
func doTokenRequest(ctx context.Context, client HTTPClient, endpoint, region string, body url.Values) ([]byte, int, error) {
	encodedBody := body.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(encodedBody))
	if err != nil {
		return nil, 0, fmt.Errorf("%w: failed to create request: %w", errUtils.ErrWebflowTokenExchange, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	log.Debug("Token exchange request",
		"endpoint", endpoint,
		webflowGrantTypeKey, body.Get(webflowGrantTypeKey),
	)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, errUtils.Build(errUtils.ErrWebflowTokenExchange).
			WithCause(err).
			WithExplanation("Failed to contact the AWS signin service").
			WithHint("Check your network connectivity").
			WithHintf("Ensure the region '%s' is correct", region).
			WithContext("endpoint", endpoint).
			Err()
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, webflowTokenMaxBodyBytes))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: failed to read response: %w", errUtils.ErrWebflowTokenExchange, err)
	}

	log.Debug("Token exchange response",
		"endpoint", endpoint,
		"status", resp.StatusCode,
	)
	return respBody, resp.StatusCode, nil
}

// decodeTokenErrorResponse turns a non-200 response body into an error wrapped
// with either ErrWebflowRefreshTokenRevoked (definitive refresh-token
// rejection) or ErrWebflowTokenExchange (all other failures).
func decodeTokenErrorResponse(respBody []byte, statusCode int, endpoint, grantType string) error {
	var errResp webflowTokenErrorResponse
	if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil && errResp.Error != "" {
		// Per RFC 6749 §5.2, invalid_grant/invalid_token on a refresh_token grant
		// mean the refresh token has been definitively rejected and will never
		// work again. Wrap with ErrWebflowRefreshTokenRevoked so callers can
		// distinguish this from transient failures (HTTP 5xx, 429, network errors)
		// and delete the cached refresh token only in this case.
		sentinel := errUtils.ErrWebflowTokenExchange
		if grantType == webflowGrantTypeRefresh && isDefinitiveOAuthError(errResp.Error) {
			sentinel = errUtils.ErrWebflowRefreshTokenRevoked
		}
		return fmt.Errorf("%w: %s: %s (HTTP %d, endpoint: %s)",
			sentinel, errResp.Error, errResp.ErrorDescription, statusCode, endpoint)
	}
	return fmt.Errorf("%w: HTTP %d (endpoint: %s, body: %s)",
		errUtils.ErrWebflowTokenExchange, statusCode, endpoint, string(respBody))
}

// parseTokenSuccessResponse decodes a 200 response body into a
// webflowTokenResponse, validating that it contains credentials.
func parseTokenSuccessResponse(respBody []byte) (*webflowTokenResponse, error) {
	var tokenResp webflowTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse token response: %w", errUtils.ErrWebflowTokenExchange, err)
	}

	if tokenResp.AccessToken.AccessKeyID == "" || tokenResp.AccessToken.SecretAccessKey == "" {
		return nil, fmt.Errorf("%w: token response missing credentials", errUtils.ErrWebflowTokenExchange)
	}

	return &tokenResp, nil
}

// tokenResponseToCredentials converts a token response to AWSCredentials.
func tokenResponseToCredentials(resp *webflowTokenResponse, region string) *types.AWSCredentials {
	expiration := ""
	if resp.ExpiresIn > 0 {
		expiration = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).Format(time.RFC3339)
	}

	return &types.AWSCredentials{
		AccessKeyID:     resp.AccessToken.AccessKeyID,
		SecretAccessKey: resp.AccessToken.SecretAccessKey,
		SessionToken:    resp.AccessToken.SessionToken,
		Region:          region,
		Expiration:      expiration,
	}
}

// isDefinitiveOAuthError reports whether an OAuth2 `error` field from the token
// endpoint means the refresh token has been definitively rejected and will never
// work again. Per RFC 6749 §5.2, only `invalid_grant` (and the rarely-used
// `invalid_token` for bearer-token endpoints) carry that meaning. All other
// error codes — `invalid_request`, `invalid_client`, `unauthorized_client`,
// `unsupported_grant_type`, `invalid_scope` — indicate request/config problems
// that do not invalidate the refresh token itself.
func isDefinitiveOAuthError(oauthErr string) bool {
	switch oauthErr {
	case "invalid_grant", "invalid_token":
		return true
	}
	return false
}
