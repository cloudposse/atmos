package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/versent/saml2aws/v2"
	"github.com/versent/saml2aws/v2/pkg/cfg"
	"github.com/versent/saml2aws/v2/pkg/creds"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	samlTimeoutSeconds    = 30
	samlDefaultSessionSec = 3600
	logFieldRole          = "role"
	minSTSSeconds         = 900
	maxSTSSeconds         = 43200
)

type assumeRoleWithSAMLClient interface {
	AssumeRoleWithSAML(ctx context.Context, params *sts.AssumeRoleWithSAMLInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleWithSAMLOutput, error)
}

// samlProvider implements AWS SAML authentication using saml2aws.
type samlProvider struct {
	name   string
	config *schema.Provider
	url    string
	region string
	// RoleToAssumeFromAssertion is set by PreAuthenticate based on the next identity in the chain.
	RoleToAssumeFromAssertion string
}

// NewSAMLProvider creates a new AWS SAML provider.
func NewSAMLProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config.Kind != "aws/saml" {
		return nil, fmt.Errorf("%w: invalid provider kind for SAML provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	if config.URL == "" {
		return nil, fmt.Errorf("%w: url is required for AWS SAML provider", errUtils.ErrInvalidProviderConfig)
	}

	if config.Region == "" {
		return nil, fmt.Errorf("%w: region is required for AWS SAML provider", errUtils.ErrInvalidProviderConfig)
	}

	return &samlProvider{
		name:                      name,
		config:                    config,
		url:                       config.URL,
		region:                    config.Region,
		RoleToAssumeFromAssertion: "",
	}, nil
}

// Kind returns the provider kind.
func (p *samlProvider) Kind() string {
	return "aws/saml"
}

// Name returns the configured provider name.
func (p *samlProvider) Name() string {
	return p.name
}

// PreAuthenticate records a hint (next identity name) to help role selection.
func (p *samlProvider) PreAuthenticate(manager types.AuthManager) error {
	// chain: [provider, identity1, identity2, ...]
	chain := manager.GetChain()
	log.Debug("SAML pre-auth: chain", "chain", chain)
	if len(chain) > 1 {
		identities := manager.GetIdentities()
		identity, exists := identities[chain[1]]
		log.Debug("SAML pre-auth: identity", "name", chain[1], "exists", exists)
		if !exists {
			return fmt.Errorf("%w: identity %q not found", errUtils.ErrInvalidAuthConfig, chain[1])
		}
		var roleArn string
		var ok bool
		if roleArn, ok = identity.Principal["assume_role"].(string); !ok || roleArn == "" {
			return fmt.Errorf("%w: assume_role is required in principal", errUtils.ErrInvalidIdentityConfig)
		}
		p.RoleToAssumeFromAssertion = roleArn
		log.Debug("SAML pre-auth: recorded role to assume from assertion", logFieldRole, p.RoleToAssumeFromAssertion)
	}

	return nil
}

// Authenticate performs SAML authentication using saml2aws.
func (p *samlProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	log.Info("Starting SAML authentication", "provider", p.name, "url", p.url)
	if p.RoleToAssumeFromAssertion == "" {
		return nil, fmt.Errorf("%w: no role to assume for assertion, SAML provider must be part of a chain", errUtils.ErrInvalidAuthConfig)
	}

	// Set up browser automation if needed.
	p.setupBrowserAutomation()

	// Create config and client + login details.
	samlConfig := p.createSAMLConfig()
	loginDetails := p.createLoginDetails()

	// Create the SAML client.
	samlClient, err := saml2aws.NewSAMLClient(samlConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create SAML client: %v", errUtils.ErrInvalidProviderConfig, err)
	}

	// Authenticate and get assertion.
	assertionB64, err := p.authenticateAndGetAssertion(samlClient, loginDetails)
	if err != nil {
		return nil, err
	}

	decodedXML, err := base64.StdEncoding.DecodeString(assertionB64)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode assertion: %v", errUtils.ErrAwsSAMLDecodeFailed, err)
	}
	rolesStr, err := saml2aws.ExtractAwsRoles(decodedXML)
	if err != nil {
		return nil, fmt.Errorf("%w: extract AWS roles: %v", errUtils.ErrAwsSAMLDecodeFailed, err)
	}
	roles, err := saml2aws.ParseAWSRoles(rolesStr)
	if err != nil {
		return nil, fmt.Errorf("%w: parse AWS roles: %v", errUtils.ErrAwsSAMLDecodeFailed, err)
	}

	selectedRole := p.selectRole(roles)
	if selectedRole == nil {
		return nil, fmt.Errorf("%w: no role selected", errUtils.ErrAuthenticationFailed)
	}

	// Assume role.
	awsCreds, err := p.assumeRoleWithSAML(ctx, assertionB64, selectedRole)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with SAML: %v", errUtils.ErrAuthenticationFailed, err)
	}

	log.Info("SAML authentication successful", "provider", p.name, logFieldRole, selectedRole.RoleARN)

	return awsCreds, nil
}

