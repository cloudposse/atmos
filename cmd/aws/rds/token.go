package rds

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// rdsTokenParser handles flag parsing with Viper precedence for the rds token command.
var rdsTokenParser *flags.StandardParser

// initCliConfigFn loads Atmos CLI configuration. Overridable in tests.
var initCliConfigFn = cfg.InitCliConfig

// authenticateForTokenFn authenticates an identity and returns credentials. Overridable in tests.
var authenticateForTokenFn = authenticateForToken

// getRDSTokenFn generates an RDS IAM auth token. Overridable in tests.
var getRDSTokenFn = awsCloud.GetRDSToken

// tokenOptions holds the resolved inputs for token generation.
type tokenOptions struct {
	Host     string
	Port     int
	Username string
	Region   string
	Identity string
}

// tokenCmd generates a short-lived RDS IAM database authentication token.
var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Generate an RDS IAM database authentication token",
	Long: `Generate a short-lived RDS IAM database authentication token using an Atmos identity.

The token is a SigV4-signed value used as the database password over a TLS connection.
It is valid for approximately 15 minutes and is generated locally (no AWS API call).

The database, user, and IAM permissions must already be configured for IAM authentication
(EnableIAMDatabaseAuthentication on the instance, an rds-db:connect IAM policy, and the
in-database grant). This command only mints the token.

The token is a live credential: do not record this command with --cast/ATMOS_CAST, since the token
is written to stdout unmasked and would be captured verbatim into the recording.

Examples:
  # Generate a token and use it as the database password
  PGPASSWORD="$(atmos aws rds token --host mydb.abc123.us-east-2.rds.amazonaws.com \
    --port 5432 --username app --region us-east-2 --identity dev-admin)" \
    psql "host=mydb.abc123.us-east-2.rds.amazonaws.com sslmode=require dbname=app user=app"`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE:               executeTokenCommand,
	// Suppress usage on errors since the command is typically scripted.
	SilenceUsage: true,
}

// executeTokenCommand binds flags (with Viper precedence) and delegates to runRDSTokenGeneration.
func executeTokenCommand(cmd *cobra.Command, _ []string) error {
	v := viper.GetViper()
	if err := rdsTokenParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Keys are namespaced under the "rds" Viper prefix (see the parser's WithViperPrefix in init).
	return runRDSTokenGeneration(tokenOptions{
		Host:     v.GetString("rds.host"),
		Port:     v.GetInt("rds.port"),
		Username: v.GetString("rds.username"),
		Region:   v.GetString("rds.region"),
		Identity: v.GetString("rds.identity"),
	})
}

// runRDSTokenGeneration validates inputs, authenticates the identity, mints the token, and prints it.
func runRDSTokenGeneration(opts tokenOptions) error {
	defer perf.Track(nil, "rds.runRDSTokenGeneration")()

	if err := validateTokenOptions(opts); err != nil {
		return err
	}

	atmosConfig, err := initCliConfigFn(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// net.JoinHostPort brackets IPv6 literals correctly (fmt.Sprintf("%s:%d") does not).
	endpoint := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))

	log.Debug("Generating RDS IAM auth token", "endpoint", endpoint, "region", opts.Region, "identity", opts.Identity)

	// Skip integrations so token generation has no login-time side effects.
	ctx := auth.ContextWithSkipIntegrations(context.Background())

	creds, err := authenticateForTokenFn(ctx, &atmosConfig.Auth, atmosConfig.CliConfigPath, opts.Identity)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrRDSTokenGeneration, err)
	}

	token, expiresAt, err := getRDSTokenFn(ctx, creds, endpoint, opts.Region, opts.Username)
	if err != nil {
		return err
	}

	// The token IS the database password. Emit it UNMASKED: the default data.Write path masks
	// the embedded AKIA... access key ID, which would silently corrupt the token. This mirrors
	// how `atmos auth env` emits real credentials. The error is handled below, so no gosec G104
	// suppression is needed here.
	if err := data.WriteUnmasked(token); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrRDSTokenGeneration, err)
	}

	// The expiry is not sensitive; write it to stderr so stdout stays a clean token.
	ui.Info(fmt.Sprintf("RDS IAM authentication token generated for %s (expires %s).", endpoint, expiresAt.UTC().Format(time.RFC3339)))

	return nil
}

