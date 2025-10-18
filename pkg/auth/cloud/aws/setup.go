package aws

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/utils"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SetupFiles sets up AWS credentials and config files for the given identity.
// basePath specifies the base directory for AWS files (from provider's files.base_path).
// If empty, uses the default ~/.aws/atmos path.
func SetupFiles(providerName, identityName string, creds types.ICredentials, basePath string) error {
	awsCreds, ok := creds.(*types.AWSCredentials)
	if !ok {
		return nil // No AWS credentials to setup
	}

	// Create AWS file manager with configured or default path.
	fileManager, err := NewAWSFileManager(basePath)
	if err != nil {
		return errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}

	// Write credentials file.
	if err := fileManager.WriteCredentials(providerName, identityName, awsCreds); err != nil {
		return fmt.Errorf("%w: failed to write AWS credentials: %v", errUtils.ErrAwsAuth, err)
	}

	// Write config file with region.
	region := awsCreds.Region
	if region == "" {
		region = "us-east-1" // Default region
	}

	if err := fileManager.WriteConfig(providerName, identityName, region, ""); err != nil {
		return fmt.Errorf("%w: failed to write AWS config: %v", errUtils.ErrAwsAuth, err)
	}

	return nil
}

// SetEnvironmentVariables sets the AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE environment variables.
// basePath specifies the base directory for AWS files (from provider's files.base_path).
// If empty, uses the default ~/.aws/atmos path.
func SetEnvironmentVariables(stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, basePath string) error {
	m, err := NewAWSFileManager(basePath)
	if err != nil {
		return errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}
	credentialsPath := m.GetCredentialsPath(providerName)
	configPath := m.GetConfigPath(providerName)

	utils.SetEnvironmentVariable(stackInfo, "AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
	utils.SetEnvironmentVariable(stackInfo, "AWS_CONFIG_FILE", configPath)
	utils.SetEnvironmentVariable(stackInfo, "AWS_PROFILE", identityName)
	return nil
}
