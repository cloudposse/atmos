package aws

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	ini "gopkg.in/ini.v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	PermissionRWX = 0o700
	PermissionRW  = 0o600
)

var (
	ErrGetHomeDir                    = errors.New("failed to get home directory")
	ErrCreateCredentialsFile         = errors.New("failed to create credentials file")
	ErrCreateConfigFile              = errors.New("failed to create config file")
	ErrLoadCredentialsFile           = errors.New("failed to load credentials file")
	ErrLoadConfigFile                = errors.New("failed to load config file")
	ErrWriteCredentialsFile          = errors.New("failed to write credentials file")
	ErrWriteConfigFile               = errors.New("failed to write config file")
	ErrSetCredentialsFilePermissions = errors.New("failed to set credentials file permissions")
	ErrSetConfigFilePermissions      = errors.New("failed to set config file permissions")
	ErrProfileSection                = errors.New("failed to get profile section")
	ErrCleanupAWSFiles               = errors.New("failed to cleanup AWS files")
)

// AWSFileManager provides helpers to manage AWS credentials/config files.
type AWSFileManager struct {
	baseDir string
}

// NewAWSFileManager creates a new AWS file manager instance.
// BasePath is optional and can be empty to use defaults.
// Precedence: 1) basePath parameter from provider spec, 2) default ~/.aws/atmos.
func NewAWSFileManager(basePath string) (*AWSFileManager, error) {
	var baseDir string

	if basePath != "" {
		// Use configured path from provider spec.
		expanded, err := homedir.Expand(basePath)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid base_path %q: %w", ErrGetHomeDir, basePath, err)
		}
		baseDir = expanded
	} else {
		// Default: ~/.aws/atmos
		homeDir, err := homedir.Dir()
		if err != nil {
			return nil, ErrGetHomeDir
		}
		baseDir = filepath.Join(homeDir, ".aws", "atmos")
	}

	return &AWSFileManager{
		baseDir: baseDir,
	}, nil
}

// WriteCredentials writes AWS credentials to the provider-specific file with identity profile.
func (m *AWSFileManager) WriteCredentials(providerName, identityName string, creds *types.AWSCredentials) error {
	credentialsPath := m.GetCredentialsPath(providerName)

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(credentialsPath), PermissionRWX); err != nil {
		errUtils.CheckErrorAndPrint(ErrCreateCredentialsFile, identityName, "failed to create credentials directory")
		return ErrCreateCredentialsFile
	}

	// Load existing INI file or create new one.
	cfg, err := ini.Load(credentialsPath)
	if err != nil {
		// ini.Load returns a wrapped error, check if the file doesn't exist.
		if !os.IsNotExist(err) {
			errUtils.CheckErrorAndPrint(ErrLoadCredentialsFile, identityName, "failed to load credentials file")
			return ErrLoadCredentialsFile
		}
		cfg = ini.Empty()
	}

	// Get or create the profile section.
	section, err := cfg.GetSection(identityName)
	if err != nil {
		section, err = cfg.NewSection(identityName)
		if err != nil {
			errUtils.CheckErrorAndPrint(ErrProfileSection, identityName, "failed to create profile section")
			return ErrProfileSection
		}
	}

	// Set credentials.
	section.Key("aws_access_key_id").SetValue(creds.AccessKeyID)
	section.Key("aws_secret_access_key").SetValue(creds.SecretAccessKey)
	if creds.SessionToken != "" {
		section.Key("aws_session_token").SetValue(creds.SessionToken)
	} else {
		// Remove session token if not present.
		section.DeleteKey("aws_session_token")
	}

	// Save file with proper permissions.
	if err := cfg.SaveTo(credentialsPath); err != nil {
		errUtils.CheckErrorAndPrint(ErrWriteCredentialsFile, identityName, "failed to write credentials file")
		return ErrWriteCredentialsFile
	}

	// Set proper file permissions.
	if err := os.Chmod(credentialsPath, PermissionRW); err != nil {
		errUtils.CheckErrorAndPrint(ErrSetCredentialsFilePermissions, identityName, "failed to set credentials file permissions")
		return ErrSetCredentialsFilePermissions
	}

	return nil
}

