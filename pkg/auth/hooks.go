package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

type (
	Provider        = types.Provider
	Identity        = types.Identity
	AuthManager     = types.AuthManager
	CredentialStore = types.CredentialStore
	Validator       = types.Validator
)

const (
	hookOpTerraformPreHook = "TerraformPreHook"
	identityKey            = "identity"
)

// TerraformPreHook runs before Terraform commands to set up authentication.
func TerraformPreHook(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	if stackInfo == nil {
		return fmt.Errorf("%w: stack info is nil", errUtils.ErrInvalidAuthConfig)
	}
	if atmosConfig == nil {
		return fmt.Errorf("%w: atmos configuration is nil", errUtils.ErrInvalidAuthConfig)
	}

	atmosLevel, authLevel := getConfigLogLevels(atmosConfig)
	log.SetLevel(authLevel)
	defer log.SetLevel(atmosLevel)
	log.SetPrefix("atmos-auth")
	defer log.SetPrefix("")

	// Check if authentication has been explicitly disabled BEFORE doing any auth setup.
	if isAuthenticationDisabled(stackInfo.Identity) {
		log.Debug("Authentication explicitly disabled, skipping identity authentication")
		return nil
	}

	authConfig, err := decodeAuthConfigFromStack(stackInfo)
	if err != nil {
		return err
	}

	// Skip if no auth config (check the merged config, not the original).
	if len(authConfig.Providers) == 0 && len(authConfig.Identities) == 0 {
		return nil
	}

	authManager, err := newAuthManager(&authConfig, stackInfo, atmosConfig.CliConfigPath)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrAuthManager, hookOpTerraformPreHook, "failed to create auth manager")
		return errUtils.ErrAuthManager
	}

	// Determine target identity and authenticate.
	ctx := context.Background()
	targetIdentityName, err := resolveTargetIdentityName(ctx, stackInfo, authManager)
	if err != nil {
		return err
	}

	if err := authenticateAndWriteEnv(ctx, authManager, targetIdentityName, atmosConfig, stackInfo); err != nil {
		return err
	}
	return nil
}

func decodeAuthConfigFromStack(stackInfo *schema.ConfigAndStacksInfo) (schema.AuthConfig, error) {
	var authConfig schema.AuthConfig
	if err := mapstructure.Decode(stackInfo.ComponentAuthSection, &authConfig); err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, hookOpTerraformPreHook, "failed to decode component auth config - check atmos.yaml or component auth section")
		return schema.AuthConfig{}, errUtils.ErrInvalidAuthConfig
	}
	return authConfig, nil
}

func resolveTargetIdentityName(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, authManager types.AuthManager) (string, error) {
	// CLI --identity flag takes precedence.
	if stackInfo.Identity != "" {
		return stackInfo.Identity, nil
	}

	// Default identity from config is primary when available.
	name, err := authManager.GetDefaultIdentity(false)
	if err == nil && name != "" {
		return name, nil
	}
	if err != nil && !isNoAuthConfigError(err) {
		return "", err
	}

	// "No auth config in base" terminal state — offer a profile-switch
	// fallback before surfacing the fatal error. This mirrors the behavior
	// of the auth commands (login, exec, shell, env, console, whoami).
	// On successful interactive re-exec this call never returns. On an
	// enriched non-interactive error, surface it. On nil, fall through to
	// the original error path.
	if fbErr := authManager.MaybeOfferAnyProfileFallback(ctx); fbErr != nil {
		return "", fbErr
	}

	// No default identity found — error out.
	// The "required" field is about auto-authentication, not primary selection.
	errUtils.CheckErrorAndPrint(errUtils.ErrNoDefaultIdentity, hookOpTerraformPreHook, "Use the identity flag or specify an identity as default.")
	return "", errUtils.ErrNoDefaultIdentity
}

// isNoAuthConfigError reports whether err indicates a terminal state where
// the loaded configuration has no usable identities or providers — a state
// that switching to a different profile may recover from. Mirrors the
// dispatcher in cmd/auth_profile_fallback.go; duplicated here to avoid a
// package cycle (cmd imports pkg/auth, not vice versa).
func isNoAuthConfigError(err error) bool {
	return errors.Is(err, errUtils.ErrNoProvidersAvailable) ||
		errors.Is(err, errUtils.ErrNoIdentitiesAvailable) ||
		errors.Is(err, errUtils.ErrNoDefaultIdentity)
}

