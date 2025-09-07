package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	log "github.com/charmbracelet/log"
	"github.com/versent/saml2aws/v2"
	"github.com/versent/saml2aws/v2/pkg/cfg"
	"github.com/versent/saml2aws/v2/pkg/creds"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// samlProvider implements AWS SAML authentication using saml2aws.
type samlProvider struct {
	name   string
	config *schema.Provider
	url    string
	region string
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
		name:   name,
		config: config,
		url:    config.URL,
		region: config.Region,
	}, nil
}

// Kind returns the provider kind.
func (p *samlProvider) Kind() string {
	return "aws/saml"
}

// Authenticate performs SAML authentication using saml2aws.
func (p *samlProvider) Authenticate(ctx context.Context) (*schema.Credentials, error) {
	log.Info("Starting SAML authentication", "provider", p.name, "url", p.url)

	// Set up browser automation if needed
	p.setupBrowserAutomation()

	// Create config and client + login details
	samlConfig := p.createSAMLConfig()
	loginDetails := p.createLoginDetails()

	// Create the SAML client
	samlClient, err := saml2aws.NewSAMLClient(samlConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create SAML client: %v", errUtils.ErrInvalidProviderConfig, err)
	}

	// Authenticate and get assertion
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

	// Assume role
	awsCreds, err := p.assumeRoleWithSAML(ctx, assertionB64, selectedRole)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with SAML: %v", errUtils.ErrAuthenticationFailed, err)
	}

	log.Info("SAML authentication successful", "provider", p.name, "role", selectedRole.RoleARN)

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
		Timeout:              30, // 30 second timeout
		AmazonWebservicesURN: "urn:amazon:webservices",
		SessionDuration:      3600, // 1 hour default
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

	// If password is provided in config, use it; otherwise prompt
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
	// Use the first role or let user select if multiple
	selectedRole := awsRoles[0]
	if len(awsRoles) > 1 {
		log.Info("Multiple AWS roles available", "count", len(awsRoles))
		// For now, use the first role. In the future, we could add role selection logic
		log.Info("Using first available role", "role", selectedRole.RoleARN)
	}

	return selectedRole
}

// assumeRoleWithSAML assumes an AWS role using SAML assertion.
func (p *samlProvider) assumeRoleWithSAML(ctx context.Context, samlAssertion string, role *saml2aws.AWSRole) (*schema.Credentials, error) {
	// Load AWS configuration (v2)
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.region))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create AWS config: %v", errUtils.ErrAuthenticationFailed, err)
	}

	stsClient := sts.NewFromConfig(cfg)

	// Assume role with SAML
	input := &sts.AssumeRoleWithSAMLInput{
		RoleArn:       aws.String(role.RoleARN),
		PrincipalArn:  aws.String(role.PrincipalARN),
		SAMLAssertion: aws.String(samlAssertion),
		DurationSeconds: aws.Int32(func() int32 {
			// Respect requested duration within STS/account limits.
			if p.config.Session.Duration != "" {
				if duration, err := time.ParseDuration(p.config.Session.Duration); err == nil {
					return int32(duration.Seconds())
				}
			}
			return 3600
		}()),
	}

	result, err := stsClient.AssumeRoleWithSAML(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with SAML: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Convert to schema.Credentials
	creds := &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
			SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
			SessionToken:    aws.ToString(result.Credentials.SessionToken),
			Region:          p.region,
			Expiration:      result.Credentials.Expiration.Format(time.RFC3339),
		},
	}

	return creds, nil
}

// getProviderType returns the SAML provider type based on configuration or URL detection.
func (p *samlProvider) getProviderType() string {
	if p.config.ProviderType != "" {
		return p.config.ProviderType
	}

	// Auto-detect provider type based on URL
	if strings.Contains(p.url, "accounts.google.com") {
		return "Browser" // Use Browser for Google Apps SAML for better compatibility
	}
	if strings.Contains(p.url, "okta.com") {
		return "Okta"
	}
	if strings.Contains(p.url, "adfs") {
		return "ADFS"
	}

	// Default to Browser for generic SAML
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

	// Validate URL format
	if _, err := url.Parse(p.url); err != nil {
		return fmt.Errorf("%w: invalid URL format: %v", errUtils.ErrInvalidProviderConfig, err)
	}

	return nil
}

// Environment returns environment variables for this provider.
func (p *samlProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Set AWS region
	env["AWS_DEFAULT_REGION"] = p.region
	env["AWS_REGION"] = p.region

	// Set saml2aws specific environment variables if needed
	if p.config.DownloadBrowserDriver {
		env["SAML2AWS_AUTO_BROWSER_DOWNLOAD"] = "true"
	}

	return env, nil
}

// setupBrowserAutomation sets up browser automation for SAML authentication.
func (p *samlProvider) setupBrowserAutomation() {
	// Set environment variables for browser automation
	if p.config.DownloadBrowserDriver {
		os.Setenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD", "true")
	}

	// For Google Apps SAML, we need to use Browser provider type
	if strings.Contains(p.url, "accounts.google.com") {
		log.Debug("Detected Google Apps SAML, using Browser provider")
	}
}
