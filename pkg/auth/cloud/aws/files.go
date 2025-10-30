package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	ini "gopkg.in/ini.v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	PermissionRWX = 0o700
	PermissionRW  = 0o600

	// File locking timeouts.
	fileLockTimeout = 10 * time.Second
	fileLockRetry   = 50 * time.Millisecond

	// Logging keys.
	logKeyProvider = "provider"
	logKeyIdentity = "identity"
	logKeyProfile  = "profile"
)

// legacyPathWarningOnce ensures we only warn about legacy path once per execution.
var legacyPathWarningOnce sync.Once

// maskAccessKey returns the first 4 characters of an access key for logging.
func maskAccessKey(accessKey string) string {
	if len(accessKey) > 4 {
		return accessKey[:4] + "..."
	}
	return accessKey
}

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
	ErrFileLockTimeout               = errors.New("failed to acquire file lock within timeout")
	ErrRemoveProfile                 = errors.New("failed to remove profile")
)

// acquireFileLock attempts to acquire an exclusive file lock with timeout and retries.
func acquireFileLock(lockPath string) (*flock.Flock, error) {
	lock := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), fileLockTimeout)
	defer cancel()

	ticker := time.NewTicker(fileLockRetry)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %s", ErrFileLockTimeout, lockPath)
		case <-ticker.C:
			locked, err := lock.TryLock()
			if err != nil {
				return nil, fmt.Errorf("failed to acquire lock: %w", err)
			}
			if locked {
				log.Debug("Acquired file lock", "lock_file", lockPath)
				return lock, nil
			}
			log.Debug("Waiting for file lock", "lock_file", lockPath)
		}
	}
}

// AWSFileManager provides helpers to manage AWS credentials/config files.
type AWSFileManager struct {
	baseDir string
}

// LoadINIFile loads an INI file with options that preserve section comments.
// This is critical for maintaining expiration metadata in credentials files.
func LoadINIFile(path string) (*ini.File, error) {
	return ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: false,
	}, path)
}

// NewAWSFileManager creates a new AWS file manager instance.
// BasePath is optional and can be empty to use defaults.
// Precedence: 1) basePath parameter from provider spec, 2) XDG config directory.
//
// Default path follows XDG Base Directory Specification:
//   - Linux: $XDG_CONFIG_HOME/atmos/aws (default: ~/.config/atmos/aws)
//   - macOS: ~/Library/Application Support/atmos/aws
//   - Windows: %APPDATA%\atmos\aws
//
// Respects ATMOS_XDG_CONFIG_HOME and XDG_CONFIG_HOME environment variables.
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
		// Default: Use XDG config directory for AWS credentials.
		// This keeps Atmos-managed AWS credentials under Atmos's namespace,
		// following the same pattern as cache and keyring storage.
		var err error
		baseDir, err = xdg.GetXDGConfigDir("aws", PermissionRWX)
		if err != nil {
			return nil, fmt.Errorf("failed to get XDG config directory for AWS: %w", err)
		}

		// Check for legacy ~/.aws/atmos path and warn if found.
		checkLegacyAWSAtmosPath(baseDir)
	}

	return &AWSFileManager{
		baseDir: baseDir,
	}, nil
}

// checkLegacyAWSAtmosPath checks if the legacy ~/.aws/atmos directory exists
// and logs a warning if it does, informing users about the new XDG-compliant location.
// Uses sync.Once to ensure the warning is only shown once per execution.
func checkLegacyAWSAtmosPath(newBaseDir string) {
	legacyPathWarningOnce.Do(func() {
		homeDir, err := homedir.Dir()
		if err != nil {
			return // Cannot determine home directory, skip check.
		}

		legacyPath := filepath.Join(homeDir, ".aws", "atmos")

		// Check if legacy path exists.
		if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
			return // Legacy path doesn't exist, nothing to warn about.
		}

		// Log warning about legacy path.
		log.Warn(fmt.Sprintf(
			"Legacy AWS credentials directory detected at %s. "+
				"Atmos now uses XDG Base Directory Specification. "+
				"New credentials are stored at %s. "+
				"Run 'atmos auth login' to re-authenticate and store credentials in the new location.",
			legacyPath,
			newBaseDir,
		))
	})
}

