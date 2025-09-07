package aws

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/internal/auth/utils"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SetupFiles sets up AWS credentials and config files for the given identity
func SetupFiles(providerName, identityName string, creds *schema.Credentials) error {
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

	return nil
}

// SetEnvironmentVariables sets the AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE environment variables.
func SetEnvironmentVariables(stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string) error {
	m := NewAWSFileManager()
	credentialsPath := m.GetCredentialsPath(providerName)
	configPath := m.GetConfigPath(providerName)

	utils.SetEnvironmentVariable(stackInfo, "AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
	utils.SetEnvironmentVariable(stackInfo, "AWS_CONFIG_FILE", configPath)
	utils.SetEnvironmentVariable(stackInfo, "AWS_PROFILE", identityName)
	return nil
}
