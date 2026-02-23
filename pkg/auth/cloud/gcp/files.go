package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	ini "gopkg.in/ini.v1"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// GCPSubdir is the subdirectory for GCP credentials.
	GCPSubdir = "gcp"
	// ADCSubdir is the subdirectory for Application Default Credentials.
	ADCSubdir = "adc"
	// ConfigSubdir is the subdirectory for gcloud-style config.
	ConfigSubdir = "config"
	// CredentialsFileName is the standard ADC filename.
	CredentialsFileName = "application_default_credentials.json"
	// ActiveConfigFileName is the gcloud active config file.
	ActiveConfigFileName = "active_config"
	// ConfigurationsSubdir is the gcloud configurations directory.
	ConfigurationsSubdir = "configurations"
	// PropertiesFileName is the gcloud properties file.
	PropertiesFileName = "properties"
	// AccessTokenFileName is the filename for the access token file.
	AccessTokenFileName = "access_token"

	// Permission for directories (owner read/write/execute only).
	permDir = 0o700
	// Permission for files (owner read/write only).
	permFile = 0o600

	// File locking timeouts.
	fileLockTimeout = 10 * time.Second
	fileLockRetry   = 50 * time.Millisecond
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
			return nil, fmt.Errorf("failed to acquire file lock within timeout: %s", lockPath)
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

