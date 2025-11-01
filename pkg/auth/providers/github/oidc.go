package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	OidcTimeout            = 10
	defaultSessionSeconds  = 3600
	minSTSSeconds          = 900
	maxSTSSeconds          = 43200
	defaultRoleSessionName = "atmos-github-oidc"
)

type assumeRoleWithWebIdentityClient interface {
	AssumeRoleWithWebIdentity(ctx context.Context, params *sts.AssumeRoleWithWebIdentityInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error)
}

// oidcProvider implements GitHub OIDC authentication.
type oidcProvider struct {
	name   string
	config *schema.Provider
	region string
	// RoleToAssumeFromWebIdentity is set by PreAuthenticate based on the next identity in the chain.
	RoleToAssumeFromWebIdentity string
}

// NewOIDCProvider creates a new GitHub OIDC provider.
func NewOIDCProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}

	if name == "" {
		return nil, fmt.Errorf("%w: provider name is required", errUtils.ErrInvalidProviderConfig)
	}

	if config.Kind != "github/oidc" {
		return nil, fmt.Errorf("%w: invalid provider kind for GitHub OIDC provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Region is required for AWS STS calls
	if config.Region == "" {
		return nil, fmt.Errorf("%w: region is required for GitHub OIDC provider", errUtils.ErrInvalidProviderConfig)
	}

	return &oidcProvider{
		name:                        name,
		config:                      config,
		region:                      config.Region,
		RoleToAssumeFromWebIdentity: "",
	}, nil
}

// Name returns the provider name.
func (p *oidcProvider) Name() string {
	return p.name
}

// PreAuthenticate records a hint (next identity name) to help role selection.
func (p *oidcProvider) PreAuthenticate(manager types.AuthManager) error {
	// chain: [provider, identity1, identity2, ...]
	chain := manager.GetChain()
	log.Debug("GitHub OIDC pre-auth: chain", "chain", chain)
	if len(chain) > 1 {
		identities := manager.GetIdentities()
		identity, exists := identities[chain[1]]
		log.Debug("GitHub OIDC pre-auth: identity", "name", chain[1], "exists", exists)
		if !exists {
			return fmt.Errorf("%w: identity %q not found", errUtils.ErrInvalidAuthConfig, chain[1])
		}
		var roleArn string
		var ok bool
		if roleArn, ok = identity.Principal["assume_role"].(string); !ok || roleArn == "" {
			return fmt.Errorf("%w: assume_role is required in principal", errUtils.ErrInvalidIdentityConfig)
		}
		p.RoleToAssumeFromWebIdentity = roleArn
		log.Debug("GitHub OIDC pre-auth: recorded role to assume from web identity", "role", p.RoleToAssumeFromWebIdentity)
	}

	return nil
}

// Kind returns the provider kind.
func (p *oidcProvider) Kind() string {
	return "github/oidc"
}

// Authenticate performs GitHub OIDC authentication with AWS AssumeRoleWithWebIdentity.
func (p *oidcProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	log.Info("Starting GitHub OIDC authentication", "provider", p.name)

	// Validate provider configuration early.
	if err := p.Validate(); err != nil {
		return nil, err
	}

	// Check if we're running in GitHub Actions.
	if !p.isGitHubActions() {
		return nil, fmt.Errorf("%w: GitHub OIDC authentication is only available in GitHub Actions environment", errUtils.ErrAuthenticationFailed)
	}

	// Ensure role ARN is set from PreAuthenticate
	if p.RoleToAssumeFromWebIdentity == "" {
		return nil, fmt.Errorf("%w: no role to assume for web identity, GitHub OIDC provider must be part of a chain", errUtils.ErrInvalidAuthConfig)
	}

	requestURL, token, err := p.requestParams()
	if err != nil {
		return nil, err
	}

	aud, err := p.audience()
	if err != nil {
		return nil, err
	}

	jwtToken, err := p.resolveJWT(ctx, requestURL, token, aud)
	if err != nil {
		return nil, err
	}

	// Assume AWS role using the GitHub OIDC token
	awsCreds, err := p.assumeRoleWithWebIdentity(ctx, jwtToken)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with web identity: %w", errUtils.ErrAuthenticationFailed, err)
	}

	log.Info("GitHub OIDC authentication successful", "provider", p.name, "role", p.RoleToAssumeFromWebIdentity)

	return awsCreds, nil
}

