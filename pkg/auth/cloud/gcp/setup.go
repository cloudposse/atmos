package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
//   - providerName: Name of the provider being set up.
//   - identityName: Name of the identity being set up.
//   - creds: The GCP credentials to write.
//
// Returns the paths of all written files and any error.
func SetupFiles(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
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
	clientID := os.Getenv("ATMOS_GCP_ADC_CLIENT_ID")
	if clientID == "" {
		clientID = "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com"
	}
	clientSecret := os.Getenv("ATMOS_GCP_ADC_CLIENT_SECRET")
	if clientSecret == "" {
		return nil, fmt.Errorf("%w: ATMOS_GCP_ADC_CLIENT_SECRET is required", errUtils.ErrInvalidAuthConfig)
	}
	adcContent := &ADCFileContent{
		Type:         "authorized_user",
		AccessToken:  creds.AccessToken,
		TokenExpiry:  formatTokenExpiry(creds.TokenExpiry),
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	adcPath, err := WriteADCFile(providerName, identityName, adcContent)
	if err != nil {
		return nil, fmt.Errorf("%w: write ADC file: %w", errUtils.ErrWriteADCFile, err)
	}
	paths = append(paths, adcPath)

	// Properties file (project/region).
	projectID := creds.ProjectID
	region := ""
	propsPath, err := WritePropertiesFile(providerName, identityName, projectID, region)
	if err != nil {
		return nil, fmt.Errorf("%w: write properties file: %w", errUtils.ErrWritePropertiesFile, err)
	}
	paths = append(paths, propsPath)

	// Access token file.
	tokenPath, err := WriteAccessTokenFile(providerName, identityName, creds.AccessToken, creds.TokenExpiry)
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

// SetAuthContext populates the GCPAuthContext in the given AuthContext
// with the credential information and file paths.
// Callers with ConfigAndStacksInfo should pass config.AuthContext.
func SetAuthContext(authContext *schema.AuthContext, providerName, identityName string, creds *types.GCPCredentials) error {
	defer perf.Track(nil, "gcp.SetAuthContext")()

	if authContext == nil {
		return nil
	}
	if creds == nil {
		return fmt.Errorf("%w: GCP credentials cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	adcPath, err := GetADCFilePath(providerName, identityName)
	if err != nil {
		return err
	}
	configDir, err := GetConfigDir(providerName, identityName)
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
func Setup(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	providerName string,
	identityName string,
	creds *types.GCPCredentials,
) error {
	defer perf.Track(nil, "gcp.Setup")()

	if err := PrepareEnvironment(ctx, atmosConfig); err != nil {
		return err
	}
	if _, err := SetupFiles(ctx, atmosConfig, providerName, identityName, creds); err != nil {
		return err
	}
	// Set env vars with project/region from creds.
	gcpAuth := &schema.GCPAuthContext{
		ProjectID:   creds.ProjectID,
		Region:      "",
		AccessToken: creds.AccessToken,
	}
	adcPath, err := GetADCFilePath(providerName, identityName)
	if err != nil {
		return err
	}
	configDir, err := GetConfigDir(providerName, identityName)
	if err != nil {
		return err
	}
	gcpAuth.CredentialsFile = adcPath
	gcpAuth.ConfigDir = configDir
	return setEnvironmentVariablesFromAuth(ctx, providerName, identityName, gcpAuth)
}

// Cleanup removes all credential files and clears environment variables
// for a GCP identity.
func Cleanup(ctx context.Context, atmosConfig *schema.AtmosConfiguration, providerName, identityName string) error {
	defer perf.Track(nil, "gcp.Cleanup")()

	_ = ctx
	_ = atmosConfig
	if err := CleanupIdentityFiles(providerName, identityName); err != nil {
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
func LoadCredentialsFromFiles(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	providerName string,
	identityName string,
) (*types.GCPCredentials, error) {
	defer perf.Track(nil, "gcp.LoadCredentialsFromFiles")()

	_ = ctx
	_ = atmosConfig
	adcPath, err := GetADCFilePath(providerName, identityName)
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
			expiry = time.Time{}
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
func CredentialsExist(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	providerName string,
	identityName string,
) (bool, error) {
	defer perf.Track(nil, "gcp.CredentialsExist")()

	creds, err := LoadCredentialsFromFiles(ctx, atmosConfig, providerName, identityName)
	if err != nil || creds == nil {
		return false, err
	}
	if creds.IsExpired() {
		return false, nil
	}
	return true, nil
}
