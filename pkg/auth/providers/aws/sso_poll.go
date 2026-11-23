package aws

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc/types"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// ssoDeviceCodeGrantType is the OAuth 2.0 device authorization grant type (RFC 8628).
	ssoDeviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	// ssoSlowDownIncrement is how much a slow_down response increases the
	// polling interval for this and all later requests (RFC 8628 section 3.5).
	ssoSlowDownIncrement = 5 * time.Second
)

// waitForPollInterval waits for d, returning early with the context's error
// when ctx is cancelled or its deadline passes.
func waitForPollInterval(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// waitPoll waits one polling interval using the injected wait function, if any.
func (p *ssoProvider) waitPoll(ctx context.Context, d time.Duration) error {
	if p.pollWait != nil {
		return p.pollWait(ctx, d)
	}
	return waitForPollInterval(ctx, d)
}

// pollForAccessToken polls the device authorization endpoint until an access token is
// available or times out. The returned ssoTokenCache carries AccessToken, RefreshToken
// (when issued), and ExpiresAt; remaining fields are populated by runDeviceAuthFlow.
//
// Polling stops as soon as ctx is cancelled. A slow_down response adds five
// seconds to the interval, as RFC 8628 requires, and a terminal OIDC error is
// returned with the AWS error kept as its cause.
func (p *ssoProvider) pollForAccessToken(ctx context.Context, oidcClient ssoTokenRefresher, registerResp *ssooidc.RegisterClientOutput, authResp *ssooidc.StartDeviceAuthorizationOutput) (ssoTokenCache, error) {
	var result ssoTokenCache
	expiresIn := authResp.ExpiresIn
	interval := authResp.Interval
	// Normalize to a sane minimum to avoid divide-by-zero and busy-waiting.
	if interval <= 0 {
		interval = 1
	}

	intervalDur := time.Duration(interval) * time.Second
	// The device code is valid for expiresIn seconds; slow_down responses make
	// each poll wait longer, so the attempt count alone cannot bound polling.
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	// Initial delay before first poll.
	if err := p.waitPoll(ctx, intervalDur); err != nil {
		return ssoTokenCache{}, p.pollCancelledError(err)
	}
	for i := 0; i < int(expiresIn/interval); i++ {
		tokenResp, err := oidcClient.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     registerResp.ClientId,
			ClientSecret: registerResp.ClientSecret,
			DeviceCode:   authResp.DeviceCode,
			GrantType:    aws.String(ssoDeviceCodeGrantType),
		})
		if err == nil {
			result.AccessToken = aws.ToString(tokenResp.AccessToken)
			result.RefreshToken = aws.ToString(tokenResp.RefreshToken)
			result.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
			break
		}

		// A cancelled context also fails the request; report the cancellation.
		if ctx.Err() != nil {
			return ssoTokenCache{}, p.pollCancelledError(ctx.Err())
		}

		var authPendingErr *types.AuthorizationPendingException
		var slowDownErr *types.SlowDownException

		switch {
		case errors.As(err, &authPendingErr):
			// The user has not approved yet: poll again at the same interval.
		case errors.As(err, &slowDownErr):
			// Slow down: add five seconds to the interval (RFC 8628 section 3.5).
			intervalDur += ssoSlowDownIncrement
		default:
			return ssoTokenCache{}, errUtils.Build(errUtils.ErrSSOTokenCreationFailed).
				WithCause(err).
				WithExplanation("Failed to create AWS SSO access token").
				WithHint("Ensure you completed the device authorization in your browser").
				WithHint("The verification code may have expired - try authenticating again").
				WithContext("provider", p.name).
				WithContext("start_url", p.startURL).
				WithExitCode(1).
				Err()
		}

		if err := p.waitPoll(ctx, intervalDur); err != nil {
			return ssoTokenCache{}, p.pollCancelledError(err)
		}
		if time.Now().After(deadline) {
			break
		}
	}

	if result.AccessToken == "" {
		return ssoTokenCache{}, errUtils.Build(errUtils.ErrSSOTokenCreationFailed).
			WithExplanation("AWS SSO authentication timed out waiting for browser confirmation").
			WithHint("Complete the device authorization in your browser within the time limit").
			WithHint("Visit the verification URL and enter the code displayed earlier").
			WithHintf("Try again with `atmos auth login --provider=%s`; Atmos keeps its own SSO token cache, so `aws sso login` does not help", p.name).
			WithContext("provider", p.name).
			WithContext("start_url", p.startURL).
			WithContext("region", p.region).
			WithExitCode(1).
			Err()
	}
	return result, nil
}

// pollCancelledError reports that device-token polling stopped because its
// context was cancelled or timed out. The context error stays in the chain.
func (p *ssoProvider) pollCancelledError(cause error) error {
	return errUtils.Build(errUtils.ErrSSOTokenCreationFailed).
		WithCause(cause).
		WithExplanation("AWS SSO authentication was cancelled before the browser confirmation arrived").
		WithHintf("Try again with `atmos auth login --provider=%s`", p.name).
		WithContext("provider", p.name).
		WithContext("start_url", p.startURL).
		WithExitCode(1).
		Err()
}
