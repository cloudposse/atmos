package gcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
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
)

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
// Path structure: {baseDir}/{realm}/gcp/{provider}/.
// Realm is required for credential isolation between different Atmos configurations.
func GetProviderDir(realm, providerName string) (string, error) {
	defer perf.Track(nil, "gcp.GetProviderDir")()

	if err := validatePathSegment("provider name", providerName); err != nil {
		return "", err
	}
	if err := validatePathSegment("realm", realm); err != nil {
		return "", err
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
// Path structure: {baseDir}/{realm}/gcp/{provider}/adc/{identity}/.
// Realm is required for credential isolation.
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
// Path structure: {baseDir}/{realm}/gcp/{provider}/adc/{identity}/application_default_credentials.json.
// Realm is required for credential isolation.
func GetADCFilePath(realm, providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetADCFilePath")()

	dir, err := GetADCDir(realm, providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, CredentialsFileName), nil
}

// GetConfigDir returns the gcloud-style config directory for a specific identity.
// Path structure: {baseDir}/{realm}/gcp/{provider}/config/{identity}/.
// Realm is required for credential isolation.
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
// Path structure: {baseDir}/{realm}/gcp/{provider}/config/{identity}/properties.
// Realm is required for credential isolation.
func GetPropertiesFilePath(realm, providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetPropertiesFilePath")()

	dir, err := GetConfigDir(realm, providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, PropertiesFileName), nil
}

// GetAccessTokenFilePath returns the path to the access token file for an identity.
// Path structure: {baseDir}/{realm}/gcp/{provider}/adc/{identity}/access_token.
// Realm is required for credential isolation.
func GetAccessTokenFilePath(realm, providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetAccessTokenFilePath")()

	dir, err := GetADCDir(realm, providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AccessTokenFileName), nil
}

// WriteADCFile writes the Application Default Credentials JSON file.
// Realm is required for credential isolation.
func WriteADCFile(realm, providerName, identityName string, content *AuthorizedUserContent) (string, error) {
	defer perf.Track(nil, "gcp.WriteADCFile")()

	if content == nil {
		return "", fmt.Errorf("%w: ADC file content cannot be nil", errUtils.ErrInvalidADCContent)
	}
	path, err := GetADCFilePath(realm, providerName, identityName)
	if err != nil {
		return "", fmt.Errorf("%w: resolve ADC file path: %w", errUtils.ErrWriteADCFile, err)
	}
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
// Realm is required for credential isolation.
func WritePropertiesFile(realm, providerName, identityName string, projectID string, region string) (string, error) {
	defer perf.Track(nil, "gcp.WritePropertiesFile")()

	path, err := GetPropertiesFilePath(realm, providerName, identityName)
	if err != nil {
		return "", fmt.Errorf("%w: resolve properties file path: %w", errUtils.ErrWritePropertiesFile, err)
	}
	b := []byte("[core]\n")
	if projectID != "" {
		b = append(b, []byte("project = "+projectID+"\n")...)
	}
	b = append(b, []byte("\n[compute]\n")...)
	if region != "" {
		b = append(b, []byte("region = "+region+"\n")...)
	}
	if err := os.WriteFile(path, b, permFile); err != nil {
		return "", fmt.Errorf("%w: write properties file: %w", errUtils.ErrWritePropertiesFile, err)
	}
	return path, nil
}

// WriteAccessTokenFile writes a simple access token file for tools that need it.
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
