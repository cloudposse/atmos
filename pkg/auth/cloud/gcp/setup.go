package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SetupFiles writes all necessary credential files for a GCP identity.
// This includes:
//   - ADC JSON file (application_default_credentials.json).
//   - gcloud properties file (for project/region defaults).
//   - Access token file (for tools that read tokens directly).
//
// Parameters:
//   - ctx: Context for cancellation.
//   - atmosConfig: Atmos configuration.
//   - realm: The realm for credential isolation (typically derived from atmos config hash).
//   - providerName: Name of the provider being set up.
//   - identityName: Name of the identity being set up.
//   - creds: The GCP credentials to write.
//
// Returns the paths of all written files and any error.
func SetupFiles(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	realm string,
	providerName string,
	identityName string,
	creds *types.GCPCredentials,
) ([]string, error) {
	defer perf.Track(nil, "gcp.SetupFiles")()

	_ = ctx
	_ = atmosConfig
	if creds == nil {
		return nil, fmt.Errorf("%w: GCP credentials cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	var paths []string

	// ADC file.
	// We use "authorized_user" type which requires client_id and client_secret.
	// These are the public gcloud CLI credentials (publicly documented, used by gcloud itself).
	// Without these, the Google Cloud SDK's threelegged.go throws "auth: client ID must be provided".
	clientID, clientSecret, err := resolveADCClientCredentials()
	if err != nil {
		return nil, err
	}
	adcContent := &ADCFileContent{
		Type:         "authorized_user",
		AccessToken:  creds.AccessToken,
		TokenExpiry:  formatTokenExpiry(creds.TokenExpiry),
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	adcPath, err := WriteADCFile(realm, providerName, identityName, adcContent)
	if err != nil {
		return nil, fmt.Errorf("%w: write ADC file: %w", errUtils.ErrWriteADCFile, err)
	}
	paths = append(paths, adcPath)

	// Properties file (project/region).
	projectID := creds.ProjectID
	region := ""
	propsPath, err := WritePropertiesFile(realm, providerName, identityName, projectID, region)
	if err != nil {
		return nil, fmt.Errorf("%w: write properties file: %w", errUtils.ErrWritePropertiesFile, err)
	}
	paths = append(paths, propsPath)

	// Access token file.
	tokenPath, err := WriteAccessTokenFile(realm, providerName, identityName, creds.AccessToken, creds.TokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("%w: write access token file: %w", errUtils.ErrWriteAccessTokenFile, err)
	}
	paths = append(paths, tokenPath)

	return paths, nil
}

func formatTokenExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

type adcClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func resolveADCClientCredentials() (string, string, error) {
	clientID := strings.TrimSpace(os.Getenv("ATMOS_GCP_ADC_CLIENT_ID"))
	if clientID == "" {
		clientID = "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com"
	}

	clientSecret := strings.TrimSpace(os.Getenv("ATMOS_GCP_ADC_CLIENT_SECRET"))
	if clientSecret != "" {
		return clientID, clientSecret, nil
	}

	adcCreds, err := readADCClientCredentials()
	if err != nil {
		return "", "", err
	}
	if adcCreds.ClientID != "" {
		clientID = adcCreds.ClientID
	}
	if adcCreds.ClientSecret == "" {
		return "", "", fmt.Errorf("%w: ADC client secret is missing; run `gcloud auth application-default login` or set ATMOS_GCP_ADC_CLIENT_SECRET", errUtils.ErrInvalidAuthConfig)
	}
	return clientID, adcCreds.ClientSecret, nil
}

func readADCClientCredentials() (*adcClientCredentials, error) {
	path := adcCredentialsPath()
	if path == "" {
		return nil, fmt.Errorf("%w: no ADC credentials found; run `gcloud auth application-default login` or set ATMOS_GCP_ADC_CLIENT_SECRET", errUtils.ErrInvalidAuthConfig)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: ADC credentials not found; run `gcloud auth application-default login` or set ATMOS_GCP_ADC_CLIENT_SECRET", errUtils.ErrInvalidAuthConfig)
		}
		return nil, fmt.Errorf("%w: read ADC credentials: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	var creds adcClientCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("%w: parse ADC credentials: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return &creds, nil
}

func adcCredentialsPath() string {
	if path := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); path != "" {
		return path
	}
	if configDir := strings.TrimSpace(os.Getenv("CLOUDSDK_CONFIG")); configDir != "" {
		return filepath.Join(configDir, "application_default_credentials.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

// SetAuthContext populates the GCPAuthContext in the given AuthContext
// with the credential information and file paths.
// Callers with ConfigAndStacksInfo should pass config.AuthContext.
// Realm is required for credential isolation.
func SetAuthContext(authContext *schema.AuthContext, realm, providerName, identityName string, creds *types.GCPCredentials) error {
	defer perf.Track(nil, "gcp.SetAuthContext")()

	if authContext == nil {
		return nil
	}
	if creds == nil {
		return fmt.Errorf("%w: GCP credentials cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	adcPath, err := GetADCFilePath(realm, providerName, identityName)
	if err != nil {
		return err
	}
	configDir, err := GetConfigDir(realm, providerName, identityName)
	if err != nil {
		return err
	}

	authContext.GCP = &schema.GCPAuthContext{
		ProjectID:           creds.ProjectID,
		ServiceAccountEmail: creds.ServiceAccountEmail,
		AccessToken:         creds.AccessToken,
		TokenExpiry:         creds.TokenExpiry,
		Region:              "",
		ConfigDir:           configDir,
		CredentialsFile:     adcPath,
	}
	return nil
}

// Setup performs the complete setup for a GCP identity:
// 1. Prepares the environment (clears conflicting vars)
// 2. Writes credential files
// 3. Sets environment variables (including project/region from creds)
//
// Call SetAuthContext separately with the stack's AuthContext so in-process
// and spawned processes use the same context.
// Realm is required for credential isolation.
func Setup(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	realm string,
	providerName string,
	identityName string,
	creds *types.GCPCredentials,
) error {
	defer perf.Track(nil, "gcp.Setup")()

	if err := PrepareEnvironment(ctx, atmosConfig); err != nil {
		return err
	}
	if _, err := SetupFiles(ctx, atmosConfig, realm, providerName, identityName, creds); err != nil {
		return err
	}
	// Set env vars with project/region from creds.
	gcpAuth := &schema.GCPAuthContext{
		ProjectID:   creds.ProjectID,
		Region:      "",
		AccessToken: creds.AccessToken,
	}
	adcPath, err := GetADCFilePath(realm, providerName, identityName)
	if err != nil {
		return err
	}
	configDir, err := GetConfigDir(realm, providerName, identityName)
	if err != nil {
		return err
	}
	gcpAuth.CredentialsFile = adcPath
	gcpAuth.ConfigDir = configDir
	return setEnvironmentVariablesFromAuth(ctx, realm, providerName, identityName, gcpAuth)
}

// Cleanup removes all credential files and clears environment variables
// for a GCP identity.
// Realm is required for credential isolation.
func Cleanup(ctx context.Context, atmosConfig *schema.AtmosConfiguration, realm, providerName, identityName string) error {
	defer perf.Track(nil, "gcp.Cleanup")()

	_ = ctx
	_ = atmosConfig
	if err := CleanupIdentityFiles(realm, providerName, identityName); err != nil {
		return err
	}
	// Clear GCP env vars for this process.
	for _, key := range GCPEnvironmentVariables {
		os.Unsetenv(key)
	}
	return nil
}

// LoadCredentialsFromFiles attempts to load existing GCP credentials from
// the credential files for an identity. Returns nil if no valid credentials exist.
// Realm is required for credential isolation.
func LoadCredentialsFromFiles(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	realm string,
	providerName string,
	identityName string,
) (*types.GCPCredentials, error) {
	defer perf.Track(nil, "gcp.LoadCredentialsFromFiles")()

	_ = ctx
	_ = atmosConfig
	adcPath, err := GetADCFilePath(realm, providerName, identityName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(adcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: read ADC file: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	var adc ADCFileContent
	if err := json.Unmarshal(data, &adc); err != nil {
		return nil, fmt.Errorf("%w: parse ADC file: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	if adc.AccessToken == "" {
		return nil, nil
	}
	var expiry time.Time
	if adc.TokenExpiry != "" {
		expiry, err = time.Parse(time.RFC3339, adc.TokenExpiry)
		if err != nil {
			return nil, fmt.Errorf("%w: parse ADC token expiry: %w", errUtils.ErrInvalidAuthConfig, err)
		}
	}
	creds := &types.GCPCredentials{
		AccessToken: adc.AccessToken,
		TokenExpiry: expiry,
		ProjectID:   "",
	}
	return creds, nil
}

// CredentialsExist checks if valid (non-expired) credentials exist for an identity.
// Realm is required for credential isolation.
func CredentialsExist(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	realm string,
	providerName string,
	identityName string,
) (bool, error) {
	defer perf.Track(nil, "gcp.CredentialsExist")()

	creds, err := LoadCredentialsFromFiles(ctx, atmosConfig, realm, providerName, identityName)
	if err != nil || creds == nil {
		return false, err
	}
	if creds.IsExpired() {
		return false, nil
	}
	return true, nil
}
