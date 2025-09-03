package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ssoProvider implements AWS IAM Identity Center authentication
type ssoProvider struct {
	name     string
	config   *schema.Provider
	startURL string
	region   string
}

// NewSSOProvider creates a new AWS SSO provider
func NewSSOProvider(name string, config *schema.Provider) (*ssoProvider, error) {
	if config.Kind != "aws/iam-identity-center" {
		return nil, fmt.Errorf("invalid provider kind for SSO provider: %s", config.Kind)
	}

	if config.StartURL == "" {
		return nil, fmt.Errorf("start_url is required for AWS SSO provider")
	}

	if config.Region == "" {
		return nil, fmt.Errorf("region is required for AWS SSO provider")
	}

	return &ssoProvider{
		name:     name,
		config:   config,
		startURL: config.StartURL,
		region:   config.Region,
	}, nil
}

// Kind returns the provider kind
func (p *ssoProvider) Kind() string {
	return "aws/iam-identity-center"
}

// Authenticate performs AWS SSO authentication
func (p *ssoProvider) Authenticate(ctx context.Context) (*schema.Credentials, error) {
	// Initialize AWS config for the SSO region
	cfg := aws.Config{
		Region: p.region,
	}

	// Create OIDC client for device authorization
	oidcClient := ssooidc.NewFromConfig(cfg)

	// Register the client
	registerResp, err := oidcClient.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("atmos-cli"),
		ClientType: aws.String("public"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to register SSO client: %w", err)
	}

	// Start device authorization
	authResp, err := oidcClient.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     registerResp.ClientId,
		ClientSecret: registerResp.ClientSecret,
		StartUrl:     aws.String(p.startURL),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start device authorization: %w", err)
	}

	// Display user code and verification URI
	fmt.Printf("Please visit %s and enter the code: %s\n",
		aws.ToString(authResp.VerificationUriComplete),
		aws.ToString(authResp.UserCode))

	// Poll for token
	var accessToken string
	expiresIn := authResp.ExpiresIn
	interval := authResp.Interval

	// Add initial delay before first poll
	time.Sleep(time.Duration(interval) * time.Second)

	for i := 0; i < int(expiresIn/interval); i++ {
		tokenResp, err := oidcClient.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     registerResp.ClientId,
			ClientSecret: registerResp.ClientSecret,
			DeviceCode:   authResp.DeviceCode,
			GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
		})

		if err == nil {
			accessToken = aws.ToString(tokenResp.AccessToken)
			break
		}

		// Check for specific AWS error types
		var authPendingErr *types.AuthorizationPendingException
		var slowDownErr *types.SlowDownException

		if errors.As(err, &authPendingErr) {
			// Authorization is still pending, continue polling
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		} else if errors.As(err, &slowDownErr) {
			// Slow down polling as requested by AWS
			time.Sleep(time.Duration(interval*2) * time.Second)
			continue
		}

		// Any other error is terminal
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	if accessToken == "" {
		return nil, fmt.Errorf("authentication timed out")
	}

	// Calculate expiration time
	expiration := time.Now().Add(time.Duration(p.getSessionDuration()) * time.Minute)

	return &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID: accessToken, // This will be used by identities to get actual credentials
			Region:      p.region,
			Expiration:  expiration.Format(time.RFC3339),
		},
	}, nil
}

// Validate validates the provider configuration
func (p *ssoProvider) Validate() error {
	if p.startURL == "" {
		return fmt.Errorf("start_url is required")
	}
	if p.region == "" {
		return fmt.Errorf("region is required")
	}
	return nil
}

// Environment returns environment variables for this provider
func (p *ssoProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	env["AWS_REGION"] = p.region
	return env, nil
}

// getSessionDuration returns the session duration in minutes
func (p *ssoProvider) getSessionDuration() int {
	if p.config.Session != nil && p.config.Session.Duration != "" {
		// Parse duration (e.g., "15m", "1h")
		if duration, err := time.ParseDuration(p.config.Session.Duration); err == nil {
			return int(duration.Minutes())
		}
	}
	return 60 // Default to 60 minutes
}
