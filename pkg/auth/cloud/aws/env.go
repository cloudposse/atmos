package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
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
		return aws.Config{}, fmt.Errorf("%w: %v", errUtils.ErrLoadAwsConfig, isolateErr)
	}

	if err != nil {
		return aws.Config{}, fmt.Errorf("%w: %v", errUtils.ErrLoadAwsConfig, err)
	}

	return cfg, nil
}
