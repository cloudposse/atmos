package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc/types"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	ssoDefaultSessionMinutes = 60
)

// ssoProvider implements AWS IAM Identity Center authentication.
type ssoProvider struct {
	name     string
	config   *schema.Provider
	startURL string
	region   string
}

// NewSSOProvider creates a new AWS SSO provider.
func NewSSOProvider(name string, config *schema.Provider) (*ssoProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != "aws/iam-identity-center" {
		return nil, fmt.Errorf("%w: invalid provider kind for SSO provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	if config.StartURL == "" {
		return nil, fmt.Errorf("%w: start_url is required for AWS SSO provider", errUtils.ErrInvalidProviderConfig)
	}

	if config.Region == "" {
		return nil, fmt.Errorf("%w: region is required for AWS SSO provider", errUtils.ErrInvalidProviderConfig)
	}

	return &ssoProvider{
		name:     name,
		config:   config,
		startURL: config.StartURL,
		region:   config.Region,
	}, nil
}

// Kind returns the provider kind.
func (p *ssoProvider) Kind() string {
	return "aws/iam-identity-center"
}

// Name returns the configured provider name.
func (p *ssoProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for SSO provider.
func (p *ssoProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// Authenticate performs AWS SSO authentication.
func (p *ssoProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	// Note: SSO provider no longer caches credentials directly.
	// Caching is handled at the manager level to prevent duplicates.

	// Build config options.
	configOpts := []func(*config.LoadOptions) error{
		config.WithRegion(p.region),
	}

	// Add custom endpoint resolver if configured.
	if resolverOpt := awsCloud.GetResolverConfigOption(nil, p.config); resolverOpt != nil {
		configOpts = append(configOpts, resolverOpt)
	}

	// Initialize AWS config for the SSO region.
	cfg, err := config.LoadDefaultConfig(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Create OIDC client for device authorization.
	oidcClient := ssooidc.NewFromConfig(cfg)

	// Register the client.
	registerResp, err := oidcClient.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("atmos-auth"),
		ClientType: aws.String("public"),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to register SSO client: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Start device authorization.
	authResp, err := oidcClient.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     registerResp.ClientId,
		ClientSecret: registerResp.ClientSecret,
		StartUrl:     aws.String(p.startURL),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to start device authorization: %v", errUtils.ErrAuthenticationFailed, err)
	}

	p.promptDeviceAuth(authResp)
	// Poll for token using helper to keep function size small.
	accessToken, tokenExpiresAt, err := p.pollForAccessToken(ctx, oidcClient, registerResp, authResp)
	if err != nil {
		return nil, err
	}

	// Calculate expiration time.
	// Use token expiration (fallback to session duration if unavailable).
	expiration := tokenExpiresAt
	if expiration.IsZero() {
		expiration = time.Now().Add(time.Duration(p.getSessionDuration()) * time.Minute)
	}
	log.Debug("Authentication successful", "expiration", expiration)

	return &authTypes.AWSCredentials{
		AccessKeyID: accessToken, // Used by identities to get actual credentials
		Region:      p.region,
		Expiration:  expiration.Format(time.RFC3339),
	}, nil
}

// promptDeviceAuth displays user code and verification URI if not in CI.
func (p *ssoProvider) promptDeviceAuth(authResp *ssooidc.StartDeviceAuthorizationOutput) {
	code := ""
	if authResp.UserCode != nil {
		code = *authResp.UserCode
	}
	if !telemetry.IsCI() {
		if authResp.VerificationUriComplete != nil && *authResp.VerificationUriComplete != "" {
			if err := utils.OpenUrl(*authResp.VerificationUriComplete); err != nil {
				log.Debug(err)
				utils.PrintfMessageToTUI("🔐 Please visit %s and enter code: %s.", *authResp.VerificationUriComplete, code)
			}
		}
	}
}

// Validate validates the provider configuration.
func (p *ssoProvider) Validate() error {
	if p.startURL == "" {
		return fmt.Errorf("%w: start_url is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.region == "" {
		return fmt.Errorf("%w: region is required", errUtils.ErrInvalidProviderConfig)
	}

	// Validate spec.files.base_path if provided.
	if err := awsCloud.ValidateFilesBasePath(p.config); err != nil {
		return err
	}

	return nil
}

// Environment returns environment variables for this provider.
func (p *ssoProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	env["AWS_REGION"] = p.region
	return env, nil
}

// Note: SSO caching is now handled at the manager level to prevent duplicate entries.

// getSessionDuration returns the session duration in minutes.
func (p *ssoProvider) getSessionDuration() int {
	if p.config.Session != nil && p.config.Session.Duration != "" {
		// Parse duration (e.g., "15m", "1h").
		if duration, err := time.ParseDuration(p.config.Session.Duration); err == nil {
			return int(duration.Minutes())
		}
	}
	return ssoDefaultSessionMinutes // Default to 60 minutes
}

// pollForAccessToken polls the device authorization endpoint until an access token is available or times out.
func (p *ssoProvider) pollForAccessToken(ctx context.Context, oidcClient *ssooidc.Client, registerResp *ssooidc.RegisterClientOutput, authResp *ssooidc.StartDeviceAuthorizationOutput) (string, time.Time, error) {
	var accessToken string
	var tokenExpiresAt time.Time
	expiresIn := authResp.ExpiresIn
	interval := authResp.Interval
	// Normalize to a sane minimum to avoid divide-by-zero and busy-waiting.
	if interval <= 0 {
		interval = 1
	}

	intervalDur := time.Duration(interval) * time.Second

	// Initial delay before first poll.
	time.Sleep(intervalDur)
	for i := 0; i < int(expiresIn/interval); i++ {
		tokenResp, err := oidcClient.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     registerResp.ClientId,
			ClientSecret: registerResp.ClientSecret,
			DeviceCode:   authResp.DeviceCode,
			GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
		})
		if err == nil {
			accessToken = aws.ToString(tokenResp.AccessToken)
			tokenExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
			break
		}

		var authPendingErr *types.AuthorizationPendingException
		var slowDownErr *types.SlowDownException

		if errors.As(err, &authPendingErr) {
			time.Sleep(intervalDur)
			continue
		} else if errors.As(err, &slowDownErr) {
			// Slow down: double the interval as requested by the server.
			intervalDur = time.Duration(interval*2) * time.Second
			time.Sleep(intervalDur)
			continue
		}

		return "", time.Time{}, fmt.Errorf("%w: failed to create token: %v", errUtils.ErrAuthenticationFailed, err)
	}

	if accessToken == "" {
		return "", time.Time{}, fmt.Errorf("%w: authentication timed out", errUtils.ErrAuthenticationFailed)
	}
	return accessToken, tokenExpiresAt, nil
}

// Logout removes provider-specific credential storage.
func (p *ssoProvider) Logout(ctx context.Context) error {
	// Get base_path from provider spec if configured.
	basePath := awsCloud.GetFilesBasePath(p.config)

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return errors.Join(errUtils.ErrLogoutFailed, err)
	}

	if err := fileManager.Cleanup(p.name); err != nil {
		log.Debug("Failed to cleanup AWS files for SSO provider", "provider", p.name, "error", err)
		return errors.Join(errUtils.ErrLogoutFailed, err)
	}

	log.Debug("Cleaned up AWS files for SSO provider", "provider", p.name)
	return nil
}

// GetFilesDisplayPath returns the display path for AWS credential files.
func (p *ssoProvider) GetFilesDisplayPath() string {
	basePath := awsCloud.GetFilesBasePath(p.config)

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return "~/.aws/atmos"
	}

	return fileManager.GetDisplayPath()
}
