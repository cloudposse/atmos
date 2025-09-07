package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/config/go-homedir"
	"gopkg.in/ini.v1"
)

const (
	DefaultAwsDirName            = ".aws"
	DefaultAwsAtmosDirName       = "atmos"
	DefaultAwsConfigDirName      = "config"
	DefaultAwsCredentialsDirName = "credentials"
)

// AWSCloudProvider implements the CloudProvider interface for AWS.
type AWSCloudProvider struct {
	homeDir string
}

// NewAWSCloudProvider creates a new AWS cloud provider instance.
func NewAWSCloudProvider() types.CloudProvider {
	homedirDir, _ := homedir.Dir()
	return &AWSCloudProvider{
		homeDir: homedirDir,
	}
}

// GetName returns the cloud provider name.
func (p *AWSCloudProvider) GetName() string {
	return "aws"
}

// SetupEnvironment configures AWS-specific environment variables and files.
func (p *AWSCloudProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, credentials types.ICredentials) error {
	awsCreds, ok := credentials.(*types.AWSCredentials)
	if !ok {
		return fmt.Errorf("%w: AWS credentials are required", errUtils.ErrAwsAuth)
	}

	// Create AWS directory structure
	awsDir := filepath.Join(p.homeDir, DefaultAwsDirName, DefaultAwsAtmosDirName, providerName)
	if err := os.MkdirAll(awsDir, PermissionRWX); err != nil {
		return fmt.Errorf("%w: failed to create AWS directory: %v", errUtils.ErrAwsAuth, err)
	}

	// Write credentials file
	credentialsPath := filepath.Join(awsDir, DefaultAwsCredentialsDirName)
	if err := p.writeCredentialsFile(credentialsPath, identityName, awsCreds); err != nil {
		return fmt.Errorf("%w: failed to write credentials file: %v", errUtils.ErrAwsAuth, err)
	}

	// Write config file
	configPath := filepath.Join(awsDir, DefaultAwsConfigDirName)
	if err := p.writeConfigFile(configPath, identityName, awsCreds.Region); err != nil {
		return fmt.Errorf("%w: failed to write config file: %v", errUtils.ErrAwsAuth, err)
	}

	return nil
}

// GetEnvironmentVariables returns AWS-specific environment variables.
func (p *AWSCloudProvider) GetEnvironmentVariables(providerName, identityName string) (map[string]string, error) {
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)

	return map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": filepath.Join(awsDir, DefaultAwsCredentialsDirName),
		"AWS_CONFIG_FILE":             filepath.Join(awsDir, DefaultAwsConfigDirName),
		"AWS_PROFILE":                 identityName,
	}, nil
}

// CleanupEnvironment removes AWS temporary files and resources.
func (p *AWSCloudProvider) CleanupEnvironment(ctx context.Context, providerName, identityName string) error {
	// For now, we keep the files for caching purposes
	// In the future, we might implement cleanup based on expiration
	return nil
}

// ValidateCredentials validates AWS credentials.
func (p *AWSCloudProvider) ValidateCredentials(ctx context.Context, credentials types.ICredentials) error {
	if credentials == nil {
		return fmt.Errorf("%w: credentials cannot be nil", errUtils.ErrAwsAuth)
	}
	if _, ok := credentials.(*types.AWSCredentials); !ok {
		return fmt.Errorf("%w: AWS credentials are required", errUtils.ErrAwsAuth)
	}

	return nil
}

// GetCredentialPaths returns paths where AWS credentials are stored.
func (p *AWSCloudProvider) GetCredentialPaths(providerName, identityName string) (map[string]string, error) {
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)

	return map[string]string{
		"credentials": filepath.Join(awsDir, "credentials"),
		"config":      filepath.Join(awsDir, "config"),
		"directory":   awsDir,
	}, nil
}

// writeCredentialsFile writes AWS credentials to the specified file.
func (p *AWSCloudProvider) writeCredentialsFile(path, identityName string, creds *types.AWSCredentials) error {
	content := fmt.Sprintf(`[%s]
aws_access_key_id = %s
aws_secret_access_key = %s
`, identityName, creds.AccessKeyID, creds.SecretAccessKey)

	if creds.SessionToken != "" {
		content += fmt.Sprintf("aws_session_token = %s\n", creds.SessionToken)
	}

	if err := p.updateProfileInFile(path, identityName, content); err != nil {
		return err
	}
	// Tighten permissions as credentials are sensitive.
	_ = os.Chmod(path, PermissionRW)
	return nil
}

// writeConfigFile writes AWS config to the specified file.
func (p *AWSCloudProvider) writeConfigFile(path, identityName, region string) error {
	if region == "" {
		region = "us-east-1" // Default region
	}

	content := fmt.Sprintf(`[profile %s]
region = %s
`, identityName, region)

	if err := p.updateProfileInFile(path, fmt.Sprintf("profile %s", identityName), content); err != nil {
		return err
	}
	_ = os.Chmod(path, PermissionRW)
	return nil
}

// Cleanup removes temporary files and resources created by this provider.
func (p *AWSCloudProvider) Cleanup(ctx context.Context, providerName, identityName string) error {
	// Currently no cleanup needed - files are persistent for caching
	// In the future, we could implement cleanup of expired credential files
	return nil
}

// GetCredentialFilePaths returns the paths to credential files managed by this provider.
func (p *AWSCloudProvider) GetCredentialFilePaths(providerName string) map[string]string {
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)
	return map[string]string{
		"credentials": filepath.Join(awsDir, DefaultAwsCredentialsDirName),
		"config":      filepath.Join(awsDir, DefaultAwsConfigDirName),
	}
}

// updateProfileInFile updates or adds a profile section in an AWS config file using INI format.
func (p *AWSCloudProvider) updateProfileInFile(path, profileName, content string) error {
	// Load existing INI file or create new one
	cfg, err := ini.Load(path)
	if err != nil {
		// If file doesn't exist, create empty INI
		cfg = ini.Empty()
	}

	// Parse the content to extract key-value pairs
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) == 0 {
		return fmt.Errorf("%w: invalid content format", errUtils.ErrAwsAuth)
	}

	// Use the given profileName for the section and get or create it.
	section, err := cfg.GetSection(profileName)
	if err != nil {
		section, err = cfg.NewSection(profileName)
		if err != nil {
			return fmt.Errorf("%w: failed to create section %s: %v", errUtils.ErrAwsAuth, profileName, err)
		}
	}

	// Add key-value pairs from remaining lines (skip header).
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		section.Key(key).SetValue(value)
	}

	// Save the file.
	return cfg.SaveTo(path)
}
