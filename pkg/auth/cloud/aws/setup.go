package aws

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/utils"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SetupFiles sets up AWS credentials and config files for the given identity.
// BasePath specifies the base directory for AWS files (from provider's files.base_path).
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
		return fmt.Errorf("%s: failed to write AWS credentials: %w", errUtils.ErrAwsAuth.Error(), err)
	}

	// Write config file with region.
	region := awsCreds.Region
	if region == "" {
		region = "us-east-1" // Default region
	}

	if err := fileManager.WriteConfig(providerName, identityName, region, ""); err != nil {
		return fmt.Errorf("%s: failed to write AWS config: %w", errUtils.ErrAwsAuth.Error(), err)
	}

	return nil
}

// SetAuthContext populates the AWS auth context with Atmos-managed credential paths.
// This enables in-process AWS SDK calls to use Atmos-managed credentials.
//
// Parameters:
//   - authContext: Runtime auth context to populate (passed by caller, used by in-process SDK calls)
//   - stackInfo: Stack configuration (may contain component-level auth overrides from inheritance)
//   - providerName: Auth provider name (e.g., "aws-sso")
//   - identityName: Identity name (e.g., "dev-admin")
//   - creds: Authenticated credentials (contains region and other AWS-specific info)
func SetAuthContext(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error {
	if authContext == nil {
		return nil // No auth context to populate.
	}

	awsCreds, ok := creds.(*types.AWSCredentials)
	if !ok {
		return nil // No AWS credentials to setup.
	}

	m, err := NewAWSFileManager("")
	if err != nil {
		return errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}

	credentialsPath := m.GetCredentialsPath(providerName)
	configPath := m.GetConfigPath(providerName)

	// Start with region from credentials.
	region := awsCreds.Region

	// Check for component-level region override from merged auth config.
	// Stack inheritance allows components to override identity configuration.
	if regionOverride := getComponentRegionOverride(stackInfo, identityName); regionOverride != "" {
		region = regionOverride
		log.Debug("Using component-level region override",
			"identity", identityName,
			"region", region,
		)
	}

	// Populate AWS auth context as the single source of truth.
	authContext.AWS = &schema.AWSAuthContext{
		CredentialsFile: credentialsPath,
		ConfigFile:      configPath,
		Profile:         identityName,
		Region:          region,
	}

	log.Debug("Set AWS auth context",
		"profile", identityName,
		"credentials", credentialsPath,
		"config", configPath,
		"region", region,
	)

	return nil
}

// getComponentRegionOverride extracts region override from component auth config.
func getComponentRegionOverride(stackInfo *schema.ConfigAndStacksInfo, identityName string) string {
	if stackInfo == nil || stackInfo.ComponentAuthSection == nil {
		return ""
	}

	identities, ok := stackInfo.ComponentAuthSection["identities"].(map[string]any)
	if !ok {
		return ""
	}

	identityCfg, ok := identities[identityName].(map[string]any)
	if !ok {
		return ""
	}

	regionOverride, ok := identityCfg["region"].(string)
	if !ok {
		return ""
	}

	return regionOverride
}

// SetEnvironmentVariables derives AWS environment variables from AuthContext.
// This populates ComponentEnvSection/ComponentEnvList for spawned processes.
// The auth context is the single source of truth; this function derives from it.
//
// Parameters:
//   - authContext: Runtime auth context containing AWS credentials
//   - stackInfo: Stack configuration to populate with environment variables
func SetEnvironmentVariables(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo) error {
	if authContext == nil || authContext.AWS == nil {
		return nil // No auth context to derive from.
	}

	if stackInfo == nil {
		return nil // No stack info to populate.
	}

	awsAuth := authContext.AWS

	// Derive environment variables from auth context.
	utils.SetEnvironmentVariable(stackInfo, "AWS_SHARED_CREDENTIALS_FILE", awsAuth.CredentialsFile)
	utils.SetEnvironmentVariable(stackInfo, "AWS_CONFIG_FILE", awsAuth.ConfigFile)
	utils.SetEnvironmentVariable(stackInfo, "AWS_PROFILE", awsAuth.Profile)

	if awsAuth.Region != "" {
		utils.SetEnvironmentVariable(stackInfo, "AWS_REGION", awsAuth.Region)
	}

	return nil
}