// WriteCredentials writes AWS credentials to the provider-specific file with identity profile.
// Uses file locking to prevent concurrent modification conflicts.
func (m *AWSFileManager) WriteCredentials(providerName, identityName string, creds *types.AWSCredentials) error {
	credentialsPath := m.GetCredentialsPath(providerName)

	log.Debug("Writing AWS credentials",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"credentials_file", credentialsPath,
		"has_session_token", creds.SessionToken != "",
	)

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(credentialsPath), PermissionRWX); err != nil {
		errUtils.CheckErrorAndPrint(ErrCreateCredentialsFile, identityName, "failed to create credentials directory")
		return ErrCreateCredentialsFile
	}

	// Acquire file lock to prevent concurrent modifications.
	lockPath := credentialsPath + ".lock"
	lock, err := acquireFileLock(lockPath)
	if err != nil {
		errUtils.CheckErrorAndPrint(ErrWriteCredentialsFile, identityName, "failed to acquire file lock")
		return fmt.Errorf("%w: %w", ErrWriteCredentialsFile, err)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Debug("Failed to release file lock", "lock_file", lockPath, "error", err)
		}
	}()

	// Load existing INI file or create new one.
	cfg, err := LoadINIFile(credentialsPath)
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

	// Add metadata comment with expiration if available (before section keys).
	// This comment is critical for credential validation when keychain access is unavailable
	// (e.g., inside Docker containers). The expiration timestamp serves as a fallback
	// to determine if credentials are still valid.
	if creds.Expiration != "" {
		section.Comment = fmt.Sprintf("atmos: expiration=%s", creds.Expiration)
	} else {
		section.Comment = ""
	}

	// Set credentials.
	section.Key("aws_access_key_id").SetValue(creds.AccessKeyID)
	section.Key("aws_secret_access_key").SetValue(creds.SecretAccessKey)
	if creds.SessionToken != "" {
		section.Key("aws_session_token").SetValue(creds.SessionToken)
		// Debug: Log credential details (masked).
		log.Debug("Writing credentials to file",
			"profile", identityName,
			"access_key_prefix", maskAccessKey(creds.AccessKeyID),
			"has_session_token", true,
			"expiration", creds.Expiration)
	} else {
		// Remove session token if not present.
		section.DeleteKey("aws_session_token")
		log.Debug("Writing credentials to file (no session token)",
			"profile", identityName,
			"access_key_prefix", maskAccessKey(creds.AccessKeyID))
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

	log.Debug("Successfully wrote AWS credentials",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"credentials_file", credentialsPath,
	)

	return nil
}

// WriteConfig writes AWS config to the provider-specific file with identity profile.
// Uses file locking to prevent concurrent modification conflicts.
func (m *AWSFileManager) WriteConfig(providerName, identityName, region, outputFormat string) error {
	configPath := m.GetConfigPath(providerName)

	log.Debug("Writing AWS config",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"config_file", configPath,
		"region", region,
		"output_format", outputFormat,
	)

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(configPath), PermissionRWX); err != nil {
		errUtils.CheckErrorAndPrint(ErrCreateConfigFile, identityName, "failed to create config directory")
		return ErrCreateConfigFile
	}

	// Acquire file lock to prevent concurrent modifications.
	lockPath := configPath + ".lock"
	lock, err := acquireFileLock(lockPath)
	if err != nil {
		errUtils.CheckErrorAndPrint(ErrWriteConfigFile, identityName, "failed to acquire file lock")
		return fmt.Errorf("%w: %w", ErrWriteConfigFile, err)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Debug("Failed to release file lock", "lock_file", lockPath, "error", err)
		}
	}()

	// Load existing INI file or create new one.
	cfg, err := LoadINIFile(configPath)
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

	log.Debug("Successfully wrote AWS config",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"config_file", configPath,
	)

	return nil
}

// RemoveConfigProfile removes an identity profile from the provider's config file.
// Uses file locking to prevent concurrent modification conflicts.
// If this is the last profile, the config file is removed entirely.
func (m *AWSFileManager) RemoveConfigProfile(providerName, identityName string) error {
	defer perf.Track(nil, "aws.files.RemoveConfigProfile")()

	configPath := m.GetConfigPath(providerName)

	log.Debug("Removing AWS config profile",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"config_file", configPath,
	)

	// Check if file exists.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Debug("Config file does not exist, nothing to remove",
			logKeyProvider, providerName,
			"config_file", configPath,
		)
		return nil
	}

	// Acquire file lock to prevent concurrent modifications.
	lockPath := configPath + ".lock"
	lock, err := acquireFileLock(lockPath)
	if err != nil {
		errUtils.CheckErrorAndPrint(ErrRemoveProfile, identityName, "failed to acquire file lock")
		return fmt.Errorf("%w: %w", ErrRemoveProfile, err)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Debug("Failed to release file lock", "lock_file", lockPath, "error", err)
		}
	}()

	// Load existing INI file.
	cfg, err := LoadINIFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted between stat and lock acquisition, that's fine.
			return nil
		}
		errUtils.CheckErrorAndPrint(ErrRemoveProfile, identityName, "failed to load config file")
		return ErrRemoveProfile
	}

	// Determine section name (AWS config uses "profile name" format, except for "default").
	var profileSectionName string
	if identityName == "default" {
		profileSectionName = "default"
	} else {
		profileSectionName = fmt.Sprintf("profile %s", identityName)
	}

	// Delete the profile section.
	cfg.DeleteSection(profileSectionName)

	// If no sections remain (or only DEFAULT section which ini library creates), remove the file.
	sections := cfg.Sections()
	hasProfiles := false
	for _, section := range sections {
		// Skip the DEFAULT section (it's always present in ini files).
		if section.Name() != ini.DefaultSection {
			hasProfiles = true
			break
		}
	}

	if !hasProfiles {
		log.Debug("No profiles remain, removing config file",
			logKeyProvider, providerName,
			"config_file", configPath,
		)
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			errUtils.CheckErrorAndPrint(ErrRemoveProfile, identityName, "failed to remove config file")
			return ErrRemoveProfile
		}
	} else {
		// Save the updated config file.
		if err := cfg.SaveTo(configPath); err != nil {
			errUtils.CheckErrorAndPrint(ErrRemoveProfile, identityName, "failed to save config file")
			return ErrRemoveProfile
		}
	}

	log.Debug("Successfully removed AWS config profile",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
	)

	return nil
}

