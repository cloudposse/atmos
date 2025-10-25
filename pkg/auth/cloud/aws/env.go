package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// problematicAWSEnvVars lists environment variables that should be ignored by Atmos auth
// to avoid authentication conflicts when using AWS SDK.
//
// These variables can interfere with Atmos's AWS authentication flow, particularly
// when using AWS IAM Identity Center (SSO) and permission sets. By clearing these
// variables before loading AWS config, we ensure Atmos uses only its managed
// credentials and configuration files.
//
// Reference: https://linear.app/cloudposse/issue/DEV-3706
var problematicAWSEnvVars = []string{
	// Authentication credentials.
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",

	// Profile configuration.
	"AWS_PROFILE",

	// File paths - these should only be set by Atmos, not inherited from the environment.
	"AWS_CONFIG_FILE",
	"AWS_SHARED_CREDENTIALS_FILE",

	// Note: AWS_REGION is intentionally NOT in this list as it's safe to inherit.
}

// WithIsolatedAWSEnv temporarily clears problematic AWS environment variables,
// executes the provided function, then restores the original values.
//
// This is used to prevent external AWS environment variables from interfering
// with Atmos's authentication flow. The AWS SDK reads these environment variables
// automatically, which can cause conflicts with Atmos-managed credentials.
//
// Usage:
//
//	err := WithIsolatedAWSEnv(func() error {
//	    cfg, err := config.LoadDefaultConfig(ctx)
//	    return err
//	})
func WithIsolatedAWSEnv(fn func() error) error {
	// Save original values and track which variables are being ignored.
	originalValues := make(map[string]string)
	ignoredVars := make([]string, 0)
	for _, key := range problematicAWSEnvVars {
		if value, exists := os.LookupEnv(key); exists {
			originalValues[key] = value
			ignoredVars = append(ignoredVars, key)
		}
	}

	// Log which environment variables are being ignored (if any).
	if len(ignoredVars) > 0 {
		log.Debug("Ignoring external AWS environment variables during authentication", "variables", ignoredVars)
	}

	// Clear problematic variables.
	for _, key := range problematicAWSEnvVars {
		os.Unsetenv(key)
	}

	// Execute the function.
	err := fn()

	// Restore original values.
	for _, key := range problematicAWSEnvVars {
		if value, exists := originalValues[key]; exists {
			os.Setenv(key, value)
		}
	}

	return err
}

// LoadIsolatedAWSConfig loads AWS configuration with problematic environment variables
// temporarily cleared to avoid conflicts with Atmos authentication.
//
// This function wraps config.LoadDefaultConfig and ensures that external AWS
// environment variables AND shared config files don't interfere with the configuration loading process.
//
// The AWS SDK by default loads from ~/.aws/config and ~/.aws/credentials even when
// AWS_PROFILE is not set. We disable shared config loading to prevent profile-based
// configuration from interfering with Atmos auth.
//
// Use this for initial authentication (SSO device flow, etc.) when you want complete isolation.
// Use LoadAtmosManagedAWSConfig when you want to use Atmos-managed credential files.
func LoadIsolatedAWSConfig(ctx context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
	var cfg aws.Config
	var err error

	// Prepend config.WithSharedConfigProfile("") to disable loading from shared config files.
	// This prevents the SDK from loading user's ~/.aws/config and ~/.aws/credentials files.
	isolatedOptFns := make([]func(*config.LoadOptions) error, 0, len(optFns)+1)
	isolatedOptFns = append(isolatedOptFns, config.WithSharedConfigProfile(""))
	isolatedOptFns = append(isolatedOptFns, optFns...)

	isolateErr := WithIsolatedAWSEnv(func() error {
		cfg, err = config.LoadDefaultConfig(ctx, isolatedOptFns...)
		return err
	})

	if isolateErr != nil {
		return aws.Config{}, fmt.Errorf("%w: %w", errUtils.ErrLoadAwsConfig, isolateErr)
	}

	if err != nil {
		return aws.Config{}, fmt.Errorf("%w: %w", errUtils.ErrLoadAwsConfig, err)
	}

	return cfg, nil
}

// LoadAtmosManagedAWSConfig loads AWS configuration while clearing external AWS environment
// variables but ALLOWING Atmos-managed credential files to be loaded.
//
// This function should be used when you want to use credentials that Atmos has already
// written to ~/.aws/atmos/<provider>/ directories. Unlike LoadIsolatedAWSConfig, this
// function ALLOWS the AWS SDK to load from shared config files and respects AWS_PROFILE,
// AWS_SHARED_CREDENTIALS_FILE, and AWS_CONFIG_FILE environment variables.
//
// It only clears credentials-related variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY,
// AWS_SESSION_TOKEN) to prevent conflicts with file-based credentials.
//
// Use this when:
// - Validating Atmos-managed credentials
// - Using credentials from the credential store
// - Any operation that needs to access previously authenticated credentials
//
// Use LoadIsolatedAWSConfig for initial authentication (SSO device flow, etc.)
func LoadAtmosManagedAWSConfig(ctx context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
	defer perf.Track(nil, "pkg/auth/cloud/aws.LoadAtmosManagedAWSConfig")()

	var cfg aws.Config
	var err error

	// Only clear credential environment variables, not file paths or profile.
	// This allows SDK to load from Atmos-managed files using AWS_PROFILE.
	credentialEnvVars := []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
	}

	// Save and clear credential variables.
	originalValues := make(map[string]string)
	clearedVars := []string{}
	for _, key := range credentialEnvVars {
		if value, exists := os.LookupEnv(key); exists {
			originalValues[key] = value
			clearedVars = append(clearedVars, key)
			os.Unsetenv(key)
		}
	}

	if len(clearedVars) > 0 {
		log.Debug("Cleared credential environment variables", "variables", clearedVars)
	}

	// Load config (respects AWS_PROFILE, AWS_SHARED_CREDENTIALS_FILE, AWS_CONFIG_FILE).
	log.Debug("Loading AWS SDK config with Atmos-managed credentials")
	cfg, err = config.LoadDefaultConfig(ctx, optFns...)

	// Restore credential variables.
	for key, value := range originalValues {
		os.Setenv(key, value)
	}

	if err != nil {
		log.Debug("Failed to load AWS SDK config", "error", err)
		return aws.Config{}, fmt.Errorf("%w: %w", errUtils.ErrLoadAwsConfig, err)
	}

	log.Debug("Successfully loaded AWS SDK config", "region", cfg.Region)

	return cfg, nil
}