// createSAMLConfig creates the saml2aws configuration.
func (p *samlProvider) createSAMLConfig() *cfg.IDPAccount {
	return &cfg.IDPAccount{
		URL:                  p.url,
		Username:             p.config.Username,
		Provider:             p.getProviderType(),
		MFA:                  "Auto",
		SkipVerify:           false,
		Timeout:              samlTimeoutSeconds, // 30 second timeout
		AmazonWebservicesURN: "urn:amazon:webservices",
		SessionDuration:      samlDefaultSessionSec, // 1 hour default
		Profile:              p.name,
		Region:               p.region,
		DownloadBrowser:      p.config.DownloadBrowserDriver,
		Headless:             false, // Force non-headless for interactive auth
	}
}

// createLoginDetails creates the login details for saml2aws.
func (p *samlProvider) createLoginDetails() *creds.LoginDetails {
	loginDetails := &creds.LoginDetails{
		URL:      p.url,
		Username: p.config.Username,
	}

	// If password is provided in config, use it; otherwise prompt.
	if p.config.Password != "" {
		loginDetails.Password = p.config.Password
	}

	return loginDetails
}

// authenticateAndGetAssertion authenticates and gets the SAML assertion.
func (p *samlProvider) authenticateAndGetAssertion(samlClient saml2aws.SAMLClient, loginDetails *creds.LoginDetails) (string, error) {
	samlAssertion, err := samlClient.Authenticate(loginDetails)
	if err != nil {
		return "", fmt.Errorf("%w: SAML authentication failed: %v", errUtils.ErrAuthenticationFailed, err)
	}

	if samlAssertion == "" {
		return "", fmt.Errorf("%w: empty SAML assertion received", errUtils.ErrAuthenticationFailed)
	}

	log.Debug("SAML assertion received.", "length", len(samlAssertion))

	return samlAssertion, nil
}

// selectRole selects the AWS role (first for now, with logging).
func (p *samlProvider) selectRole(awsRoles []*saml2aws.AWSRole) *saml2aws.AWSRole {
	// Try preferred hint first, then fall back to the first role.
	hint := strings.ToLower(p.RoleToAssumeFromAssertion)
	for _, r := range awsRoles {
		if strings.Contains(strings.ToLower(r.RoleARN), hint) || strings.Contains(strings.ToLower(r.PrincipalARN), hint) {
			log.Debug("Selecting role matching preferred hint", logFieldRole, r.RoleARN, "hint", p.RoleToAssumeFromAssertion)
			return r
		}
	}

	if len(awsRoles) > 0 {
		log.Debug("No role matched hint; falling back to first role.", logFieldRole, awsRoles[0].RoleARN)
		return awsRoles[0]
	}
	return nil
}

