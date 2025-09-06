package aws

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SetupAWSFiles is a shared function that AWS identities can use to set up AWS credential files
func SetupAWSFiles(ctx context.Context, awsFileManager types.AWSFileManager, providerName, identityName string, creds *schema.Credentials, config *schema.AuthConfig) error {
	if creds.AWS == nil {
		return fmt.Errorf("no AWS credentials found")
	}

	// Write credentials file to provider directory with identity profile
	if err := awsFileManager.WriteCredentials(providerName, identityName, creds.AWS); err != nil {
		return fmt.Errorf("failed to write AWS credentials: %w", err)
	}

	// Write config file to provider directory with identity profile
	region := creds.AWS.Region
	if region == "" {
		// For AWS user identities, get region from identity credentials config
		if providerName == "aws-user" {
			if identity, exists := config.Identities[identityName]; exists {
				if r, ok := identity.Credentials["region"].(string); ok && r != "" {
					region = r
				}
			}
		}
		// Fallback to provider config
		if region == "" {
			if provider, exists := config.Providers[providerName]; exists {
				region = provider.Region
			}
		}
	}
	if err := awsFileManager.WriteConfig(providerName, identityName, region, ""); err != nil {
		return fmt.Errorf("failed to write AWS config: %w", err)
	}

	// Set environment variables using provider name for file paths
	if err := awsFileManager.SetEnvironmentVariables(providerName); err != nil {
		return fmt.Errorf("failed to set AWS environment variables: %w", err)
	}

	return nil
}
