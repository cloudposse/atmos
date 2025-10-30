package auth

import (
	"context"
	"fmt"

	charm "github.com/charmbracelet/log"
	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
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

	authConfig, err := decodeAuthConfigFromStack(stackInfo)
	if err != nil {
		return err
	}

	// Skip if no auth config (check the merged config, not the original).
	if len(authConfig.Providers) == 0 && len(authConfig.Identities) == 0 {
		log.Debug("No auth configuration found, skipping authentication")
		return nil
	}

	authManager, err := newAuthManager(&authConfig, stackInfo)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrAuthManager, hookOpTerraformPreHook, "failed to create auth manager")
		return errUtils.ErrAuthManager
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
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, hookOpTerraformPreHook, "failed to decode component auth config - check atmos.yaml or component auth section")
		return schema.AuthConfig{}, errUtils.ErrInvalidAuthConfig
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
		errUtils.CheckErrorAndPrint(errUtils.ErrDefaultIdentity, hookOpTerraformPreHook, "failed to get default identity")
		return "", errUtils.ErrDefaultIdentity
	}
	if name == "" {
		errUtils.CheckErrorAndPrint(errUtils.ErrNoDefaultIdentity, hookOpTerraformPreHook, "Use the identity flag or specify an identity as default.")
		return "", errUtils.ErrNoDefaultIdentity
	}
	return name, nil
}

func authenticateAndWriteEnv(ctx context.Context, authManager types.AuthManager, identityName string, atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	log.Info("Authenticating with identity", "identity", identityName)
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return fmt.Errorf("failed to authenticate with identity %q: %w", identityName, err)
	}
	log.Debug("Authentication successful", "identity", whoami.Identity, "expiration", whoami.Expiration)

	// Get environment variables from the identity and add to ComponentEnvSection.
	// This is provider-agnostic and works for AWS, Azure, GCP, GitHub, etc.
	envVars, err := authManager.GetEnvironmentVariables(identityName)
	if err != nil {
		return fmt.Errorf("failed to get environment variables: %w", err)
	}

	// Add auth environment variables to ComponentEnvSection which gets passed to Terraform/workflows.
	if stackInfo.ComponentEnvSection == nil {
		stackInfo.ComponentEnvSection = make(map[string]any)
	}
	for k, v := range envVars {
		stackInfo.ComponentEnvSection[k] = v
	}

	if err := utils.PrintAsYAMLToFileDescriptor(atmosConfig, stackInfo.ComponentEnvSection); err != nil {
		return fmt.Errorf("failed to print component env section: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("%v: failed to create auth manager: %w", errUtils.ErrAuthManager, err)
	}
	return authManager, nil
}

func getConfigLogLevels(atmosConfig *schema.AtmosConfiguration) (charm.Level, charm.Level) {
	if atmosConfig == nil {
		return charm.InfoLevel, charm.InfoLevel
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
			// Convert Atmos LogLevel string to charm.Level.
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