// AuthorizedUserContent represents the structure of an authorized_user ADC JSON file.
type AuthorizedUserContent struct {
	Type         string `json:"type"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	TokenExpiry  string `json:"token_expiry,omitempty"`
}

// GetGCPBaseDir returns the base directory for GCP credentials.
// Returns: ~/.config/atmos/ (realm and gcp subdirs are added by callers).
func GetGCPBaseDir() (string, error) {
	defer perf.Track(nil, "gcp.GetGCPBaseDir")()

	dir, err := xdg.GetXDGConfigDir("", permDir)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return dir, nil
}

// GetProviderDir returns the directory for a specific GCP provider.
// Path structure: {baseDir}/[realm]/gcp/{provider}/.
// When realm is empty, legacy paths are used without a realm subdirectory.
func GetProviderDir(realm, providerName string) (string, error) {
	defer perf.Track(nil, "gcp.GetProviderDir")()

	if err := validatePathSegment("provider name", providerName); err != nil {
		return "", err
	}
	if realm != "" {
		if err := validatePathSegment("realm", realm); err != nil {
			return "", err
		}
	}
	base, err := GetGCPBaseDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, realm, GCPSubdir, providerName)
	if err := os.MkdirAll(dir, permDir); err != nil {
		return "", fmt.Errorf("%w: failed to create provider directory: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return dir, nil
}

// GetADCDir returns the directory for ADC credentials for a specific identity.
// Path structure: {baseDir}/[realm]/gcp/{provider}/adc/{identity}/.
func GetADCDir(realm, providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetADCDir")()

	providerDir, err := GetProviderDir(realm, providerName)
	if err != nil {
		return "", err
	}
	if err := validatePathSegment("identity name", identityName); err != nil {
		return "", err
	}
	dir := filepath.Join(providerDir, ADCSubdir, identityName)
	if err := os.MkdirAll(dir, permDir); err != nil {
		return "", fmt.Errorf("%w: failed to create ADC directory: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return dir, nil
}

// GetADCFilePath returns the path to the ADC JSON file for a specific identity.
// Path structure: {baseDir}/[realm]/gcp/{provider}/adc/{identity}/application_default_credentials.json.
func GetADCFilePath(realm, providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetADCFilePath")()

	dir, err := GetADCDir(realm, providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, CredentialsFileName), nil
}

// GetConfigDir returns the gcloud-style config directory for a specific identity.
// Path structure: {baseDir}/[realm]/gcp/{provider}/config/{identity}/.
func GetConfigDir(realm, providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetConfigDir")()

	providerDir, err := GetProviderDir(realm, providerName)
	if err != nil {
		return "", err
	}
	if err := validatePathSegment("identity name", identityName); err != nil {
		return "", err
	}
	dir := filepath.Join(providerDir, ConfigSubdir, identityName)
	if err := os.MkdirAll(dir, permDir); err != nil {
		return "", fmt.Errorf("%w: failed to create config directory: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return dir, nil
}

// GetPropertiesFilePath returns the path to the gcloud properties file.
// Path structure: {baseDir}/[realm]/gcp/{provider}/config/{identity}/properties.
func GetPropertiesFilePath(realm, providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetPropertiesFilePath")()

	dir, err := GetConfigDir(realm, providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, PropertiesFileName), nil
}

// GetAccessTokenFilePath returns the path to the access token file for an identity.
// Path structure: {baseDir}/[realm]/gcp/{provider}/adc/{identity}/access_token.
func GetAccessTokenFilePath(realm, providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetAccessTokenFilePath")()

	dir, err := GetADCDir(realm, providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AccessTokenFileName), nil
}

// WriteADCFile writes the Application Default Credentials JSON file.
// Uses file locking to prevent concurrent modification conflicts.
func WriteADCFile(realm, providerName, identityName string, content *AuthorizedUserContent) (string, error) {
	defer perf.Track(nil, "gcp.WriteADCFile")()

	if content == nil {
		return "", fmt.Errorf("%w: ADC file content cannot be nil", errUtils.ErrInvalidADCContent)
	}
	path, err := GetADCFilePath(realm, providerName, identityName)
	if err != nil {
		return "", fmt.Errorf("%w: resolve ADC file path: %w", errUtils.ErrWriteADCFile, err)
	}

	// Acquire file lock to prevent concurrent modifications.
	lockPath := path + ".lock"
	lock, err := acquireFileLock(lockPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrWriteADCFile, err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to release file lock", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%w: marshal ADC content: %w", errUtils.ErrWriteADCFile, err)
	}
	if err := os.WriteFile(path, data, permFile); err != nil {
		return "", fmt.Errorf("%w: write ADC file: %w", errUtils.ErrWriteADCFile, err)
	}
	return path, nil
}

// WritePropertiesFile writes the gcloud-style properties file (INI format).
// Uses the ini.v1 library for consistent, properly-escaped INI generation
// and file locking to prevent concurrent modification conflicts.
func WritePropertiesFile(realm, providerName, identityName string, projectID string, region string) (string, error) {
	defer perf.Track(nil, "gcp.WritePropertiesFile")()

	path, err := GetPropertiesFilePath(realm, providerName, identityName)
	if err != nil {
		return "", fmt.Errorf("%w: resolve properties file path: %w", errUtils.ErrWritePropertiesFile, err)
	}

	// Acquire file lock to prevent concurrent modifications.
	lockPath := path + ".lock"
	lock, err := acquireFileLock(lockPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrWritePropertiesFile, err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to release file lock", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	// Use ini.v1 library for consistent, properly-escaped INI generation.
	cfg := ini.Empty()

	// [core] section.
	coreSection, err := cfg.NewSection("core")
	if err != nil {
		return "", fmt.Errorf("%w: create core section: %w", errUtils.ErrWritePropertiesFile, err)
	}
	if projectID != "" {
		if _, err := coreSection.NewKey("project", projectID); err != nil {
			return "", fmt.Errorf("%w: set project key: %w", errUtils.ErrWritePropertiesFile, err)
		}
	}

	// [compute] section.
	computeSection, err := cfg.NewSection("compute")
	if err != nil {
		return "", fmt.Errorf("%w: create compute section: %w", errUtils.ErrWritePropertiesFile, err)
	}
	if region != "" {
		if _, err := computeSection.NewKey("region", region); err != nil {
			return "", fmt.Errorf("%w: set region key: %w", errUtils.ErrWritePropertiesFile, err)
		}
	}

	if err := cfg.SaveTo(path); err != nil {
		return "", fmt.Errorf("%w: write properties file: %w", errUtils.ErrWritePropertiesFile, err)
	}

	// Set proper file permissions.
	if err := os.Chmod(path, permFile); err != nil {
		return "", fmt.Errorf("%w: set properties file permissions: %w", errUtils.ErrWritePropertiesFile, err)
	}

	return path, nil
}

// WriteAccessTokenFile writes a simple access token file for tools that need it.
// Uses file locking to prevent concurrent modification conflicts.
// Realm is required for credential isolation.
func WriteAccessTokenFile(realm, providerName, identityName string, accessToken string, expiry time.Time) (string, error) {
	defer perf.Track(nil, "gcp.WriteAccessTokenFile")()

	if accessToken == "" {
		return "", fmt.Errorf("%w: access token cannot be empty", errUtils.ErrWriteAccessTokenFile)
	}
	path, err := GetAccessTokenFilePath(realm, providerName, identityName)
	if err != nil {
		return "", fmt.Errorf("%w: resolve access token file path: %w", errUtils.ErrWriteAccessTokenFile, err)
	}

	// Acquire file lock to prevent concurrent modifications.
	lockPath := path + ".lock"
	lock, err := acquireFileLock(lockPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrWriteAccessTokenFile, err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to release file lock", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	content := accessToken + "\n"
	if !expiry.IsZero() {
		content += expiry.Format(time.RFC3339) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), permFile); err != nil {
		return "", fmt.Errorf("%w: write access token file: %w", errUtils.ErrWriteAccessTokenFile, err)
	}
	return path, nil
}

// CleanupIdentityFiles removes all credential files for an identity.
// Uses file locking to prevent concurrent modification conflicts.
// Realm is required for credential isolation.
func CleanupIdentityFiles(realm, providerName, identityName string) error {
	defer perf.Track(nil, "gcp.CleanupIdentityFiles")()

	providerDir, err := GetProviderDir(realm, providerName)
	if err != nil {
		return err
	}
	if err := validatePathSegment("identity name", identityName); err != nil {
		return err
	}

	// Acquire file lock on the provider directory to prevent concurrent cleanup conflicts.
	lockPath := filepath.Join(providerDir, identityName+".cleanup.lock")
	lock, err := acquireFileLock(lockPath)
	if err != nil {
		return fmt.Errorf("cleanup identity files: %w", err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to release file lock", "lock_file", lockPath, "error", unlockErr)
		}
		// Clean up the lock file itself after releasing the lock.
		os.Remove(lockPath)
	}()

	adcDir := filepath.Join(providerDir, ADCSubdir, identityName)
	configDir := filepath.Join(providerDir, ConfigSubdir, identityName)
	var errs []error
	for _, dir := range []string{adcDir, configDir} {
		if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
			errs = append(errs, errors.Join(errUtils.ErrRemoveDirectory, fmt.Errorf("failed to remove %s: %w", dir, err)))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func validatePathSegment(label, value string) error {
	if value == "" {
		return fmt.Errorf("%w: %s is required", errUtils.ErrInvalidAuthConfig, label)
	}
	if value == "." || value == ".." {
		return fmt.Errorf("%w: %s must not be %q", errUtils.ErrInvalidAuthConfig, label, value)
	}
	if strings.ContainsAny(value, "/\\") {
		return fmt.Errorf("%w: %s must not contain path separators", errUtils.ErrInvalidAuthConfig, label)
	}
	return nil
}