// isAuthenticationDisabled checks if authentication has been explicitly disabled.
func isAuthenticationDisabled(identityName string) bool {
	return identityName == cfg.IdentityFlagDisabledValue
}

func authenticateAndWriteEnv(ctx context.Context, authManager types.AuthManager, identityName string, atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	log.Debug("Authenticating with identity", "identity", identityName)
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return fmt.Errorf("failed to authenticate with identity %q: %w", identityName, err)
	}
	log.Debug("Authentication successful", "identity", whoami.Identity, "expiration", whoami.Expiration)

	// Authenticate additional required identities so their profiles exist in the shared credentials file.
	// This is needed for Terraform components that use multiple AWS provider aliases.
	authenticateAdditionalIdentities(ctx, authManager, identityName)

	// Build base env: os.Environ() + existing stack env vars from ComponentEnvSection.
	// Including os.Environ() ensures PrepareShellEnvironment can delete problematic vars
	// (e.g., IRSA credentials injected by EKS pod identity webhook) from the full
	// process environment, producing a sanitized base for subprocess execution.
	baseEnvList := mergeOsEnvironWithSection(os.Environ(), stackInfo.ComponentEnvSection)

	// Prepare shell environment with auth credentials.
	// This configures file-based credentials (AWS_SHARED_CREDENTIALS_FILE, AWS_PROFILE, etc.)
	// and removes problematic credential vars (IRSA, direct credentials).
	envList, err := authManager.PrepareShellEnvironment(ctx, identityName, baseEnvList)
	if err != nil {
		return fmt.Errorf("failed to prepare environment variables: %w", err)
	}

	// Store sanitized environment as the base for subprocess execution.
	// ExecuteShellCommand will use this instead of re-reading os.Environ(),
	// which would reintroduce the problematic vars that were just removed.
	stackInfo.SanitizedEnv = envList

	// Extract auth-specific vars back to ComponentEnvSection for logging/display
	// and downstream processing (e.g., terraform.go adds ATMOS-specific vars).
	if stackInfo.ComponentEnvSection == nil {
		stackInfo.ComponentEnvSection = make(map[string]any)
	}
	for _, envVar := range envList {
		if idx := strings.IndexByte(envVar, '='); idx >= 0 {
			key := envVar[:idx]
			value := envVar[idx+1:]
			// Only write auth-managed vars to ComponentEnvSection, not the full os env.
			if isAuthManagedVar(key) {
				stackInfo.ComponentEnvSection[key] = value
			}
		}
	}

	if err := utils.PrintAsYAMLToFileDescriptor(atmosConfig, stackInfo.ComponentEnvSection); err != nil {
		return fmt.Errorf("failed to print component env section: %w", err)
	}
	return nil
}

// mergeOsEnvironWithSection merges os.Environ() with ComponentEnvSection into a single env list.
// ComponentEnvSection values override os.Environ() values for the same key.
func mergeOsEnvironWithSection(osEnviron []string, envSection map[string]any) []string {
	// Start with os.Environ() as base.
	result := make([]string, 0, len(osEnviron)+len(envSection))
	result = append(result, osEnviron...)

	// Append ComponentEnvSection entries (override os.Environ() via last-occurrence-wins).
	for k, v := range envSection {
		if v != nil {
			result = append(result, fmt.Sprintf("%s=%v", k, v))
		}
	}

	return result
}

