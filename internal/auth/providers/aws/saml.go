package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	log "github.com/charmbracelet/log"
	"github.com/versent/saml2aws/v2"
	"github.com/versent/saml2aws/v2/pkg/cfg"
	"github.com/versent/saml2aws/v2/pkg/creds"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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
	if err := p.setupBrowserAutomation(); err != nil {
		log.Warn("Failed to setup browser automation", "error", err)
	}

	// Create saml2aws configuration
	samlConfig := &cfg.IDPAccount{
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

	// Set up saml2aws client
	loginDetails := &creds.LoginDetails{
		URL:      p.url,
		Username: p.config.Username,
	}

	// If password is provided in config, use it; otherwise prompt
	if p.config.Password != "" {
		loginDetails.Password = p.config.Password
	}

	// Create the SAML client
	samlClient, err := saml2aws.NewSAMLClient(samlConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create SAML client: %v", errUtils.ErrInvalidProviderConfig, err)
	}

	// Perform authentication
	samlAssertion, err := samlClient.Authenticate(loginDetails)
	if err != nil {
		return nil, fmt.Errorf("%w: SAML authentication failed: %v", errUtils.ErrAuthenticationFailed, err)
	}

	if samlAssertion == "" {
		return nil, fmt.Errorf("%w: empty SAML assertion received", errUtils.ErrAuthenticationFailed)
	}

	log.Debug("SAML assertion received.", "length", len(samlAssertion))

	// For Google Apps SAML, we need to extract the assertion from the Response
	// The response contains a saml2p:Response with embedded saml2:Assertion
	processedAssertion := p.preprocessGoogleSAMLResponse(samlAssertion)

	// Log the processed assertion for debugging
	if len(processedAssertion) > 0 {
		// Decode and log the processed assertion content
		if decoded, err := base64.StdEncoding.DecodeString(processedAssertion); err == nil {
			log.Debug("Processed assertion XML content", "xml", string(decoded)[:min(2000, len(decoded))])
		}
	}

	// Try different formats for saml2aws.ExtractAwsRoles()
	// First try the processed assertion
	roleStrings, err := saml2aws.ExtractAwsRoles([]byte(processedAssertion))
	if err != nil {
		log.Debug("Failed with processed assertion, trying original", "error", err)

		// Try with original assertion
		roleStrings, err = saml2aws.ExtractAwsRoles([]byte(samlAssertion))
		if err != nil {
			log.Debug("Failed with original assertion, trying decoded", "error", err)

			// Try with decoded assertion (if it was Base64 encoded)
			if decoded, decodeErr := base64.StdEncoding.DecodeString(samlAssertion); decodeErr == nil {
				roleStrings, err = saml2aws.ExtractAwsRoles(decoded)
				if err != nil {
					log.Debug("Failed with decoded assertion", "error", err)
				} else {
					log.Debug("Success with decoded assertion format")
				}
			}

			// If still failing, try with the decoded processed assertion
			if err != nil && len(processedAssertion) > 0 {
				if decoded, decodeErr := base64.StdEncoding.DecodeString(processedAssertion); decodeErr == nil {
					roleStrings, err = saml2aws.ExtractAwsRoles(decoded)
					if err != nil {
						log.Debug("Failed with decoded processed assertion", "error", err)
					} else {
						log.Debug("Success with decoded processed assertion format")
					}
				}
			}

			if err != nil {
				log.Error("All SAML assertion formats failed", "error", err)
				return nil, fmt.Errorf("%w: failed to extract AWS roles from SAML assertion (tried multiple formats): %v", errUtils.ErrAuthenticationFailed, err)
			}
		} else {
			log.Debug("Success with original assertion format")
		}
	} else {
		log.Debug("Success with processed assertion format")
	}

	if len(roleStrings) == 0 {
		return nil, fmt.Errorf("%w: no AWS roles found in SAML assertion", errUtils.ErrAuthenticationFailed)
	}

	// Parse role strings into AWSRole structs
	awsRoles, err := saml2aws.ParseAWSRoles(roleStrings)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse AWS roles: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Use the first role or let user select if multiple
	selectedRole := awsRoles[0]
	if len(awsRoles) > 1 {
		log.Info("Multiple AWS roles available", "count", len(awsRoles))
		// For now, use the first role. In the future, we could add role selection logic
		log.Info("Using first available role", "role", selectedRole.RoleARN)
	}

	// Assume the role using the best assertion format.
	assertionForSTS := samlAssertion
	if len(processedAssertion) > 0 {
		assertionForSTS = processedAssertion
	}
	awsCreds, err := p.assumeRoleWithSAML(ctx, assertionForSTS, selectedRole)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with SAML: %v", errUtils.ErrAuthenticationFailed, err)
	}

	log.Info("SAML authentication successful", "provider", p.name, "role", selectedRole.RoleARN)

	return awsCreds, nil
}