// assumeRoleWithSAML assumes an AWS role using SAML assertion.
func (p *samlProvider) assumeRoleWithSAML(ctx context.Context, samlAssertion string, role *saml2aws.AWSRole) (*types.AWSCredentials, error) {
	return p.assumeRoleWithSAMLWithDeps(ctx, samlAssertion, role, config.LoadDefaultConfig, func(cfg aws.Config) assumeRoleWithSAMLClient {
		return sts.NewFromConfig(cfg)
	})
}

func (p *samlProvider) assumeRoleWithSAMLWithDeps(
	ctx context.Context,
	samlAssertion string,
	role *saml2aws.AWSRole,
	loader func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error),
	factory func(aws.Config) assumeRoleWithSAMLClient,
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
		return nil, fmt.Errorf("%w: failed to create AWS config: %v", errUtils.ErrAuthenticationFailed, err)
	}

	stsClient := factory(cfg)

	// Assume role with SAML.
	input := &sts.AssumeRoleWithSAMLInput{
		RoleArn:         aws.String(role.RoleARN),
		PrincipalArn:    aws.String(role.PrincipalARN),
		SAMLAssertion:   aws.String(samlAssertion),
		DurationSeconds: aws.Int32(p.requestedSessionSeconds()),
	}

	result, err := stsClient.AssumeRoleWithSAML(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with SAML: %v", errUtils.ErrAuthenticationFailed, err)
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
func (p *samlProvider) requestedSessionSeconds() int32 {
	// Defaults.
	var sec int32 = samlDefaultSessionSec
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

// getProviderType returns the SAML provider type based on configuration or URL detection.
func (p *samlProvider) getProviderType() string {
	if p.config.ProviderType != "" {
		return p.config.ProviderType
	}

	// Auto-detect provider type based on URL.
	if strings.Contains(p.url, "accounts.google.com") {
		return "Browser" // Use Browser for Google Apps SAML for better compatibility
	}
	if strings.Contains(p.url, "okta.com") {
		return "Okta"
	}
	if strings.Contains(p.url, "adfs") {
		return "ADFS"
	}

	// Default to Browser for generic SAML.
	return "Browser"
}

// Validate validates the provider configuration.
func (p *samlProvider) Validate() error {
	if p.url == "" {
		return fmt.Errorf("%w: URL is required for SAML provider", errUtils.ErrInvalidProviderConfig)
	}

	if p.region == "" {
		return fmt.Errorf("%w: region is required for SAML provider", errUtils.ErrInvalidProviderConfig)
	}

	// Validate URL format strictly.
	u, err := url.ParseRequestURI(p.url)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%w: invalid URL format: %v", errUtils.ErrInvalidProviderConfig, err)
	}

	return nil
}

// Environment returns environment variables for this provider.
func (p *samlProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Set AWS region.
	env["AWS_DEFAULT_REGION"] = p.region
	env["AWS_REGION"] = p.region

	// Set saml2aws specific environment variables if needed.
	if p.config.DownloadBrowserDriver {
		env["SAML2AWS_AUTO_BROWSER_DOWNLOAD"] = "true"
	}

	return env, nil
}

// setupBrowserAutomation sets up browser automation for SAML authentication.
func (p *samlProvider) setupBrowserAutomation() {
	// Set environment variables for browser automation.
	if p.config.DownloadBrowserDriver {
		os.Setenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD", "true")
	}

	// For Google Apps SAML, we need to use Browser provider type.
	if strings.Contains(p.url, "accounts.google.com") {
		log.Debug("Detected Google Apps SAML, using Browser provider")
	}
}

// Logout removes provider-specific credential storage.
func (p *samlProvider) Logout(ctx context.Context) error {
	fileManager, err := awsCloud.NewAWSFileManager()
	if err != nil {
		return errors.Join(errUtils.ErrLogoutFailed, err)
	}

	if err := fileManager.Cleanup(p.name); err != nil {
		log.Debug("Failed to cleanup AWS files for SAML provider", "provider", p.name, "error", err)
		return errors.Join(errUtils.ErrLogoutFailed, err)
	}

	log.Debug("Cleaned up AWS files for SAML provider", "provider", p.name)
	return nil
}
