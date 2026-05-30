package ecr

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// loginCmd logs in to AWS ECR registries.
var loginCmd = &cobra.Command{
	Use:   "login [integration]",
	Short: "Login to AWS ECR registries",
	Long: `Login to AWS ECR registries using a named integration or identity.

By default, uses a named integration's ECR config. Use --identity to specify
a different identity whose linked integrations should be executed. Use --registry
to override with explicit registry URLs. Use --public to log in to ECR Public
(public.ecr.aws) using ambient AWS credentials, or pair it with --identity.

Examples:
  # Login using a named integration
  atmos aws ecr login dev/ecr

  # Login using an identity's linked integrations
  atmos aws ecr login --identity dev-admin

  # Login to ECR Public using ambient AWS credentials (zero-config)
  atmos aws ecr login --public

  # Login to ECR Public using a specific identity
  atmos aws ecr login --public --identity dev-admin

  # Override with explicit registry URL
  atmos aws ecr login --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.MaximumNArgs(1),
	RunE:               executeLoginCommand,
}

func executeLoginCommand(cmd *cobra.Command, args []string) error {
	// Handle positional "help" argument (e.g., "atmos aws ecr login help").
	if len(args) > 0 && args[0] == "help" {
		return cmd.Help()
	}

	ctx := context.Background()

	// Get flag values (errors are ignored as flags are guaranteed to exist by Cobra).
	identityName, _ := cmd.Flags().GetString("identity")
	registries, _ := cmd.Flags().GetStringArray("registry")
	public, _ := cmd.Flags().GetBool("public")

	// Get integration name from positional argument.
	var integrationName string
	if len(args) > 0 {
		integrationName = args[0]
	}

	// Reject mutually exclusive modes before doing any work.
	if err := validateLoginModes(public, integrationName, registries); err != nil {
		return err
	}

	// Case 1: Explicit registries — no Atmos config needed, uses ambient AWS credentials.
	if len(registries) > 0 {
		return executeExplicitRegistries(ctx, registries)
	}

	// Case 2: ECR Public with ambient credentials — no Atmos config needed.
	if public && identityName == "" {
		return executePublicLoginAmbient(ctx)
	}

	// Remaining cases require Atmos config for the auth manager.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}
	defer perf.Track(&atmosConfig, "aws.ecr.executeLoginCommand")()

	// Case 3: ECR Public via a specific identity.
	if public {
		return executePublicLoginWithIdentity(ctx, &atmosConfig, identityName)
	}

	// Case 4: Named integration or identity's linked integrations.
	return executeWithAuthManager(ctx, &atmosConfig, identityName, integrationName)
}

// validateLoginModes rejects mutually exclusive login modes.
func validateLoginModes(public bool, integrationName string, registries []string) error {
	if !public {
		return nil
	}
	if integrationName != "" {
		return fmt.Errorf("%w: --public cannot be combined with an integration argument", errUtils.ErrMutuallyExclusiveFlags)
	}
	if len(registries) > 0 {
		return fmt.Errorf("%w: --public cannot be combined with --registry", errUtils.ErrMutuallyExclusiveFlags)
	}
	return nil
}

// executePublicLoginAmbient logs in to ECR Public using ambient AWS credentials
// (the AWS SDK default credential chain: env vars, shared config, SSO, IMDS/IRSA/ECS).
func executePublicLoginAmbient(ctx context.Context) error {
	defer perf.Track(nil, "aws.ecr.executePublicLoginAmbient")()

	creds, err := awsCloud.LoadDefaultAWSCredentials(ctx)
	if err != nil {
		return err
	}

	return publicLogin(ctx, creds)
}

// executePublicLoginWithIdentity authenticates the given identity, then logs in to
// ECR Public using that identity's credentials.
func executePublicLoginWithIdentity(ctx context.Context, atmosConfig *schema.AtmosConfiguration, identityName string) error {
	defer perf.Track(atmosConfig, "aws.ecr.executePublicLoginWithIdentity")()

	// Reject the interactive selection sentinel; this command requires an explicit name.
	if identityName == cfg.IdentityFlagSelectValue {
		return errUtils.ErrECRIdentitySelect
	}

	authManager, err := createAuthManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, err)
	}
	if whoami.Credentials == nil {
		return fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, errUtils.ErrIdentityCredentialsNone)
	}

	return publicLogin(ctx, whoami.Credentials)
}

// publicLogin exchanges AWS credentials for an ECR Public authorization token and
// writes the resulting Docker credentials for public.ecr.aws.
func publicLogin(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "aws.ecr.publicLogin")()

	result, err := awsCloud.GetPublicAuthorizationToken(ctx, creds)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrECRPublicAuthFailed, err)
	}

	dockerConfig, err := docker.NewConfigManager()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrIntegrationFailed, err)
	}

	if err := dockerConfig.WriteAuth(awsCloud.ECRPublicRegistryURL, result.Username, result.Password); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
	}

	// Log success with actual expiration time from the ECR Public token.
	expiresIn := time.Until(result.ExpiresAt).Round(time.Minute)
	ui.Success(fmt.Sprintf("ECR Public login: %s (expires in %s)", awsCloud.ECRPublicRegistryURL, expiresIn))

	// Set DOCKER_CONFIG so downstream Docker commands use the same config location.
	_ = os.Setenv("DOCKER_CONFIG", dockerConfig.GetConfigDir())

	return nil
}

// executeWithAuthManager handles integration and identity-based ECR login modes.
func executeWithAuthManager(ctx context.Context, atmosConfig *schema.AtmosConfiguration, identityName, integrationName string) error {
	authManager, err := createAuthManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Reject ambiguous input: both integration name and --identity provided.
	if integrationName != "" && identityName != "" {
		return fmt.Errorf("%w: --identity cannot be combined with an integration argument", errUtils.ErrMutuallyExclusiveFlags)
	}

	// Case 2: Named integration.
	if integrationName != "" {
		return authManager.ExecuteIntegration(ctx, integrationName)
	}

	// Case 3: Identity's linked integrations.
	if identityName != "" {
		// Reject the interactive selection sentinel; this command requires an explicit name.
		if identityName == cfg.IdentityFlagSelectValue {
			return errUtils.ErrECRIdentitySelect
		}
		return authManager.ExecuteIdentityIntegrations(ctx, identityName)
	}

	// No arguments provided.
	return errUtils.ErrECRLoginNoArgs
}

// executeExplicitRegistries performs ECR login for explicit registry URLs.
// This uses the current AWS credentials from the environment (not Atmos identities).
func executeExplicitRegistries(ctx context.Context, registries []string) error {
	defer perf.Track(nil, "aws.ecr.executeExplicitRegistries")()

	// Load AWS credentials from environment.
	creds, err := awsCloud.LoadDefaultAWSCredentials(ctx)
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
		accountID, region, err := awsCloud.ParseRegistryURL(registry)
		if err != nil {
			return err // ParseRegistryURL already wraps with ErrECRInvalidRegistry.
		}

		result, err := awsCloud.GetAuthorizationToken(ctx, creds, accountID, region)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", errUtils.ErrECRLoginFailed, registry, err)
		}

		if err := dockerConfig.WriteAuth(registry, result.Username, result.Password); err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err)
		}

		// Log success with actual expiration time from ECR token.
		expiresIn := time.Until(result.ExpiresAt).Round(time.Minute)
		ui.Success(fmt.Sprintf("ECR login: %s (expires in %s)", registry, expiresIn))
	}

	// Set DOCKER_CONFIG so downstream Docker commands use the same config location.
	// This ensures Docker CLI and container tools find the ECR credentials.
	_ = os.Setenv("DOCKER_CONFIG", dockerConfig.GetConfigDir())

	return nil
}

// createAuthManager creates a new auth manager for ECR operations.
func createAuthManager(authConfig *schema.AuthConfig, cliConfigPath string) (auth.AuthManager, error) {
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	return auth.NewAuthManager(authConfig, credStore, validator, authStackInfo, cliConfigPath)
}

func init() {
	// Add --identity flag locally since this command is outside the auth command tree.
	loginCmd.Flags().StringP("identity", "i", "", "Specify the target identity to use for linked integrations.")

	// Set NoOptDefVal to enable optional flag value.
	// When --identity is used without a value, it will receive IdentityFlagSelectValue.
	identityFlag := loginCmd.Flags().Lookup("identity")
	if identityFlag != nil {
		identityFlag.NoOptDefVal = cfg.IdentityFlagSelectValue
	}

	// Add --registry as a local flag specific to login.
	loginCmd.Flags().StringArrayP("registry", "r", nil, "ECR registry URL(s) - explicit mode")

	// Add --public to log in to ECR Public (public.ecr.aws). Uses ambient AWS
	// credentials by default, or the credentials of --identity when provided.
	loginCmd.Flags().Bool("public", false, "Login to AWS ECR Public (public.ecr.aws)")

	EcrCmd.AddCommand(loginCmd)
}
