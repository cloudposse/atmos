package aws

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"gopkg.in/ini.v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// loadAWSCredentialsFromEnvironment loads AWS credentials from files using environment variables.
// This is a shared helper for all AWS identity types to use with noop keyring.
// It temporarily sets AWS env vars, loads credentials via SDK, then restores original env.
func loadAWSCredentialsFromEnvironment(ctx context.Context, env map[string]string) (*types.AWSCredentials, error) {
	// Extract AWS environment variables.
	credsFile, hasCredsFile := env["AWS_SHARED_CREDENTIALS_FILE"]
	configFile, hasConfigFile := env["AWS_CONFIG_FILE"]
	profile, hasProfile := env["AWS_PROFILE"]
	region := env["AWS_REGION"] // Optional.

	if !hasCredsFile || !hasConfigFile || !hasProfile {
		return nil, fmt.Errorf("%w: AWS_SHARED_CREDENTIALS_FILE, AWS_CONFIG_FILE, AWS_PROFILE", errUtils.ErrAwsMissingEnvVars)
	}

	log.Debug("Loading AWS credentials from files",
		"credentials_file", credsFile,
		"config_file", configFile,
		"profile", profile,
		"region", region,
	)

	// Temporarily set environment variables for AWS SDK.
	originalEnv := make(map[string]string)
	envVarsToSet := map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": credsFile,
		"AWS_CONFIG_FILE":             configFile,
		"AWS_PROFILE":                 profile,
	}
	if region != "" {
		envVarsToSet["AWS_REGION"] = region
	}

	// Save original values and set new ones.
	for key, value := range envVarsToSet {
		if origValue, exists := os.LookupEnv(key); exists {
			originalEnv[key] = origValue
		}
		os.Setenv(key, value)
	}

	// Restore original environment when done.
	defer func() {
		for key := range envVarsToSet {
			if origValue, hadOriginal := originalEnv[key]; hadOriginal {
				os.Setenv(key, origValue)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Load AWS config using SDK (which will read from the files via env vars).
	cfg, err := awsConfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config from files: %w", err)
	}

	// Retrieve credentials from the config.
	awsCreds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS credentials from files: %w", err)
	}

	// Build our credential struct.
	creds := &types.AWSCredentials{
		AccessKeyID:     awsCreds.AccessKeyID,
		SecretAccessKey: awsCreds.SecretAccessKey,
		SessionToken:    awsCreds.SessionToken,
		Region:          cfg.Region,
	}

	// Get expiration from AWS SDK if available.
	if !awsCreds.Expires.IsZero() {
		creds.Expiration = awsCreds.Expires.Format(time.RFC3339)
	} else if creds.SessionToken != "" {
		// SDK doesn't have expiration, but we have a session token.
		// Try to read expiration from metadata comment in credentials file.
		if expiration := readExpirationFromMetadata(credsFile, profile); expiration != "" {
			creds.Expiration = expiration
			log.Debug("Loaded expiration from credentials file metadata",
				"profile", profile,
				"expiration", expiration,
			)
		}
	}

	log.Debug("Successfully loaded AWS credentials from files",
		"profile", profile,
		"region", creds.Region,
		"has_session_token", creds.SessionToken != "",
		"has_expiration", creds.Expiration != "",
	)

	return creds, nil
}

// readExpirationFromMetadata reads expiration from atmos comment in credentials file.
// Format: ; atmos: expiration=2025-10-24T23:42:49Z
// Returns empty string if not found or invalid.
func readExpirationFromMetadata(credentialsPath, profile string) string {
	// Load the credentials file with comment preservation enabled.
	cfg, err := ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: false,
	}, credentialsPath)
	if err != nil {
		log.Debug("Failed to load credentials file for metadata",
			"path", credentialsPath,
			"error", err,
		)
		return ""
	}

	// Get the profile section.
	section, err := cfg.GetSection(profile)
	if err != nil {
		log.Debug("Profile section not found in credentials file",
			"profile", profile,
		)
		return ""
	}

	// Check if section has a comment with metadata.
	if section.Comment == "" {
		return ""
	}

	// Parse comment: "; atmos: expiration=2025-10-24T23:42:49Z"
	// The ini library includes the comment prefix (;) when reading.
	comment := strings.TrimSpace(section.Comment)
	comment = strings.TrimPrefix(comment, ";")
	comment = strings.TrimPrefix(comment, "#")
	comment = strings.TrimSpace(comment)

	if !strings.HasPrefix(comment, "atmos:") {
		return ""
	}

	// Extract key=value pairs.
	metadata := strings.TrimPrefix(comment, "atmos:")
	metadata = strings.TrimSpace(metadata)

	// Simple parsing: split by spaces and look for expiration=value.
	parts := strings.Fields(metadata)
	for _, part := range parts {
		if strings.HasPrefix(part, "expiration=") {
			expiration := strings.TrimPrefix(part, "expiration=")
			// Validate it's a valid RFC3339 timestamp.
			if _, err := time.Parse(time.RFC3339, expiration); err == nil {
				return expiration
			}
			log.Debug("Invalid expiration format in metadata",
				"expiration", expiration,
				"error", err,
			)
		}
	}

	return ""
}