// RemoveCredentialsProfile removes an identity profile from the provider's credentials file.
// Uses file locking to prevent concurrent modification conflicts.
// If this is the last profile, the credentials file is removed entirely.
func (m *AWSFileManager) RemoveCredentialsProfile(providerName, identityName string) error {
	defer perf.Track(nil, "aws.files.RemoveCredentialsProfile")()

	credentialsPath := m.GetCredentialsPath(providerName)

	log.Debug("Removing AWS credentials profile",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
		"credentials_file", credentialsPath,
	)

	// Check if file exists.
	if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
		log.Debug("Credentials file does not exist, nothing to remove",
			logKeyProvider, providerName,
			"credentials_file", credentialsPath,
		)
		return nil
	}

	// Acquire file lock to prevent concurrent modifications.
	lockPath := credentialsPath + ".lock"
	lock, err := acquireFileLock(lockPath)
	if err != nil {
		errUtils.CheckErrorAndPrint(ErrRemoveProfile, identityName, "failed to acquire file lock")
		return fmt.Errorf("%w: %w", ErrRemoveProfile, err)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Debug("Failed to release file lock", "lock_file", lockPath, "error", err)
		}
	}()

	// Load existing INI file.
	cfg, err := LoadINIFile(credentialsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted between stat and lock acquisition, that's fine.
			return nil
		}
		errUtils.CheckErrorAndPrint(ErrRemoveProfile, identityName, "failed to load credentials file")
		return ErrRemoveProfile
	}

	// AWS credentials use plain profile names (no "profile" prefix).
	profileSectionName := identityName

	// Delete the profile section.
	cfg.DeleteSection(profileSectionName)

	// If no sections remain (or only DEFAULT section which ini library creates), remove the file.
	sections := cfg.Sections()
	hasProfiles := false
	for _, section := range sections {
		// Skip the DEFAULT section (it's always present in ini files).
		if section.Name() != ini.DefaultSection {
			hasProfiles = true
			break
		}
	}

	if !hasProfiles {
		log.Debug("No profiles remain, removing credentials file",
			logKeyProvider, providerName,
			"credentials_file", credentialsPath,
		)
		if err := os.Remove(credentialsPath); err != nil && !os.IsNotExist(err) {
			errUtils.CheckErrorAndPrint(ErrRemoveProfile, identityName, "failed to remove credentials file")
			return ErrRemoveProfile
		}
	} else {
		// Save the updated credentials file.
		if err := cfg.SaveTo(credentialsPath); err != nil {
			errUtils.CheckErrorAndPrint(ErrRemoveProfile, identityName, "failed to save credentials file")
			return ErrRemoveProfile
		}
	}

	log.Debug("Successfully removed AWS credentials profile",
		logKeyProvider, providerName,
		logKeyIdentity, identityName,
	)

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
	defer perf.Track(nil, "aws.files.Cleanup")()

	providerDir := filepath.Join(m.baseDir, providerName)

	log.Debug("Cleaning up AWS files directory",
		"provider", providerName,
		"directory", providerDir)

	if err := os.RemoveAll(providerDir); err != nil {
		// If directory doesn't exist, that's not an error (already cleaned up).
		if os.IsNotExist(err) {
			log.Debug("AWS files directory already removed (does not exist)",
				"provider", providerName,
				"directory", providerDir)
			return nil
		}
		errUtils.CheckErrorAndPrint(ErrCleanupAWSFiles, providerDir, "failed to cleanup AWS files")
		return ErrCleanupAWSFiles
	}

	log.Debug("Successfully removed AWS files directory",
		"provider", providerName,
		"directory", providerDir)

	return nil
}

// DeleteIdentity removes only the specified identity's sections from AWS INI files.
// This preserves other identities using the same provider.
// Uses file locking to prevent concurrent modification conflicts.
func (m *AWSFileManager) DeleteIdentity(ctx context.Context, providerName, identityName string) error {
	defer perf.Track(nil, "aws.files.DeleteIdentity")()

	var errs []error

	// Remove identity section from credentials file (with file locking).
	if err := m.RemoveCredentialsProfile(providerName, identityName); err != nil {
		errs = append(errs, fmt.Errorf("failed to remove credentials profile: %w", err))
	}

	// Remove identity section from config file (with file locking).
	if err := m.RemoveConfigProfile(providerName, identityName); err != nil {
		errs = append(errs, fmt.Errorf("failed to remove config profile: %w", err))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// removeIniSection removes a section from an INI file.
func (m *AWSFileManager) removeIniSection(filePath, sectionName string) error {
	// Load INI file.
	cfg, err := LoadINIFile(filePath)
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
	defer perf.Track(nil, "aws.files.CleanupAll")()

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