// isGitHubActions checks if we're running in GitHub Actions environment.
func (p *oidcProvider) isGitHubActions() bool {
	if err := viper.BindEnv("github.actions", "GITHUB_ACTIONS"); err != nil {
		log.Trace("Failed to bind github.actions environment variable", "error", err)
	}
	return viper.GetString("github.actions") == "true"
}

// requestParams loads the request URL and token from the GitHub Actions environment.
func (p *oidcProvider) requestParams() (string, string, error) {
	if err := viper.BindEnv("github.oidc.request_token", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"); err != nil {
		log.Trace("Failed to bind github.oidc.request_token environment variable", "error", err)
	}
	token := viper.GetString("github.oidc.request_token")
	if token == "" {
		return "", "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_TOKEN not found - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}

	if err := viper.BindEnv("github.oidc.request_url", "ACTIONS_ID_TOKEN_REQUEST_URL"); err != nil {
		log.Trace("Failed to bind github.oidc.request_url environment variable", "error", err)
	}
	requestURL := viper.GetString("github.oidc.request_url")
	if requestURL == "" {
		return "", "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL not found - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}
	return requestURL, token, nil
}

// audience extracts the required audience from provider config.
func (p *oidcProvider) audience() (string, error) {
	if p.config != nil && p.config.Spec != nil {
		if v, ok := p.config.Spec["audience"].(string); ok && v != "" {
			return v, nil
		}
		return "", fmt.Errorf("%w: audience is required in provider spec", errUtils.ErrInvalidProviderConfig)
	}
	return "", nil
}

// resolveJWT returns an ACTIONS_ID_TOKEN if present, otherwise fetches a token from the endpoint.
func (p *oidcProvider) resolveJWT(ctx context.Context, requestURL, token, aud string) (string, error) {
	if err := viper.BindEnv("github.oidc.id_token", "ACTIONS_ID_TOKEN"); err != nil {
		log.Trace("Failed to bind github.oidc.id_token environment variable", "error", err)
	}
	if jwt := viper.GetString("github.oidc.id_token"); jwt != "" {
		return jwt, nil
	}
	jwtToken, err := p.getOIDCToken(ctx, requestURL, token, aud)
	if err != nil {
		return "", errors.Join(errUtils.ErrAuthenticationFailed, err)
	}
	return jwtToken, nil
}

// getOIDCToken retrieves the JWT token from GitHub's OIDC endpoint.
func (p *oidcProvider) getOIDCToken(ctx context.Context, requestURL, requestToken, audience string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: create OIDC request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	q := req.URL.Query()
	if audience != "" {
		q.Set("audience", audience)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: OidcTimeout * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: call OIDC endpoint: %w", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: OIDC endpoint returned status %s", errUtils.ErrAuthenticationFailed, resp.Status)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("%w: decode OIDC response: %w", errUtils.ErrAuthenticationFailed, err)
	}
	if out.Value == "" {
		return "", fmt.Errorf("%w: empty token in OIDC response", errUtils.ErrAuthenticationFailed)
	}
	return out.Value, nil
}

// assumeRoleWithWebIdentity assumes an AWS role using the GitHub OIDC token.
func (p *oidcProvider) assumeRoleWithWebIdentity(ctx context.Context, webIdentityToken string) (*types.AWSCredentials, error) {
	// Use LoadIsolatedAWSConfig to avoid conflicts with external AWS env vars.
	return p.assumeRoleWithWebIdentityWithDeps(ctx, webIdentityToken, awsCloud.LoadIsolatedAWSConfig, func(cfg aws.Config) assumeRoleWithWebIdentityClient {
		return sts.NewFromConfig(cfg)
	})
}

func (p *oidcProvider) assumeRoleWithWebIdentityWithDeps(
	ctx context.Context,
	webIdentityToken string,
	loader func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error),
	factory func(aws.Config) assumeRoleWithWebIdentityClient,
) (*types.AWSCredentials, error) {
	// Build config options
	configOpts := []func(*config.LoadOptions) error{
		config.WithRegion(p.region),
	}

	// Add custom endpoint resolver if configured
	if resolverOpt := awsCloud.GetResolverConfigOption(nil, p.config); resolverOpt != nil {
		configOpts = append(configOpts, resolverOpt)
	}

	// Load AWS configuration (v2).
	cfg, err := loader(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create AWS config: %w", errUtils.ErrAuthenticationFailed, err)
	}

	stsClient := factory(cfg)

	// Get role session name from config or use default
	roleSessionName := defaultRoleSessionName
	if p.config.Spec != nil {
		if name, ok := p.config.Spec["role_session_name"].(string); ok && name != "" {
			roleSessionName = name
		}
	}

	// Assume role with web identity.
	input := &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(p.RoleToAssumeFromWebIdentity),
		WebIdentityToken: aws.String(webIdentityToken),
		RoleSessionName:  aws.String(roleSessionName),
		DurationSeconds:  aws.Int32(p.requestedSessionSeconds()),
	}

	result, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with web identity: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Convert to AWSCredentials.
	creds := &types.AWSCredentials{
		AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(result.Credentials.SessionToken),
		Region:          p.region,
		Expiration:      result.Credentials.Expiration.Format(time.RFC3339),
	}

	return creds, nil
}

