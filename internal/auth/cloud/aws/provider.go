package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/ini.v1"
)

// AWSCloudProvider implements the CloudProvider interface for AWS
type AWSCloudProvider struct {
	homeDir string
}

// NewAWSCloudProvider creates a new AWS cloud provider instance
func NewAWSCloudProvider() types.CloudProvider {
	homeDir, _ := os.UserHomeDir()
	return &AWSCloudProvider{
		homeDir: homeDir,
	}
}

// GetName returns the cloud provider name
func (p *AWSCloudProvider) GetName() string {
	return "aws"
}

// SetupEnvironment configures AWS-specific environment variables and files
func (p *AWSCloudProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, credentials *schema.Credentials) error {
	if credentials == nil || credentials.AWS == nil {
		return fmt.Errorf("AWS credentials are required")
	}

	// Create AWS directory structure
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create AWS directory: %w", err)
	}

	// Write credentials file
	credentialsPath := filepath.Join(awsDir, "credentials")
	if err := p.writeCredentialsFile(credentialsPath, identityName, credentials.AWS); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	// Write config file
	configPath := filepath.Join(awsDir, "config")
	if err := p.writeConfigFile(configPath, identityName, credentials.AWS.Region); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetEnvironmentVariables returns AWS-specific environment variables
func (p *AWSCloudProvider) GetEnvironmentVariables(providerName, identityName string) map[string]string {
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)

	return map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": filepath.Join(awsDir, "credentials"),
		"AWS_CONFIG_FILE":             filepath.Join(awsDir, "config"),
		"AWS_PROFILE":                 identityName,
	}
}

// CleanupEnvironment removes AWS temporary files and resources
func (p *AWSCloudProvider) CleanupEnvironment(ctx context.Context, providerName, identityName string) error {
	// For now, we keep the files for caching purposes
	// In the future, we might implement cleanup based on expiration
	return nil
}

// ValidateCredentials validates AWS credentials
func (p *AWSCloudProvider) ValidateCredentials(ctx context.Context, credentials *schema.Credentials) error {
	if credentials == nil {
		return fmt.Errorf("credentials cannot be nil")
	}

	if credentials.AWS == nil {
		return fmt.Errorf("AWS credentials are required")
	}

	if credentials.AWS.AccessKeyID == "" {
		return fmt.Errorf("AWS access key ID is required")
	}

	if credentials.AWS.SecretAccessKey == "" {
		return fmt.Errorf("AWS secret access key is required")
	}

	return nil
}

// GetCredentialPaths returns paths where AWS credentials are stored
func (p *AWSCloudProvider) GetCredentialPaths(providerName, identityName string) (map[string]string, error) {
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)

	return map[string]string{
		"credentials": filepath.Join(awsDir, "credentials"),
		"config":      filepath.Join(awsDir, "config"),
		"directory":   awsDir,
	}, nil
}

// writeCredentialsFile writes AWS credentials to the specified file
func (p *AWSCloudProvider) writeCredentialsFile(path, identityName string, creds *schema.AWSCredentials) error {
	content := fmt.Sprintf(`[%s]
aws_access_key_id = %s
aws_secret_access_key = %s
`, identityName, creds.AccessKeyID, creds.SecretAccessKey)

	if creds.SessionToken != "" {
		content += fmt.Sprintf("aws_session_token = %s\n", creds.SessionToken)
	}

	return p.updateProfileInFile(path, identityName, content)
}

// writeConfigFile writes AWS config to the specified file
func (p *AWSCloudProvider) writeConfigFile(path, identityName, region string) error {
	if region == "" {
		region = "us-east-1" // Default region
	}

	content := fmt.Sprintf(`[profile %s]
region = %s
`, identityName, region)

	return p.updateProfileInFile(path, fmt.Sprintf("profile %s", identityName), content)
}

// Cleanup removes temporary files and resources created by this provider
func (p *AWSCloudProvider) Cleanup(ctx context.Context, providerName, identityName string) error {
	// Currently no cleanup needed - files are persistent for caching
	// In the future, we could implement cleanup of expired credential files
	return nil
}

// GetCredentialFilePaths returns the paths to credential files managed by this provider
func (p *AWSCloudProvider) GetCredentialFilePaths(providerName string) map[string]string {
	awsDir := filepath.Join(p.homeDir, ".aws", "atmos", providerName)
	return map[string]string{
		"credentials": filepath.Join(awsDir, "credentials"),
		"config":      filepath.Join(awsDir, "config"),
	}
}

// updateProfileInFile updates or adds a profile section in an AWS config file using INI format
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
		return fmt.Errorf("invalid content format")
	}

	// Skip the section header line (e.g., "[profile name]" or "[name]")
	sectionName := strings.Trim(lines[0], "[]")

	// Get or create the section
	section, err := cfg.NewSection(sectionName)
	if err != nil {
		return fmt.Errorf("failed to create section %s: %w", sectionName, err)
	}

	// Add key-value pairs from remaining lines
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

	// Save the file with proper permissions
	return cfg.SaveTo(path)
}
