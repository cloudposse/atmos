package environment

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	ini "gopkg.in/ini.v1"

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

// WriteCredentials writes AWS credentials to the provider-specific file with identity profile
func (m *awsFileManager) WriteCredentials(providerName, identityName string, creds *schema.AWSCredentials) error {
	credentialsPath := m.GetCredentialsPath(providerName)
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(credentialsPath), 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Load existing INI file or create new one
	cfg, err := ini.Load(credentialsPath)
	if err != nil {
		// File doesn't exist or is invalid, create new one
		cfg = ini.Empty()
	}

	// Get or create the profile section
	section, err := cfg.NewSection(identityName)
	if err != nil {
		return fmt.Errorf("failed to create profile section: %w", err)
	}

	// Set credentials
	section.Key("aws_access_key_id").SetValue(creds.AccessKeyID)
	section.Key("aws_secret_access_key").SetValue(creds.SecretAccessKey)
	if creds.SessionToken != "" {
		section.Key("aws_session_token").SetValue(creds.SessionToken)
	} else {
		// Remove session token if not present
		section.DeleteKey("aws_session_token")
	}

	// Save file with proper permissions
	if err := cfg.SaveTo(credentialsPath); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	// Set proper file permissions
	if err := os.Chmod(credentialsPath, 0600); err != nil {
		return fmt.Errorf("failed to set credentials file permissions: %w", err)
	}

	return nil
}

// WriteConfig writes AWS config to the provider-specific file with identity profile
func (m *awsFileManager) WriteConfig(providerName, identityName, region, outputFormat string) error {
	configPath := m.GetConfigPath(providerName)
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Load existing INI file or create new one
	cfg, err := ini.Load(configPath)
	if err != nil {
		// File doesn't exist or is invalid, create new one
		cfg = ini.Empty()
	}

	// Get or create the profile section (AWS config uses "profile name" format, except for "default")
	var profileSectionName string
	if identityName == "default" {
		profileSectionName = "default"
	} else {
		profileSectionName = fmt.Sprintf("profile %s", identityName)
	}
	section, err := cfg.NewSection(profileSectionName)
	if err != nil {
		return fmt.Errorf("failed to create profile section: %w", err)
	}

	// Debug logging for region
	log.Debug("AWS WriteConfig", "providerName", providerName, "identityName", identityName, "region", region, "outputFormat", outputFormat)
	
	// Set config values only if they are not empty
	if region != "" {
		section.Key("region").SetValue(region)
	} else {
		// Remove region key if not present
		section.DeleteKey("region")
	}
	
	// Set output format only if explicitly provided
	if outputFormat != "" {
		section.Key("output").SetValue(outputFormat)
	} else {
		// Remove output key if not present
		section.DeleteKey("output")
	}

	// Save file with proper permissions
	if err := cfg.SaveTo(configPath); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Set proper file permissions
	if err := os.Chmod(configPath, 0600); err != nil {
		return fmt.Errorf("failed to set config file permissions: %w", err)
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

// GetEnvironmentVariables returns the AWS file environment variables as EnvironmentVariable slice
func (m *awsFileManager) GetEnvironmentVariables(providerName, identityName string) []schema.EnvironmentVariable {
	credentialsPath := m.GetCredentialsPath(providerName)
	configPath := m.GetConfigPath(providerName)

	return []schema.EnvironmentVariable{
		{Key: "AWS_SHARED_CREDENTIALS_FILE", Value: credentialsPath},
		{Key: "AWS_CONFIG_FILE", Value: configPath},
		{Key: "AWS_PROFILE", Value: identityName},
	}
}

// Cleanup removes AWS files for the provider
func (m *awsFileManager) Cleanup(providerName string) error {
	providerDir := filepath.Join(m.baseDir, providerName)
	
	if err := os.RemoveAll(providerDir); err != nil {
		return fmt.Errorf("failed to cleanup AWS files: %w", err)
	}

	return nil
}