// validateTokenOptions ensures the required inputs are present and in range.
//
// --region is intentionally NOT required: GetRDSToken falls back to the authenticated identity's
// credential region when the flag is empty, so rejecting an empty region here would run before that
// fallback and break the documented contract. GetRDSToken errors clearly only when neither the flag
// nor the credentials supply a region.
func validateTokenOptions(opts tokenOptions) error {
	switch {
	case opts.Host == "":
		return fmt.Errorf("%w: --host is required", errUtils.ErrRDSTokenGeneration)
	case opts.Port == 0:
		return fmt.Errorf("%w: --port is required", errUtils.ErrRDSTokenGeneration)
	case opts.Port < 1 || opts.Port > 65535:
		return fmt.Errorf("%w: --port must be between 1 and 65535, got %d", errUtils.ErrRDSTokenGeneration, opts.Port)
	case opts.Username == "":
		return fmt.Errorf("%w: --username is required", errUtils.ErrRDSTokenGeneration)
	}
	return nil
}

// authenticateForToken authenticates an identity and returns credentials.
func authenticateForToken(ctx context.Context, authConfig *schema.AuthConfig, cliConfigPath, identityName string) (types.ICredentials, error) {
	defer perf.Track(nil, "rds.authenticateForToken")()

	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStoreWithConfig(authConfig)
	validator := validation.NewValidator()

	mgr, err := auth.NewAuthManager(authConfig, credStore, validator, authStackInfo, cliConfigPath)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Resolve which identity to authenticate (see resolveIdentityName).
	identityName, err = resolveIdentityName(mgr, authConfig, identityName)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrRDSTokenGeneration, err)
	}

	whoami, err := mgr.Authenticate(ctx, identityName)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, err)
	}

	if whoami.Credentials == nil {
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, errUtils.ErrIdentityCredentialsNone)
	}

	return whoami.Credentials, nil
}

// resolveIdentityName determines which identity to authenticate. An explicit name is used as-is;
// otherwise a single configured identity is used directly, and any other configuration defers to
// the canonical resolver (GetDefaultIdentity), which honors an identity marked default:true,
// disambiguates multiple defaults, and prompts on a TTY (erroring in CI). This deliberately does
// not reimplement the weak "single identity only" heuristic that silently ignored default:true.
func resolveIdentityName(mgr types.AuthManager, authConfig *schema.AuthConfig, identityName string) (string, error) {
	if identityName != "" {
		return identityName, nil
	}
	if authConfig != nil && len(authConfig.Identities) == 1 {
		for name := range authConfig.Identities {
			return name, nil
		}
	}
	return mgr.GetDefaultIdentity(false)
}

func init() {
	rdsTokenParser = flags.NewStandardParser(
		flags.WithStringFlag("host", "", "", "RDS/Aurora endpoint hostname (required)"),
		flags.WithIntFlag("port", "", 0, "Database port (required)"),
		flags.WithStringFlag("username", "u", "", "Database user name (required)"),
		flags.WithStringFlag("region", "", "", "AWS region of the database endpoint (optional; defaults to the identity's credential region)"),
		flags.WithStringFlag("identity", "i", "", "Atmos identity to authenticate with"),
		flags.WithEnvVars("host", "ATMOS_AWS_RDS_HOST"),
		flags.WithEnvVars("port", "ATMOS_AWS_RDS_PORT"),
		flags.WithEnvVars("username", "ATMOS_AWS_RDS_USERNAME"),
		flags.WithEnvVars("region", "ATMOS_AWS_RDS_REGION"),
		flags.WithEnvVars("identity", "ATMOS_IDENTITY"),
		// Namespace the Viper keys to rds.* so the bare "region"/"identity"/"host"/"port"/"username"
		// keys do not collide with the sibling `aws security`/`compliance`/`eks`/`workflow` commands
		// that bind the same bare keys (with different env vars) on the shared global Viper. Without
		// this, viper.BindEnv is last-writer-wins per key across nondeterministic package init order,
		// so e.g. ATMOS_AWS_SECURITY_REGION could satisfy this command's --region.
		flags.WithViperPrefix("rds"),
	)

	rdsTokenParser.RegisterFlags(tokenCmd)

	if err := rdsTokenParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	RdsCmd.AddCommand(tokenCmd)
}
