package auth

import (
	"context"
	"fmt"
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

const hookOpTerraformPreHook = "TerraformPreHook"

// TerraformPreHook runs before Terraform commands to set up authentication.
func TerraformPreHook(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	if stackInfo == nil {
		return errUtils.Build(errUtils.ErrInvalidAuthConfig).
			WithExplanation("Stack info is nil - this is an internal error").
			WithHint("Please report this issue at https://github.com/cloudposse/atmos/issues").
			Err()
	}
	if atmosConfig == nil {
		return errUtils.Build(errUtils.ErrInvalidAuthConfig).
			WithExplanation("Atmos configuration is nil - this is an internal error").
			WithHint("Please report this issue at https://github.com/cloudposse/atmos/issues").
			Err()
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

	authManager, err := newAuthManager(&authConfig, stackInfo)
	if err != nil {
		return errUtils.Build(errUtils.ErrAuthManager).
			WithCause(err).
			WithExplanation("Failed to create auth manager").
			WithHint("Check your auth configuration in atmos.yaml").
			WithContext("profile", FormatProfile(stackInfo.ProfilesFromArg)).
			Err()
	}

	// Determine target identity and authenticate.
	targetIdentityName, err := resolveTargetIdentityName(stackInfo, authManager)
	if err != nil {
		return err
	}

	if err := authenticateAndWriteEnv(context.Background(), authManager, targetIdentityName, atmosConfig, stackInfo); err != nil {
		return err
	}
	return nil
}

func decodeAuthConfigFromStack(stackInfo *schema.ConfigAndStacksInfo) (schema.AuthConfig, error) {
	var authConfig schema.AuthConfig
	if err := mapstructure.Decode(stackInfo.ComponentAuthSection, &authConfig); err != nil {
		return schema.AuthConfig{}, errUtils.Build(errUtils.ErrInvalidAuthConfig).
			WithCause(err).
			WithExplanation("Failed to decode component auth config").
			WithHint("Check your auth configuration in atmos.yaml or component auth section").
			WithContext("profile", FormatProfile(stackInfo.ProfilesFromArg)).
			Err()
	}
	return authConfig, nil
}

func resolveTargetIdentityName(stackInfo *schema.ConfigAndStacksInfo, authManager types.AuthManager) (string, error) {
	if stackInfo.Identity != "" {
		return stackInfo.Identity, nil
	}
	// Hooks don't have CLI flags, so never force selection here.
	name, err := authManager.GetDefaultIdentity(false)
	if err != nil {
		// Return error directly - it already has ErrorBuilder context with hints.
		return "", err
	}
	if name == "" {
		return "", errUtils.Build(errUtils.ErrNoDefaultIdentity).
			WithExplanation("No default identity is configured for authentication").
			WithHint("Use --identity flag to specify an identity").
			WithHint("Or set default: true on an identity in your auth configuration").
			Err()
	}
	return name, nil
}

// isAuthenticationDisabled checks if authentication has been explicitly disabled.
func isAuthenticationDisabled(identityName string) bool {
	return identityName == cfg.IdentityFlagDisabledValue
}

func authenticateAndWriteEnv(ctx context.Context, authManager types.AuthManager, identityName string, atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	log.Debug("Authenticating with identity", "identity", identityName)
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		// Return error directly - it already has ErrorBuilder context.
		return err
	}
	log.Debug("Authentication successful", "identity", whoami.Identity, "expiration", whoami.Expiration)

	// Convert ComponentEnvSection to env list for PrepareShellEnvironment.
	// This includes any component-specific env vars already set in the stack config.
	baseEnvList := componentEnvSectionToList(stackInfo.ComponentEnvSection)

	// Prepare shell environment with auth credentials.
	// This configures file-based credentials (AWS_SHARED_CREDENTIALS_FILE, AWS_PROFILE, etc.).
	envList, err := authManager.PrepareShellEnvironment(ctx, identityName, baseEnvList)
	if err != nil {
		// Return error directly - it already has ErrorBuilder context.
		return err
	}

	// Convert back to ComponentEnvSection map for downstream processing.
	if stackInfo.ComponentEnvSection == nil {
		stackInfo.ComponentEnvSection = make(map[string]any)
	}
	for _, envVar := range envList {
		if idx := strings.IndexByte(envVar, '='); idx >= 0 {
			key := envVar[:idx]
			value := envVar[idx+1:]
			stackInfo.ComponentEnvSection[key] = value
		}
	}

	if err := utils.PrintAsYAMLToFileDescriptor(atmosConfig, stackInfo.ComponentEnvSection); err != nil {
		return errUtils.Build(errUtils.ErrAuthManager).
			WithCause(err).
			WithExplanation("Failed to print component env section").
			WithHint("This is an internal error - please report at https://github.com/cloudposse/atmos/issues").
			WithContext("profile", FormatProfile(stackInfo.ProfilesFromArg)).
			WithContext("identity", identityName).
			Err()
	}
	return nil
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

func newAuthManager(authConfig *schema.AuthConfig, stackInfo *schema.ConfigAndStacksInfo) (types.AuthManager, error) {
	// Create auth manager components.
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	// Create auth manager with merged configuration and stack info (so identities can mutate it).
	authManager, err := NewAuthManager(
		authConfig,
		credStore,
		validator,
		stackInfo,
	)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAuthManager).
			WithCause(err).
			WithExplanation("Failed to create auth manager").
			WithHint("Check your auth configuration in atmos.yaml").
			WithContext("profile", FormatProfile(stackInfo.ProfilesFromArg)).
			Err()
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
