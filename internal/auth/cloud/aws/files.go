package aws

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/charmbracelet/log"
	ini "gopkg.in/ini.v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/config/go-homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	PermissionRWX = 0o700
	PermissionRW  = 0o600
)

// AWSFileManager provides helpers to manage AWS credentials/config files.
type AWSFileManager struct {
	baseDir string
}

// NewAWSFileManager creates a new AWS file manager instance.
func NewAWSFileManager() *AWSFileManager {
	homeDir, _ := homedir.Dir()
	return &AWSFileManager{
		baseDir: filepath.Join(homeDir, ".aws", "atmos"),
	}
}

// WriteCredentials writes AWS credentials to the provider-specific file with identity profile.
func (m *AWSFileManager) WriteCredentials(providerName, identityName string, creds *types.AWSCredentials) error {
	credentialsPath := m.GetCredentialsPath(providerName)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(credentialsPath), PermissionRWX); err != nil {
		return fmt.Errorf("%w: failed to create credentials directory: %v", errUtils.ErrAwsAuth, err)
	}

	// Load existing INI file or create new one
	cfg, err := ini.Load(credentialsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: failed to load credentials file: %w", errUtils.ErrAwsAuth, err)
		}
		cfg = ini.Empty()
	}

	// Get or create the profile section
	section, err := cfg.NewSection(identityName)
	if err != nil {
		return fmt.Errorf("%w: failed to create profile section: %v", errUtils.ErrAwsAuth, err)
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
		return fmt.Errorf("%w: failed to write credentials file: %v", errUtils.ErrAwsAuth, err)
	}

	// Set proper file permissions
	if err := os.Chmod(credentialsPath, PermissionRWX); err != nil {
		return fmt.Errorf("%w: failed to set credentials file permissions: %v", errUtils.ErrAwsAuth, err)
	}

	return nil
}

// WriteConfig writes AWS config to the provider-specific file with identity profile.
func (m *AWSFileManager) WriteConfig(providerName, identityName, region, outputFormat string) error {
	configPath := m.GetConfigPath(providerName)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), PermissionRWX); err != nil {
		return fmt.Errorf("%w: failed to create config directory: %v", errUtils.ErrAwsAuth, err)
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

	section := cfg.Section(profileSectionName)
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
		return fmt.Errorf("%w: failed to write config file: %v", errUtils.ErrAwsAuth, err)
	}

	// Set proper file permissions
	if err := os.Chmod(configPath, PermissionRW); err != nil {
		return fmt.Errorf("%w: failed to set config file permissions: %v", errUtils.ErrAwsAuth, err)
	}

	return nil
}

// GetCredentialsPath returns the path to the credentials file for the provider.
func (m *AWSFileManager) GetCredentialsPath(providerName string) string {
	return filepath.Join(m.baseDir, providerName, "credentials")
}

// GetConfigPath returns the path to the config file for the provider.
func (m *AWSFileManager) GetConfigPath(providerName string) string {
	return filepath.Join(m.baseDir, providerName, "config")
}

// GetEnvironmentVariables returns the AWS file environment variables as EnvironmentVariable slice.
func (m *AWSFileManager) GetEnvironmentVariables(providerName, identityName string) []schema.EnvironmentVariable {
	credentialsPath := m.GetCredentialsPath(providerName)
	configPath := m.GetConfigPath(providerName)

	return []schema.EnvironmentVariable{
		{Key: "AWS_SHARED_CREDENTIALS_FILE", Value: credentialsPath},
		{Key: "AWS_CONFIG_FILE", Value: configPath},
		{Key: "AWS_PROFILE", Value: identityName},
	}
}

// Cleanup removes AWS files for the provider.
func (m *AWSFileManager) Cleanup(providerName string) error {
	providerDir := filepath.Join(m.baseDir, providerName)

	if err := os.RemoveAll(providerDir); err != nil {
		return fmt.Errorf("failed to cleanup AWS files: %w", err)
	}

	return nil
}