// assumeRoleWithSAML assumes an AWS role using SAML assertion.
func (p *samlProvider) assumeRoleWithSAML(ctx context.Context, samlAssertion string, role *saml2aws.AWSRole) (*schema.Credentials, error) {
	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(p.region),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create AWS session: %v", errUtils.ErrAuthenticationFailed, err)
	}

	stsClient := sts.New(sess)

	// Assume role with SAML
	input := &sts.AssumeRoleWithSAMLInput{
		RoleArn:         aws.String(role.RoleARN),
		PrincipalArn:    aws.String(role.PrincipalARN),
		SAMLAssertion:   aws.String(samlAssertion),
		DurationSeconds: aws.Int64(3600), // 1 hour
	}

	result, err := stsClient.AssumeRoleWithSAML(input)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with SAML: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Convert to schema.Credentials
	creds := &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     *result.Credentials.AccessKeyId,
			SecretAccessKey: *result.Credentials.SecretAccessKey,
			SessionToken:    *result.Credentials.SessionToken,
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
func (p *samlProvider) setupBrowserAutomation() error {
	// Set environment variables for browser automation
	if p.config.DownloadBrowserDriver {
		os.Setenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD", "true")
	}

	// For Google Apps SAML, we need to use Browser provider type
	if strings.Contains(p.url, "accounts.google.com") {
		log.Debug("Detected Google Apps SAML, using Browser provider")
	}

	return nil
}

// preprocessGoogleSAMLResponse processes Google Apps SAML response to extract the assertion.
func (p *samlProvider) preprocessGoogleSAMLResponse(samlResponse string) string {
	// First, try to decode the SAML response if it's Base64 encoded
	decodedResponse := samlResponse
	if decoded, err := base64.StdEncoding.DecodeString(samlResponse); err == nil {
		decodedResponse = string(decoded)
		log.Debug("Successfully decoded Base64 SAML response", "originalLength", len(samlResponse), "decodedLength", len(decodedResponse))
	}

	// Check if this looks like a Google Apps SAML response
	if !strings.Contains(decodedResponse, "saml2p:Response") {
		log.Debug("Not a Google Apps SAML response, returning original")
		return samlResponse // Return original encoded response
	}

	// Find the assertion within the response
	assertionStart := strings.Index(decodedResponse, "<saml2:Assertion")
	if assertionStart == -1 {
		log.Debug("No saml2:Assertion found in decoded response, returning original")
		return samlResponse
	}

	assertionEnd := strings.Index(decodedResponse[assertionStart:], "</saml2:Assertion>")
	if assertionEnd == -1 {
		log.Debug("No closing saml2:Assertion tag found, returning original")
		return samlResponse
	}

	// Extract the assertion including the closing tag
	assertion := decodedResponse[assertionStart : assertionStart+assertionEnd+len("</saml2:Assertion>")]
	log.Debug("Extracted assertion from Google SAML response", "length", len(assertion))

	// Re-encode the extracted assertion as Base64 for saml2aws
	encodedAssertion := base64.StdEncoding.EncodeToString([]byte(assertion))
	log.Debug("Re-encoded assertion as Base64", "encodedLength", len(encodedAssertion))

	return encodedAssertion
}

// setupSAML2AWSConfig creates the saml2aws configuration directory and file.
func (p *samlProvider) setupSAML2AWSConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".saml2aws")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("failed to create saml2aws config directory: %w", err)
	}

	return nil
}
