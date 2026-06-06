package gcp

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/option"

	"github.com/cloudposse/atmos/pkg/perf"
)

// IAMCredentialsService provides access to IAM Credentials API token generation.
type IAMCredentialsService interface {
	GenerateAccessToken(ctx context.Context, name string, req *iamcredentials.GenerateAccessTokenRequest) (*iamcredentials.GenerateAccessTokenResponse, error)
}

// IAMCredentialsServiceFactory creates IAM credentials service instances for a token.
type IAMCredentialsServiceFactory func(ctx context.Context, accessToken string) (IAMCredentialsService, error)

type iamCredentialsService struct {
	svc *iamcredentials.Service
}

func (s *iamCredentialsService) GenerateAccessToken(ctx context.Context, name string, req *iamcredentials.GenerateAccessTokenRequest) (*iamcredentials.GenerateAccessTokenResponse, error) {
	return s.svc.Projects.ServiceAccounts.GenerateAccessToken(name, req).Context(ctx).Do()
}

// NewIAMCredentialsService creates an IAM Credentials service using an access token.
func NewIAMCredentialsService(ctx context.Context, accessToken string) (IAMCredentialsService, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	svc, err := iamcredentials.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	return &iamCredentialsService{svc: svc}, nil
}

// ImpersonateServiceAccount generates an access token for a service account.
func ImpersonateServiceAccount(
	ctx context.Context,
	svc IAMCredentialsService,
	serviceAccountEmail string,
	scopes []string,
	delegates []string,
	lifetime string,
) (string, time.Time, error) {
	defer perf.Track(nil, "gcp.ImpersonateServiceAccount")()

	if svc == nil {
		return "", time.Time{}, fmt.Errorf("IAM credentials service is required")
	}
	name := fmt.Sprintf("projects/-/serviceAccounts/%s", serviceAccountEmail)
	req := &iamcredentials.GenerateAccessTokenRequest{
		Scope:     scopes,
		Delegates: delegates,
		Lifetime:  lifetime,
	}

	resp, err := svc.GenerateAccessToken(ctx, name, req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate access token: %w", err)
	}
	if resp == nil {
		return "", time.Time{}, fmt.Errorf("generate access token: empty response")
	}

	expiry, err := time.Parse(time.RFC3339, resp.ExpireTime)
	if err != nil {
		expiry = time.Now().Add(1 * time.Hour)
	}

	return resp.AccessToken, expiry, nil
}
