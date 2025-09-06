package aws

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/schema"
)

// SetupFiles sets up AWS credentials and config files for the given identity
func SetupFiles(ctx context.Context, providerName, identityName string, creds *schema.Credentials) error {
	if creds.AWS == nil {
		return nil // No AWS credentials to setup
	}

	// Create AWS file manager
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".aws", "atmos")
	
	fileManager := &AWSFileManager{
		baseDir: baseDir,
	}

	// Write credentials file
	if err := fileManager.WriteCredentials(providerName, identityName, creds.AWS); err != nil {
		return err
	}

	// Write config file with region
	region := creds.AWS.Region
	if region == "" {
		region = "us-east-1" // Default region
	}
	
	if err := fileManager.WriteConfig(providerName, identityName, region, ""); err != nil {
		return err
	}

	// Set environment variables
	return fileManager.SetEnvironmentVariables(providerName)
}