// WriteConfig writes AWS config to the provider-specific file with identity profile.
func (m *AWSFileManager) WriteConfig(providerName, identityName, region, outputFormat string) error {
	configPath := m.GetConfigPath(providerName)

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(configPath), PermissionRWX); err != nil {
		errUtils.CheckErrorAndPrint(ErrCreateConfigFile, identityName, "failed to create config directory")
		return ErrCreateConfigFile
	}

	// Load existing INI file or create new one.
	cfg, err := ini.Load(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			errUtils.CheckErrorAndPrint(ErrLoadConfigFile, identityName, "failed to load config file")
			return ErrLoadConfigFile
		}
		cfg = ini.Empty()
	}
	// Get or create the profile section (AWS config uses "profile name" format, except for "default").
	var profileSectionName string
	if identityName == "default" {
		profileSectionName = "default"
	} else {
		profileSectionName = fmt.Sprintf("profile %s", identityName)
	}

	section := cfg.Section(profileSectionName)
	log.Debug("AWS WriteConfig", "providerName", providerName, "identityName", identityName, "region", region, "outputFormat", outputFormat)

	// Set config values only if they are not empty.
	if region != "" {
		section.Key("region").SetValue(region)
	} else {
		// Remove region key if not present.
		section.DeleteKey("region")
	}

	// Set output format only if explicitly provided.
	if outputFormat != "" {
		section.Key("output").SetValue(outputFormat)
	} else {
		// Remove output key if not present.
		section.DeleteKey("output")
	}

	// Save file with proper permissions.
	if err := cfg.SaveTo(configPath); err != nil {
		errUtils.CheckErrorAndPrint(ErrWriteConfigFile, identityName, "failed to write config file")
		return ErrWriteConfigFile
	}

	// Set proper file permissions.
	if err := os.Chmod(configPath, PermissionRW); err != nil {
		errUtils.CheckErrorAndPrint(ErrSetConfigFilePermissions, identityName, "failed to set config file permissions")
		return ErrSetConfigFilePermissions
	}

	return nil
}

// GetBaseDir returns the base directory path.
func (m *AWSFileManager) GetBaseDir() string {
	return m.baseDir
}

// GetDisplayPath returns a user-friendly display path (with ~ if under home directory).
func (m *AWSFileManager) GetDisplayPath() string {
	homeDir, err := homedir.Dir()
	if err == nil && homeDir != "" && strings.HasPrefix(m.baseDir, homeDir) {
		return strings.Replace(m.baseDir, homeDir, "~", 1)
	}
	return m.baseDir
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
		// If directory doesn't exist, that's not an error (already cleaned up).
		if os.IsNotExist(err) {
			return nil
		}
		errUtils.CheckErrorAndPrint(ErrCleanupAWSFiles, providerDir, "failed to cleanup AWS files")
		return ErrCleanupAWSFiles
	}

	return nil
}

// CleanupIdentity removes only the specified identity's sections from AWS INI files.
// This preserves other identities using the same provider.
func (m *AWSFileManager) CleanupIdentity(providerName, identityName string) error {
	var errs []error

	// Remove identity section from credentials file.
	credentialsPath := m.GetCredentialsPath(providerName)
	if err := m.removeIniSection(credentialsPath, identityName); err != nil {
		errs = append(errs, fmt.Errorf("failed to remove credentials section: %w", err))
	}

	// Remove identity section from config file.
	// AWS config uses "profile <name>" format, except for "default".
	configPath := m.GetConfigPath(providerName)
	configSectionName := identityName
	if identityName != "default" {
		configSectionName = "profile " + identityName
	}
	if err := m.removeIniSection(configPath, configSectionName); err != nil {
		errs = append(errs, fmt.Errorf("failed to remove config section: %w", err))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// removeIniSection removes a section from an INI file.
func (m *AWSFileManager) removeIniSection(filePath, sectionName string) error {
	// Load INI file.
	cfg, err := ini.Load(filePath)
	if err != nil {
		// If file doesn't exist, section is already removed.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to load INI file: %w", err)
	}

	// Delete the section.
	cfg.DeleteSection(sectionName)

	// If no sections remain, remove the file entirely.
	if len(cfg.Sections()) == 1 && cfg.Sections()[0].Name() == ini.DefaultSection {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove empty file: %w", err)
		}
		log.Debug("Removed empty INI file", "path", filePath)
		return nil
	}

	// Save the updated INI file.
	if err := cfg.SaveTo(filePath); err != nil {
		return fmt.Errorf("failed to save INI file: %w", err)
	}

	log.Debug("Removed INI section", "file", filePath, "section", sectionName)
	return nil
}

// CleanupAll removes entire base directory (all providers).
func (m *AWSFileManager) CleanupAll() error {
	if err := os.RemoveAll(m.baseDir); err != nil {
		// If directory doesn't exist, that's not an error (already cleaned up).
		if os.IsNotExist(err) {
			return nil
		}
		errUtils.CheckErrorAndPrint(ErrCleanupAWSFiles, m.baseDir, "failed to cleanup all AWS files")
		return ErrCleanupAWSFiles
	}

	return nil
}
