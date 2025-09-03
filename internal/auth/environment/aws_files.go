package environment

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// awsFileManager implements the AWSFileManager interface
type awsFileManager struct {
	baseDir string
}

// NewAWSFileManager creates a new AWS file manager instance
func NewAWSFileManager() types.AWSFileManager {
	homeDir, _ := os.UserHomeDir()
	return &awsFileManager{
		baseDir: filepath.Join(homeDir, ".aws", "atmos"),
	}
}

// WriteCredentials writes AWS credentials to the provider-specific file
func (m *awsFileManager) WriteCredentials(providerName string, creds *schema.AWSCredentials) error {
	credentialsPath := m.GetCredentialsPath(providerName)
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(credentialsPath), 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Create credentials file content
	content := fmt.Sprintf(`[default]
aws_access_key_id = %s
aws_secret_access_key = %s
`, creds.AccessKeyID, creds.SecretAccessKey)

	if creds.SessionToken != "" {
		content += fmt.Sprintf("aws_session_token = %s\n", creds.SessionToken)
	}

	// Write file
	if err := os.WriteFile(credentialsPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// WriteConfig writes AWS config to the provider-specific file
func (m *awsFileManager) WriteConfig(providerName string, region string) error {
	configPath := m.GetConfigPath(providerName)
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create config file content
	content := fmt.Sprintf(`[default]
region = %s
output = json
`, region)

	// Write file
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetCredentialsPath returns the path to the credentials file for the provider
func (m *awsFileManager) GetCredentialsPath(providerName string) string {
	return filepath.Join(m.baseDir, providerName, "credentials")
}

// GetConfigPath returns the path to the config file for the provider
func (m *awsFileManager) GetConfigPath(providerName string) string {
	return filepath.Join(m.baseDir, providerName, "config")
}

// SetEnvironmentVariables sets the AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE environment variables
func (m *awsFileManager) SetEnvironmentVariables(providerName string) error {
	credentialsPath := m.GetCredentialsPath(providerName)
	configPath := m.GetConfigPath(providerName)

	if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath); err != nil {
		return fmt.Errorf("failed to set AWS_SHARED_CREDENTIALS_FILE: %w", err)
	}

	if err := os.Setenv("AWS_CONFIG_FILE", configPath); err != nil {
		return fmt.Errorf("failed to set AWS_CONFIG_FILE: %w", err)
	}

	return nil
}

// Cleanup removes AWS files for the provider
func (m *awsFileManager) Cleanup(providerName string) error {
	providerDir := filepath.Join(m.baseDir, providerName)
	
	if err := os.RemoveAll(providerDir); err != nil {
		return fmt.Errorf("failed to cleanup AWS files: %w", err)
	}

	return nil
}
