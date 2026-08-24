package gcp

import (
	"context"
	"os"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GCPEnvironmentVariables lists all GCP-related environment variables that Atmos manages.
var GCPEnvironmentVariables = []string{
	"GOOGLE_APPLICATION_CREDENTIALS",
	"GOOGLE_CLOUD_PROJECT",
	"GCLOUD_PROJECT",
	"CLOUDSDK_CORE_PROJECT",
	"CLOUDSDK_CONFIG",
	"CLOUDSDK_ACTIVE_CONFIG_NAME",
	"GOOGLE_OAUTH_ACCESS_TOKEN",
	"GOOGLE_CLOUD_REGION",
	"CLOUDSDK_COMPUTE_REGION",
	"GOOGLE_CLOUD_ZONE",
	"CLOUDSDK_COMPUTE_ZONE",
}

// PrepareEnvironment clears GCP-related environment variables to ensure
// a clean state before setting up isolated credentials.
// This prevents conflicts between user's existing gcloud config and Atmos-managed credentials.
func PrepareEnvironment() error {
	defer perf.Track(nil, "gcp.PrepareEnvironment")()

	for _, key := range GCPEnvironmentVariables {
		if _, ok := os.LookupEnv(key); ok {
			log.Debug("Clearing GCP environment variable", "key", key)
			os.Unsetenv(key)
		}
	}
	return nil
}

// GetCurrentGCPEnvironment returns the current GCP-related environment variables from the process.
// Call before PrepareEnvironment and pass the result to RestoreEnvironment on cleanup.
func GetCurrentGCPEnvironment() map[string]string {
	defer perf.Track(nil, "gcp.GetCurrentGCPEnvironment")()

	out := make(map[string]string, len(GCPEnvironmentVariables))
	for _, key := range GCPEnvironmentVariables {
		if v, ok := os.LookupEnv(key); ok {
			out[key] = v
		}
	}
	return out
}

// SetEnvironmentVariables sets the GCP environment variables based on the
// identity configuration and credential file paths.
// Realm is required for credential isolation.
func SetEnvironmentVariables(ctx context.Context, realm, providerName, identityName string) error {
	defer perf.Track(nil, "gcp.SetEnvironmentVariables")()

	return setEnvironmentVariablesFromAuth(ctx, realm, providerName, identityName, nil)
}

// SetEnvironmentVariablesFromStackInfo sets GCP environment variables using
// paths for identityName and project/region from stackInfo.AuthContext.GCP when present.
// Call this after SetAuthContext so project/region are applied.
// Realm is required for credential isolation.
func SetEnvironmentVariablesFromStackInfo(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, realm, providerName, identityName string) error {
	defer perf.Track(nil, "gcp.SetEnvironmentVariablesFromStackInfo")()

	var gcpAuth *schema.GCPAuthContext
	if stackInfo != nil && stackInfo.AuthContext != nil {
		gcpAuth = stackInfo.AuthContext.GCP
	}
	return setEnvironmentVariablesFromAuth(ctx, realm, providerName, identityName, gcpAuth)
}

func setEnvironmentVariablesFromAuth(ctx context.Context, realm, providerName, identityName string, gcpAuth *schema.GCPAuthContext) error {
	_ = ctx // Context is reserved for future cancellation support.
	env, err := GetEnvironmentVariablesForIdentity(realm, providerName, identityName, gcpAuth)
	if err != nil {
		return err
	}
	for k, v := range env {
		os.Setenv(k, v)
	}
	return nil
}

// GetEnvironmentVariables returns a map of environment variables that should be set
// for the given identity, without actually setting them.
// authContext is optional; when authContext.GCP is set, project/region are included.
// Realm is required for credential isolation.
func GetEnvironmentVariables(realm, providerName, identityName string) (map[string]string, error) {
	defer perf.Track(nil, "gcp.GetEnvironmentVariables")()

	var gcpAuth *schema.GCPAuthContext
	return GetEnvironmentVariablesForIdentity(realm, providerName, identityName, gcpAuth)
}

// ProjectEnvVars returns project-related environment variables.
func ProjectEnvVars(projectID string) map[string]string {
	if projectID == "" {
		return map[string]string{}
	}
	return map[string]string{
		"GOOGLE_CLOUD_PROJECT":  projectID,
		"GCLOUD_PROJECT":        projectID,
		"CLOUDSDK_CORE_PROJECT": projectID,
	}
}

// RegionEnvVars returns region-related environment variables.
func RegionEnvVars(region string) map[string]string {
	if region == "" {
		return map[string]string{}
	}
	return map[string]string{
		"GOOGLE_CLOUD_REGION":     region,
		"CLOUDSDK_COMPUTE_REGION": region,
	}
}

// ZoneEnvVars returns zone-related environment variables.
func ZoneEnvVars(zone string) map[string]string {
	if zone == "" {
		return map[string]string{}
	}
	return map[string]string{
		"GOOGLE_CLOUD_ZONE":     zone,
		"CLOUDSDK_COMPUTE_ZONE": zone,
	}
}

// GetEnvironmentVariablesForIdentity returns the env map for an identity.
// gcpAuth may be nil; when set, project/region/credentials path from auth are used.
// Realm is required for credential isolation.
//
// When an access token is available, we use GOOGLE_OAUTH_ACCESS_TOKEN instead of
// GOOGLE_APPLICATION_CREDENTIALS. This is because our ADC files don't have refresh
// tokens (we get access tokens via service account impersonation), and the Google
// Cloud SDK's authorized_user credential type requires a refresh token.
func GetEnvironmentVariablesForIdentity(realm, providerName, identityName string, gcpAuth *schema.GCPAuthContext) (map[string]string, error) {
	env := make(map[string]string)

	configDir, err := GetConfigDir(realm, providerName, identityName)
	if err != nil {
		return nil, err
	}

	env["CLOUDSDK_CONFIG"] = configDir
	env["CLOUDSDK_ACTIVE_CONFIG_NAME"] = "config_atmos"

	// Determine if we have an access token available.
	hasAccessToken := gcpAuth != nil && gcpAuth.AccessToken != ""

	if hasAccessToken {
		// When we have an access token (from service account impersonation),
		// use GOOGLE_OAUTH_ACCESS_TOKEN directly. Don't set GOOGLE_APPLICATION_CREDENTIALS
		// because our ADC file format (authorized_user without refresh_token) causes
		// Google Cloud SDK to fail with "refresh token must be provided".
		env["GOOGLE_OAUTH_ACCESS_TOKEN"] = gcpAuth.AccessToken
	} else {
		// No access token available, fall back to file-based credentials.
		// If CredentialsFile is explicitly set in gcpAuth, use it; otherwise use default path.
		if gcpAuth != nil && gcpAuth.CredentialsFile != "" {
			env["GOOGLE_APPLICATION_CREDENTIALS"] = gcpAuth.CredentialsFile
		} else {
			adcPath, err := GetADCFilePath(realm, providerName, identityName)
			if err != nil {
				return nil, err
			}
			env["GOOGLE_APPLICATION_CREDENTIALS"] = adcPath
		}
	}

	if gcpAuth != nil {
		if gcpAuth.ConfigDir != "" {
			env["CLOUDSDK_CONFIG"] = gcpAuth.ConfigDir
		}
		for key, value := range ProjectEnvVars(gcpAuth.ProjectID) {
			env[key] = value
		}
		for key, value := range RegionEnvVars(gcpAuth.Region) {
			env[key] = value
		}
		for key, value := range ZoneEnvVars(gcpAuth.Location) {
			env[key] = value
		}
	}

	return env, nil
}

// RestoreEnvironment restores the original GCP environment variables.
// This is used during logout or cleanup.
func RestoreEnvironment(savedEnv map[string]string) error {
	defer perf.Track(nil, "gcp.RestoreEnvironment")()

	for _, key := range GCPEnvironmentVariables {
		os.Unsetenv(key)
	}
	for k, v := range savedEnv {
		os.Setenv(k, v)
	}
	return nil
}
