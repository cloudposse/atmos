package acr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// loginCmd logs in to Azure ACR registries.
var loginCmd = &cobra.Command{
	Use:   "login [integration]",
	Short: "Login to Azure Container Registry",
	Long: `Login to Azure Container Registry (ACR) using a named integration or identity.

By default, uses a named integration's ACR config. Use --identity to specify
a different identity whose linked integrations should be executed. Use --registry
to override with explicit registry login server URLs.

Examples:
  # Login using a named integration
  atmos azure acr login dev/acr

  # Login using an identity's linked integrations
  atmos azure acr login --identity dev-admin

  # Pick an identity interactively (requires a TTY)
  atmos azure acr login --identity

  # Override with explicit registry login server
  atmos azure acr login --registry myregistry.azurecr.io`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.MaximumNArgs(1),
	RunE:               executeLoginCommand,
}

// Testing seams: package-level indirection over the external Azure calls so
// tests can stub them. They default to the real implementations.
var (
	loadDefaultAzureCredentials = azureCloud.LoadDefaultAzureCredentials
	getAuthorizationToken       = azureCloud.GetAuthorizationToken
	loginParser                 *flags.StandardParser
)

func executeLoginCommand(cmd *cobra.Command, args []string) error {
	// Handle positional "help" argument (e.g., "atmos azure acr login help").
	if len(args) > 0 && args[0] == "help" {
		return cmd.Help()
	}

	ctx := context.Background()

	// Get flag values (errors are ignored as flags are guaranteed to exist by Cobra).
	identityName, _ := cmd.Flags().GetString("identity")
	registries, _ := cmd.Flags().GetStringSlice("registry")

	// Get integration name from positional argument.
	var integrationName string
	if len(args) > 0 {
		integrationName = args[0]
	}

	// Case 1: Explicit registries — no Atmos config needed, uses ambient Azure credentials.
	if len(registries) > 0 {
		if identityName != "" || integrationName != "" {
			return fmt.Errorf("%w: --registry cannot be combined with --identity or an integration argument", errUtils.ErrMutuallyExclusiveFlags)
		}
		return executeExplicitRegistries(ctx, registries)
	}

	// Remaining cases require Atmos config for the auth manager.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}
	defer perf.Track(&atmosConfig, "azure.acr.executeLoginCommand")()

	// Case 2: Named integration or identity's linked integrations.
	return executeWithAuthManager(ctx, &atmosConfig, identityName, integrationName)
}

// executeWithAuthManager handles integration and identity-based ACR login modes.
func executeWithAuthManager(ctx context.Context, atmosConfig *schema.AtmosConfiguration, identityName, integrationName string) error {
	authManager, err := createAuthManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Reject ambiguous input: both integration name and --identity provided.
	if integrationName != "" && identityName != "" {
		return fmt.Errorf("%w: --identity cannot be combined with an integration argument", errUtils.ErrMutuallyExclusiveFlags)
	}

	// Case: Named integration.
	if integrationName != "" {
		return authManager.ExecuteIntegration(ctx, integrationName)
	}

	// Case: Identity's linked integrations.
	if identityName != "" {
		// Resolve the interactive-selection sentinel to a concrete identity (prompts if needed).
		identityName, err = resolveSelectedIdentity(authManager, identityName)
		if err != nil {
			return err
		}
		return authManager.ExecuteIdentityIntegrations(ctx, identityName)
	}

	// No arguments provided.
	return errUtils.ErrACRLoginNoArgs
}

// executeExplicitRegistries performs ACR login for explicit registry login server URLs.
// This uses ambient Azure credentials from the environment (not Atmos identities).
func executeExplicitRegistries(ctx context.Context, registries []string) error {
	defer perf.Track(nil, "azure.acr.executeExplicitRegistries")()

	// Load Azure credentials from the ambient environment.
	creds, err := loadDefaultAzureCredentials(ctx)
	if err != nil {
		return err
	}

	// Create Docker config manager.
	dockerConfig, err := docker.NewConfigManager()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIntegrationFailed, err)
	}

	// Login to each registry.
	for _, registry := range registries {
		name, err := azureCloud.ParseRegistryURL(registry)
		if err != nil {
			return err // ParseRegistryURL already wraps with ErrACRInvalidRegistry.
		}
		loginServer := azureCloud.BuildRegistryURL(name)

		result, err := getAuthorizationToken(ctx, creds, loginServer)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", errUtils.ErrACRLoginFailed, loginServer, err)
		}

		if err := dockerConfig.WriteAuth(loginServer, result.Username, result.Password); err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
		}

		// Log success with actual expiration time, when known.
		if result.ExpiresAt.IsZero() {
			ui.Success(fmt.Sprintf("ACR login: %s", loginServer))
		} else {
			expiresIn := time.Until(result.ExpiresAt).Round(time.Minute)
			ui.Success(fmt.Sprintf("ACR login: %s (expires in %s)", loginServer, expiresIn))
		}
	}

	// Set DOCKER_CONFIG so downstream Docker commands use the same config location.
	// This ensures Docker CLI and container tools find the ACR credentials.
	_ = os.Setenv("DOCKER_CONFIG", dockerConfig.GetConfigDir())

	return nil
}

// resolveSelectedIdentity resolves the interactive-selection sentinel ("__SELECT__",
// produced when --identity is passed without a value) to a concrete identity by
// prompting the user. Concrete names pass through unchanged. In non-interactive
// contexts (no TTY / CI), GetDefaultIdentity returns ErrIdentitySelectionRequiresTTY.
func resolveSelectedIdentity(authManager auth.AuthManager, identityName string) (string, error) {
	defer perf.Track(nil, "azure.acr.resolveSelectedIdentity")()

	if identityName != cfg.IdentityFlagSelectValue {
		return identityName, nil
	}

	selected, err := authManager.GetDefaultIdentity(true)
	if err != nil {
		// User explicitly aborted (Ctrl+C/ESC) — surface a clean SIGINT exit.
		if errors.Is(err, errUtils.ErrUserAborted) {
			return "", errUtils.WithExitCode(err, errUtils.ExitCodeSIGINT)
		}
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDefaultIdentity, err)
	}
	return selected, nil
}

// createAuthManager creates a new auth manager for ACR operations.
// It is a package-level var so tests can inject a mock auth manager.
var createAuthManager = func(authConfig *schema.AuthConfig, cliConfigPath string) (auth.AuthManager, error) {
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStoreWithConfig(authConfig)
	validator := validation.NewValidator()
	return auth.NewAuthManager(authConfig, credStore, validator, authStackInfo, cliConfigPath)
}

func init() {
	// Register command-local flags through the standard parser so identity keeps
	// its optional-value behavior and flag definitions remain consistent.
	loginParser = flags.NewStandardParser(
		flags.WithIdentityFlag(),
		flags.WithStringSliceFlag("registry", "r", nil, "ACR registry login server URL(s) - explicit mode"),
	)
	loginParser.RegisterFlags(loginCmd)

	AcrCmd.AddCommand(loginCmd)
}
