package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/viper"
	"github.com/versent/saml2aws/v2"
	"github.com/versent/saml2aws/v2/pkg/cfg"
	"github.com/versent/saml2aws/v2/pkg/creds"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	samlTimeoutSeconds            = 30
	samlDefaultSessionSec         = 3600
	logFieldRole                  = "role"
	logFieldDriver                = "driver"
	minSTSSeconds                 = 900
	maxSTSSeconds                 = 43200
	playwrightCacheDir            = "ms-playwright"
	playwrightCacheDirPermissions = 0o755
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
	samlDriver := p.getDriver()
	downloadBrowser := p.shouldDownloadBrowser()

	log.Info("Starting SAML authentication", "provider", p.name, "url", p.url, logFieldDriver, samlDriver)
	log.Debug("SAML configuration",
		logFieldDriver, samlDriver,
		"download_browser", downloadBrowser,
		"requires_drivers", samlDriver == "Browser")

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
		return nil, fmt.Errorf("%w: failed to create SAML client: %w", errUtils.ErrInvalidProviderConfig, err)
	}

	// Authenticate and get assertion.
	assertionB64, err := p.authenticateAndGetAssertion(samlClient, loginDetails)
	if err != nil {
		return nil, err
	}

	decodedXML, err := base64.StdEncoding.DecodeString(assertionB64)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode assertion: %w", errUtils.ErrAwsSAMLDecodeFailed, err)
	}
	rolesStr, err := saml2aws.ExtractAwsRoles(decodedXML)
	if err != nil {
		return nil, fmt.Errorf("%w: extract AWS roles: %w", errUtils.ErrAwsSAMLDecodeFailed, err)
	}
	roles, err := saml2aws.ParseAWSRoles(rolesStr)
	if err != nil {
		return nil, fmt.Errorf("%w: parse AWS roles: %w", errUtils.ErrAwsSAMLDecodeFailed, err)
	}

	selectedRole := p.selectRole(roles)
	if selectedRole == nil {
		return nil, fmt.Errorf("%w: no role selected", errUtils.ErrAuthenticationFailed)
	}

	// Assume role.
	awsCreds, err := p.assumeRoleWithSAML(ctx, assertionB64, selectedRole)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role with SAML: %w", errUtils.ErrAuthenticationFailed, err)
	}

	log.Info("SAML authentication successful", "provider", p.name, logFieldRole, selectedRole.RoleARN)

	return awsCreds, nil
}

// createSAMLConfig creates the saml2aws configuration.
func (p *samlProvider) createSAMLConfig() *cfg.IDPAccount {
	return &cfg.IDPAccount{
		URL:                  p.url,
		Username:             p.config.Username,
		Provider:             p.getDriver(),
		MFA:                  "Auto",
		SkipVerify:           false,
		Timeout:              samlTimeoutSeconds, // 30 second timeout.
		AmazonWebservicesURN: "urn:amazon:webservices",
		SessionDuration:      samlDefaultSessionSec, // 1 hour default.
		Profile:              p.name,
		Region:               p.region,
		DownloadBrowser:      p.shouldDownloadBrowser(), // Intelligently enable based on driver availability.
		Headless:             false,                     // Force non-headless for interactive auth.
	}
}

