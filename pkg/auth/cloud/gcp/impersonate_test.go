package gcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iamcredentials/v1"
)

type mockIAMService struct {
	resp *iamcredentials.GenerateAccessTokenResponse
	err  error
}

func (m *mockIAMService) GenerateAccessToken(_ context.Context, _ string, _ *iamcredentials.GenerateAccessTokenRequest) (*iamcredentials.GenerateAccessTokenResponse, error) {
	return m.resp, m.err
}

func TestImpersonateServiceAccount_Success(t *testing.T) {
	expiry := time.Now().Add(30 * time.Minute).UTC()
	svc := &mockIAMService{
		resp: &iamcredentials.GenerateAccessTokenResponse{
			AccessToken: "generated-token",
			ExpireTime:  expiry.Format(time.RFC3339),
		},
	}

	token, exp, err := ImpersonateServiceAccount(
		context.Background(),
		svc,
		"sa@proj.iam.gserviceaccount.com",
		[]string{"https://www.googleapis.com/auth/cloud-platform"},
		nil,
		"3600s",
	)
	require.NoError(t, err)
	assert.Equal(t, "generated-token", token)
	assert.WithinDuration(t, expiry, exp, time.Second)
}

func TestImpersonateServiceAccount_NilService(t *testing.T) {
	_, _, err := ImpersonateServiceAccount(
		context.Background(),
		nil,
		"sa@proj.iam.gserviceaccount.com",
		[]string{"https://www.googleapis.com/auth/cloud-platform"},
		nil,
		"",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IAM credentials service is required")
}

func TestImpersonateServiceAccount_APIError(t *testing.T) {
	svc := &mockIAMService{err: errors.New("permission denied")}

	_, _, err := ImpersonateServiceAccount(
		context.Background(),
		svc,
		"sa@proj.iam.gserviceaccount.com",
		[]string{"https://www.googleapis.com/auth/cloud-platform"},
		nil,
		"",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generate access token")
	assert.Contains(t, err.Error(), "permission denied")
}

func TestImpersonateServiceAccount_NilResponse(t *testing.T) {
	svc := &mockIAMService{resp: nil}

	_, _, err := ImpersonateServiceAccount(
		context.Background(),
		svc,
		"sa@proj.iam.gserviceaccount.com",
		[]string{"https://www.googleapis.com/auth/cloud-platform"},
		nil,
		"",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestImpersonateServiceAccount_InvalidExpiry(t *testing.T) {
	svc := &mockIAMService{
		resp: &iamcredentials.GenerateAccessTokenResponse{
			AccessToken: "token-with-bad-expiry",
			ExpireTime:  "not-a-valid-time",
		},
	}

	token, exp, err := ImpersonateServiceAccount(
		context.Background(),
		svc,
		"sa@proj.iam.gserviceaccount.com",
		[]string{"https://www.googleapis.com/auth/cloud-platform"},
		nil,
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, "token-with-bad-expiry", token)
	// Should fall back to ~1 hour from now.
	assert.WithinDuration(t, time.Now().Add(1*time.Hour), exp, 5*time.Second)
}

func TestImpersonateServiceAccount_WithDelegates(t *testing.T) {
	var capturedReq *iamcredentials.GenerateAccessTokenRequest
	svc := &mockIAMService{
		resp: &iamcredentials.GenerateAccessTokenResponse{
			AccessToken: "delegated-token",
			ExpireTime:  time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	// Wrap to capture the request.
	wrapper := &capturingIAMService{inner: svc, captured: &capturedReq}

	delegates := []string{
		"projects/-/serviceAccounts/intermediate@proj.iam.gserviceaccount.com",
	}
	_, _, err := ImpersonateServiceAccount(
		context.Background(),
		wrapper,
		"sa@proj.iam.gserviceaccount.com",
		[]string{"scope1"},
		delegates,
		"1800s",
	)
	require.NoError(t, err)
	require.NotNil(t, capturedReq)
	assert.Equal(t, delegates, capturedReq.Delegates)
	assert.Equal(t, "1800s", capturedReq.Lifetime)
	assert.Equal(t, []string{"scope1"}, capturedReq.Scope)
}

type capturingIAMService struct {
	inner    IAMCredentialsService
	captured **iamcredentials.GenerateAccessTokenRequest
}

func (c *capturingIAMService) GenerateAccessToken(ctx context.Context, name string, req *iamcredentials.GenerateAccessTokenRequest) (*iamcredentials.GenerateAccessTokenResponse, error) {
	*c.captured = req
	return c.inner.GenerateAccessToken(ctx, name, req)
}