// requestedSessionSeconds returns the desired session duration within STS/account limits.
func (p *oidcProvider) requestedSessionSeconds() int32 {
	// Defaults.
	var sec int32 = defaultSessionSeconds
	if p.config == nil || p.config.Session == nil || p.config.Session.Duration == "" {
		return sec
	}
	if duration, err := time.ParseDuration(p.config.Session.Duration); err == nil {
		sec = int32(duration.Seconds())
		if sec < minSTSSeconds {
			sec = minSTSSeconds
		}
		if sec > maxSTSSeconds {
			sec = maxSTSSeconds
		}
	}
	return sec
}

// Validate validates the provider configuration.
func (p *oidcProvider) Validate() error {
	audience, err := p.audience()
	if err != nil {
		return err
	}
	if audience == "" {
		return fmt.Errorf("%w: audience is required in provider spec", errUtils.ErrInvalidProviderConfig)
	}
	if p.region == "" {
		return fmt.Errorf("%w: region is required for GitHub OIDC provider", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns environment variables for this provider.
func (p *oidcProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Set AWS region for downstream processes
	env["AWS_DEFAULT_REGION"] = p.region
	env["AWS_REGION"] = p.region

	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For GitHub OIDC providers, this method is typically not called directly since GitHub OIDC providers
// authenticate to get identity credentials, which then have their own PrepareEnvironment.
// However, we implement it for interface compliance.
func (p *oidcProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "github.oidcProvider.PrepareEnvironment")()

	// GitHub OIDC provider doesn't write credential files itself - that's done by identities.
	// Just return the environment unchanged.
	return environ, nil
}

// Logout removes provider-specific credential storage.
func (p *oidcProvider) Logout(ctx context.Context) error {
	defer perf.Track(nil, "github.oidcProvider.Logout")()

	// Get base_path from provider spec if configured.
	basePath := awsCloud.GetFilesBasePath(p.config)

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return errors.Join(errUtils.ErrProviderLogout, errUtils.ErrLogoutFailed, err)
	}

	if err := fileManager.Cleanup(p.name); err != nil {
		log.Debug("Failed to cleanup AWS files for GitHub OIDC provider", "provider", p.name, "error", err)
		return errors.Join(errUtils.ErrProviderLogout, errUtils.ErrLogoutFailed, err)
	}

	log.Debug("Cleaned up AWS files for GitHub OIDC provider", "provider", p.name)
	return nil
}

// GetFilesDisplayPath returns the display path for AWS credential files.
func (p *oidcProvider) GetFilesDisplayPath() string {
	defer perf.Track(nil, "github.oidcProvider.GetFilesDisplayPath")()

	basePath := awsCloud.GetFilesBasePath(p.config)

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return "~/.aws/atmos"
	}

	return fileManager.GetDisplayPath()
}
