package gcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// ADCFileContent represents the structure of an ADC JSON file.
type ADCFileContent struct {
	Type         string `json:"type"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	TokenExpiry  string `json:"token_expiry,omitempty"`

	// For service account impersonation.
	ServiceAccountImpersonationURL string `json:"service_account_impersonation_url,omitempty"`

	// For workload identity federation.
	Audience         string            `json:"audience,omitempty"`
	SubjectTokenType string            `json:"subject_token_type,omitempty"`
	TokenURL         string            `json:"token_url,omitempty"`
	CredentialSource *CredentialSource `json:"credential_source,omitempty"`
}

// CredentialSource defines where to get the source credential for WIF.
type CredentialSource struct {
	File          string            `json:"file,omitempty"`
	URL           string            `json:"url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	EnvironmentID string            `json:"environment_id,omitempty"`
	Format        *CredentialFormat `json:"format,omitempty"`
}

// CredentialFormat defines the format of the credential source.
type CredentialFormat struct {
	Type                  string `json:"type,omitempty"`
	SubjectTokenFieldName string `json:"subject_token_field_name,omitempty"`
}

// GetGCPBaseDir returns the base directory for GCP credentials.
// Returns: ~/.config/atmos/gcp/.
func GetGCPBaseDir() (string, error) {
	defer perf.Track(nil, "gcp.GetGCPBaseDir")()

	return xdg.GetXDGConfigDir(GCPSubdir, permDir)
}

// GetProviderDir returns the directory for a specific GCP provider.
// Returns: ~/.config/atmos/gcp/<provider-name>/.
func GetProviderDir(providerName string) (string, error) {
	defer perf.Track(nil, "gcp.GetProviderDir")()

	if providerName == "" {
		return "", fmt.Errorf("%w: provider name is required", errUtils.ErrInvalidAuthConfig)
	}
	base, err := GetGCPBaseDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, providerName)
	if err := os.MkdirAll(dir, permDir); err != nil {
		return "", fmt.Errorf("%w: failed to create provider directory: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return dir, nil
}

// GetADCDir returns the directory for ADC credentials for a specific identity.
// Returns: ~/.config/atmos/gcp/<provider-name>/adc/<identity-name>/.
func GetADCDir(providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetADCDir")()

	providerDir, err := GetProviderDir(providerName)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(providerDir, ADCSubdir, identityName)
	if err := os.MkdirAll(dir, permDir); err != nil {
		return "", fmt.Errorf("%w: failed to create ADC directory: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return dir, nil
}

// GetADCFilePath returns the path to the ADC JSON file for a specific identity.
// Returns: ~/.config/atmos/gcp/<provider-name>/adc/<identity-name>/application_default_credentials.json.
func GetADCFilePath(providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetADCFilePath")()

	dir, err := GetADCDir(providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, CredentialsFileName), nil
}

// GetConfigDir returns the gcloud-style config directory for a specific identity.
// Returns: ~/.config/atmos/gcp/<provider-name>/config/<identity-name>/.
func GetConfigDir(providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetConfigDir")()

	providerDir, err := GetProviderDir(providerName)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(providerDir, ConfigSubdir, identityName)
	if err := os.MkdirAll(dir, permDir); err != nil {
		return "", fmt.Errorf("%w: failed to create config directory: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return dir, nil
}

// GetPropertiesFilePath returns the path to the gcloud properties file.
// Returns: ~/.config/atmos/gcp/<provider-name>/config/<identity-name>/properties
func GetPropertiesFilePath(providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetPropertiesFilePath")()

	dir, err := GetConfigDir(providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, PropertiesFileName), nil
}

// GetAccessTokenFilePath returns the path to the access token file for an identity.
// Returns: ~/.config/atmos/gcp/<provider-name>/adc/<identity-name>/access_token
func GetAccessTokenFilePath(providerName, identityName string) (string, error) {
	defer perf.Track(nil, "gcp.GetAccessTokenFilePath")()

	dir, err := GetADCDir(providerName, identityName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AccessTokenFileName), nil
}

// WriteADCFile writes the Application Default Credentials JSON file.
func WriteADCFile(providerName, identityName string, content *ADCFileContent) (string, error) {
	defer perf.Track(nil, "gcp.WriteADCFile")()

	if content == nil {
		return "", fmt.Errorf("%w: ADC file content cannot be nil", errUtils.ErrInvalidADCContent)
	}
	path, err := GetADCFilePath(providerName, identityName)
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
func WritePropertiesFile(providerName, identityName string, projectID string, region string) (string, error) {
	defer perf.Track(nil, "gcp.WritePropertiesFile")()

	path, err := GetPropertiesFilePath(providerName, identityName)
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
func WriteAccessTokenFile(providerName, identityName string, accessToken string, expiry time.Time) (string, error) {
	defer perf.Track(nil, "gcp.WriteAccessTokenFile")()

	path, err := GetAccessTokenFilePath(providerName, identityName)
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
func CleanupIdentityFiles(providerName, identityName string) error {
	defer perf.Track(nil, "gcp.CleanupIdentityFiles")()

	providerDir, err := GetProviderDir(providerName)
	if err != nil {
		return err
	}
	adcDir := filepath.Join(providerDir, ADCSubdir, identityName)
	configDir := filepath.Join(providerDir, ConfigSubdir, identityName)
	for _, dir := range []string{adcDir, configDir} {
		if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", dir, err)
		}
	}
	return nil
}