// createLoginDetails creates the login details for saml2aws.
func (p *samlProvider) createLoginDetails() *creds.LoginDetails {
	loginDetails := &creds.LoginDetails{
		URL:             p.url,
		Username:        p.config.Username,
		DownloadBrowser: p.shouldDownloadBrowser(), // Enable automatic driver installation.
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
		return "", fmt.Errorf("%w: SAML authentication failed: %w", errUtils.ErrAuthenticationFailed, err)
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
	// Use LoadIsolatedAWSConfig to avoid conflicts with external AWS env vars.
	return p.assumeRoleWithSAMLWithDeps(ctx, samlAssertion, role, awsCloud.LoadIsolatedAWSConfig, func(cfg aws.Config) assumeRoleWithSAMLClient {
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
		return nil, fmt.Errorf("%w: failed to create AWS config: %w", errUtils.ErrAuthenticationFailed, err)
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
		return nil, fmt.Errorf("%w: failed to assume role with SAML: %w", errUtils.ErrAuthenticationFailed, err)
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

// getDriver returns the SAML driver type based on configuration or URL detection.
// Priority: Browser (if drivers available) > provider-specific fallbacks (GoogleApps/Okta/ADFS).
func (p *samlProvider) getDriver() string {
	// If user explicitly set driver, always respect their choice.
	if p.config.Driver != "" {
		log.Debug("Using explicitly configured SAML driver", logFieldDriver, p.config.Driver)
		return p.config.Driver
	}

	// Backward compatibility: check deprecated provider_type field.
	if p.config.ProviderType != "" {
		log.Warn("The 'provider_type' field is deprecated. Please use 'driver' instead", "current_value", p.config.ProviderType)
		return p.config.ProviderType
	}

	// Check if Playwright drivers are available or can be auto-downloaded.
	if p.hasPlaywrightDriversOrCanDownload() {
		log.Debug("Playwright drivers available, using Browser provider for best compatibility")
		return "Browser"
	}

	// Fallback to provider-specific types (use API/HTTP calls without browser automation).
	// These are less reliable but don't require browser drivers.
	if strings.Contains(p.url, "accounts.google.com") {
		log.Debug("Falling back to GoogleApps provider (no drivers available)")
		return "GoogleApps"
	}
	if strings.Contains(p.url, "okta.com") {
		log.Debug("Falling back to Okta provider (no drivers available)")
		return "Okta"
	}
	if strings.Contains(p.url, "adfs") {
		log.Debug("Falling back to ADFS provider (no drivers available)")
		return "ADFS"
	}

	// If no drivers and no known provider type, try Browser anyway with auto-download.
	log.Debug("Unknown provider, defaulting to Browser with auto-download")
	return "Browser"
}

// Validate validates the provider configuration.
func (p *samlProvider) Validate() error {
	defer perf.Track(nil, "aws.samlProvider.Validate")()

	if p.url == "" {
		return fmt.Errorf("%w: URL is required for SAML provider", errUtils.ErrInvalidProviderConfig)
	}

	if p.region == "" {
		return fmt.Errorf("%w: region is required for SAML provider", errUtils.ErrInvalidProviderConfig)
	}

	// Validate URL format strictly.
	u, err := url.ParseRequestURI(p.url)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%w: invalid URL format: %w", errUtils.ErrInvalidProviderConfig, err)
	}

	// Validate spec.files.base_path if provided.
	if err := awsCloud.ValidateFilesBasePath(p.config); err != nil {
		return err
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

// PrepareEnvironment prepares environment variables for external processes.
// For SAML providers, this method is typically not called directly since SAML providers
// authenticate to get identity credentials, which then have their own PrepareEnvironment.
// However, we implement it for interface compliance.
func (p *samlProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "aws.samlProvider.PrepareEnvironment")()

	// SAML provider doesn't write credential files itself - that's done by identities.
	// Just return the environment unchanged.
	return environ, nil
}

// playwrightDriversInstalled checks if valid Playwright drivers are installed in standard locations.
// Returns true if drivers are found, false if not found or home directory cannot be determined.
func (p *samlProvider) playwrightDriversInstalled() bool {
	var playwrightPaths []string

	// Check Atmos XDG cache directory first (allows users to consolidate all Atmos data).
	if xdgCacheDir, err := xdg.GetXDGCacheDir(playwrightCacheDir, playwrightCacheDirPermissions); err == nil {
		playwrightPaths = append(playwrightPaths, xdgCacheDir)
	}

	// Check playwright-go's hardcoded cache locations.
	// Note: playwright-go does NOT respect XDG_CACHE_HOME, it uses its own hardcoded paths.
	homeDir, err := os.UserHomeDir()
	if err == nil {
		playwrightPaths = append(playwrightPaths,
			filepath.Join(homeDir, ".cache", playwrightCacheDir),            // Linux (playwright-go default).
			filepath.Join(homeDir, "Library", "Caches", playwrightCacheDir), // macOS (playwright-go default).
			filepath.Join(homeDir, "AppData", "Local", playwrightCacheDir),  // Windows (playwright-go default).
		)
	} else {
		log.Debug("Cannot determine home directory for driver detection", "error", err)
	}

	// On Windows, also check LOCALAPPDATA if it's set (for test environments or custom configs).
	// playwright-go uses os.UserCacheDir() which returns LOCALAPPDATA directly on Windows.
	if runtime.GOOS == "windows" {
		v := viper.New()
		if err := v.BindEnv("LOCALAPPDATA"); err == nil {
			if localAppData := v.GetString("LOCALAPPDATA"); localAppData != "" {
				playwrightPaths = append(playwrightPaths, filepath.Join(localAppData, playwrightCacheDir))
			}
		}
	}

	for _, path := range playwrightPaths {
		if p.hasValidPlaywrightDrivers(path) {
			log.Debug("Found valid Playwright drivers", "path", path)
			return true
		}
	}

	log.Debug("No valid Playwright drivers found", "checked_paths", playwrightPaths)
	return false
}

// hasPlaywrightDriversOrCanDownload checks if Playwright drivers are available or can be downloaded.
// Returns true if drivers exist or auto-download is enabled.
func (p *samlProvider) hasPlaywrightDriversOrCanDownload() bool {
	// If user explicitly enabled download, drivers will be available.
	if p.config.DownloadBrowserDriver {
		log.Debug("Browser driver download explicitly enabled, drivers will be available")
		return true
	}

	return p.playwrightDriversInstalled()
}

// hasValidPlaywrightDrivers checks if a path contains actual browser binaries, not just empty directories.
func (p *samlProvider) hasValidPlaywrightDrivers(path string) bool {
	// Check if directory exists.
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if !info.IsDir() {
		return false
	}

	// Read directory contents to check if it has subdirectories with browsers.
	entries, err := os.ReadDir(path)
	if err != nil {
		log.Debug("Cannot read Playwright directory", "path", path, "error", err)
		return false
	}

	// Check if there are any version subdirectories with content.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		versionPath := filepath.Join(path, entry.Name())
		versionEntries, err := os.ReadDir(versionPath)
		if err != nil {
			continue
		}

		// If version directory has any files/subdirectories, consider it valid.
		if len(versionEntries) > 0 {
			log.Debug("Found browser binaries in version directory", "version", entry.Name(), "files", len(versionEntries))
			return true
		}
	}

	log.Debug("Playwright directory exists but contains no browser binaries", "path", path)
	return false
}

// shouldDownloadBrowser determines if browser drivers should be auto-downloaded.
// It checks if the user explicitly configured download_browser_driver, otherwise
// it intelligently enables auto-download for "Browser" driver if drivers aren't found.
func (p *samlProvider) shouldDownloadBrowser() bool {
	// If user explicitly set download_browser_driver, respect their choice.
	if p.config.DownloadBrowserDriver {
		log.Debug("Browser driver download explicitly enabled in config")
		return true
	}

	samlDriver := p.getDriver()

	// Only auto-download for "Browser" driver (others like GoogleApps, Okta don't need drivers).
	if samlDriver != "Browser" {
		log.Debug("SAML driver does not require browser drivers", logFieldDriver, samlDriver)
		return false
	}

	// Check if Playwright drivers are already installed.
	if p.playwrightDriversInstalled() {
		log.Debug("Found valid Playwright drivers, auto-download disabled")
		return false
	}

	// No valid drivers found, enable auto-download.
	log.Debug("No valid Playwright drivers found, enabling auto-download for Browser driver")
	return true
}

// setupBrowserAutomation sets up browser automation for SAML authentication.
func (p *samlProvider) setupBrowserAutomation() {
	// Set environment variables for browser automation.
	if p.shouldDownloadBrowser() {
		os.Setenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD", "true")
		log.Debug("Browser driver auto-download enabled", logFieldDriver, p.getDriver())
	}
}

// Logout removes provider-specific credential storage.
func (p *samlProvider) Logout(ctx context.Context) error {
	defer perf.Track(nil, "aws.samlProvider.Logout")()

	// Get base_path from provider spec if configured.
	basePath := awsCloud.GetFilesBasePath(p.config)

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return errors.Join(errUtils.ErrProviderLogout, errUtils.ErrLogoutFailed, err)
	}

	if err := fileManager.Cleanup(p.name); err != nil {
		log.Debug("Failed to cleanup AWS files for SAML provider", "provider", p.name, "error", err)
		return errors.Join(errUtils.ErrProviderLogout, errUtils.ErrLogoutFailed, err)
	}

	log.Debug("Cleaned up AWS files for SAML provider", "provider", p.name)
	return nil
}

// GetFilesDisplayPath returns the display path for AWS credential files.
func (p *samlProvider) GetFilesDisplayPath() string {
	defer perf.Track(nil, "aws.samlProvider.GetFilesDisplayPath")()

	basePath := awsCloud.GetFilesBasePath(p.config)

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		return "~/.aws/atmos"
	}

	return fileManager.GetDisplayPath()
}