// isAuthManagedVar returns true if the env var key is managed by the auth system.
// These are the vars that PrepareEnvironment sets for Atmos-managed credentials.
func isAuthManagedVar(key string) bool {
	switch key {
	case "AWS_SHARED_CREDENTIALS_FILE",
		"AWS_CONFIG_FILE",
		"AWS_PROFILE",
		"AWS_SDK_LOAD_CONFIG",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
		"AWS_EC2_METADATA_DISABLED",
		// Azure auth-managed vars.
		"AZURE_CONFIG_DIR",
		"AZURE_SUBSCRIPTION_ID",
		"ARM_SUBSCRIPTION_ID",
		"ARM_TENANT_ID",
		"ARM_CLIENT_ID",
		"ARM_CLIENT_SECRET",
		"ARM_USE_OIDC",
		"ARM_OIDC_TOKEN",
		// GCP auth-managed vars.
		"GOOGLE_APPLICATION_CREDENTIALS",
		"CLOUDSDK_CONFIG",
		"GOOGLE_CLOUD_PROJECT",
		"GCLOUD_PROJECT",
		"CLOUDSDK_CORE_PROJECT":
		return true
	default:
		return false
	}
}

// authenticateAdditionalIdentities authenticates non-primary identities marked as required.
// Failures are non-fatal: errors are logged as warnings but don't fail the hook.
// This ensures all required profiles exist in the shared credentials file for Terraform
// components that use multiple AWS provider aliases (e.g., hub-spoke networking).
//
// TODO: Azure credentials are keyed by provider name, not identity. If two identities
// share the same Azure provider name, the second will overwrite the first. AWS merges
// profiles into INI sections and GCP isolates by directory, so they handle this correctly.
// Consider adopting a per-identity storage strategy for Azure if multi-identity Azure
// support becomes a requirement.
func authenticateAdditionalIdentities(ctx context.Context, authManager types.AuthManager,
	primaryIdentity string,
) {
	for name, identity := range authManager.GetIdentities() {
		if !identity.Required || strings.EqualFold(name, primaryIdentity) {
			continue
		}
		log.Debug("Authenticating additional required identity", identityKey, name)
		if _, err := authManager.Authenticate(ctx, name); err != nil {
			log.Warn("Failed to authenticate additional identity (non-fatal)",
				identityKey, name, "error", err)
		}
	}
}

// componentEnvSectionToList converts ComponentEnvSection map to environment variable list.
func componentEnvSectionToList(envSection map[string]any) []string {
	var envList []string
	for k, v := range envSection {
		if v != nil {
			envList = append(envList, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return envList
}

func newAuthManager(authConfig *schema.AuthConfig, stackInfo *schema.ConfigAndStacksInfo, cliConfigPath string) (types.AuthManager, error) {
	// Create auth manager components.
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	// Create auth manager with merged configuration and stack info (so identities can mutate it).
	authManager, err := NewAuthManager(
		authConfig,
		credStore,
		validator,
		stackInfo,
		cliConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("%v: failed to create auth manager: %w", errUtils.ErrAuthManager, err)
	}
	return authManager, nil
}

func getConfigLogLevels(atmosConfig *schema.AtmosConfiguration) (log.Level, log.Level) {
	if atmosConfig == nil {
		return log.InfoLevel, log.InfoLevel
	}
	// Get the current atmos log level that was already set by setupLogger in root.go.
	// This respects ATMOS_LOGS_LEVEL env var and --logs-level flag with case-insensitive parsing.
	atmosLevel := log.GetLevel()

	// Determine auth log level (fallback to atmos level).
	authLevel := atmosLevel
	if atmosConfig.Auth.Logs.Level != "" {
		// Parse the auth log level using Atmos' ParseLogLevel for case-insensitive parsing.
		// This ensures "Warning", "warning", "WARN", "warn" all work correctly.
		if atmosLogLevel, err := log.ParseLogLevel(atmosConfig.Auth.Logs.Level); err == nil {
			// Convert Atmos LogLevel string to log.Level.
			switch atmosLogLevel {
			case log.LogLevelTrace:
				authLevel = log.TraceLevel
			case log.LogLevelDebug:
				authLevel = log.DebugLevel
			case log.LogLevelInfo:
				authLevel = log.InfoLevel
			case log.LogLevelWarning:
				authLevel = log.WarnLevel
			case log.LogLevelError:
				authLevel = log.ErrorLevel
			case log.LogLevelOff:
				authLevel = log.FatalLevel
			}
		}
	}
	return atmosLevel, authLevel
}
