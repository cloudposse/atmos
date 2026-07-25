package rds

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/cacerts"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/shell"
	"github.com/cloudposse/atmos/pkg/ui"
)

// rdsConnectParser handles flag parsing with Viper precedence for the rds connect command.
var rdsConnectParser *flags.StandardParser

// runClientFn executes the resolved database client (psql/mysql). Overridable in tests.
// It sets the child's env verbatim and inherits the terminal, so the token is passed via env only.
var runClientFn = shell.RunCommand

// findCABundleFn locates the host's system CA bundle. Overridable in tests.
var findCABundleFn = cacerts.Find

// connectOptions holds the resolved inputs for a connect invocation.
type connectOptions struct {
	Integration  string
	Host         string
	Port         int
	Username     string
	Region       string
	Identity     string
	Engine       string
	Database     string
	CABundle     string
	PrintCommand bool
}

// engineSpec describes how to invoke a database client for a supported engine.
type engineSpec struct {
	name        string
	bin         string
	passwordEnv string
	defaultPort int
}

// engineSpecs is the closed set of supported database engines. Mirrors the flat table-dispatch
// idiom of pkg/store's storeBuilders rather than the open mutex+Register integration registry.
var engineSpecs = map[string]engineSpec{
	"postgres": {name: "postgres", bin: "psql", passwordEnv: "PGPASSWORD", defaultPort: 5432},
	"mysql":    {name: "mysql", bin: "mysql", passwordEnv: "MYSQL_PWD", defaultPort: 3306},
}

// connectCmd opens an interactive psql/mysql session using a freshly minted RDS IAM token.
var connectCmd = &cobra.Command{
	Use:   "connect [integration]",
	Short: "Connect to an RDS/Aurora database using an IAM authentication token",
	Long: `Open an interactive psql/mysql session against an Amazon RDS or Aurora database using an
Atmos identity and a freshly minted RDS IAM authentication token — no static password, no AWS CLI.

A fresh ~15-minute token is minted for each invocation and passed to the client via the child
process environment only (never on the command line or written to disk). The connection is made
over TLS (verify-full), trusting the host's system CA bundle unless --ca-bundle is given.

Provide the connection details as flags, or reference an aws/rds integration declared in atmos.yaml
by name (flags override the integration's declared values):

  atmos aws rds connect my-db
  atmos aws rds connect --host mydb.abc123.us-east-2.rds.amazonaws.com --port 5432 \
    --username app --region us-east-2 --identity dev-admin

The database, user, and IAM permissions must already be configured for IAM authentication
(EnableIAMDatabaseAuthentication, an rds-db:connect IAM policy, and the in-database grant).`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.MaximumNArgs(1),
	RunE:               executeConnectCommand,
	// Suppress usage on errors since the command execs an interactive client.
	SilenceUsage: true,
}

// executeConnectCommand binds flags (with Viper precedence) and delegates to runRDSConnect.
func executeConnectCommand(cmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	if err := rdsConnectParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Keys are namespaced under the "rds-connect" Viper prefix (see the parser in init). Identity is
	// read via the global parser, which handles the -i interactive-select sentinel and ATMOS_IDENTITY.
	opts := connectOptions{
		Host:         v.GetString("rds-connect.host"),
		Port:         v.GetInt("rds-connect.port"),
		Username:     v.GetString("rds-connect.username"),
		Region:       v.GetString("rds-connect.region"),
		Identity:     flags.ParseGlobalFlags(cmd, v).Identity.Value(),
		Engine:       v.GetString("rds-connect.engine"),
		Database:     v.GetString("rds-connect.database"),
		CABundle:     v.GetString("rds-connect.ca-bundle"),
		PrintCommand: v.GetBool("rds-connect.print-command"),
	}
	if len(args) == 1 {
		opts.Integration = args[0]
	}

	return runRDSConnect(opts)
}

// runRDSConnect resolves the connection, mints a fresh token, and execs the DB client.
func runRDSConnect(opts connectOptions) error {
	defer perf.Track(nil, "rds.runRDSConnect")()

	atmosConfig, err := initCliConfigFn(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	// A named integration supplies connection defaults; explicit flags override them.
	if opts.Integration != "" {
		if err := applyIntegration(&opts, &atmosConfig.Auth); err != nil {
			return err
		}
	}

	if err := validateConnectOptions(opts); err != nil {
		return err
	}

	spec, err := resolveEngine(opts.Engine, opts.Port)
	if err != nil {
		return err
	}

	caPath := opts.CABundle
	if caPath == "" {
		caPath = findCABundleFn()
	}
	clientArgs := buildClientArgs(spec, opts, caPath)

	// --print-command renders the resolved command without minting a token or connecting.
	if opts.PrintCommand {
		ui.Writeln(fmt.Sprintf("%s  # with %s=<token> injected into the environment", strings.Join(clientArgs, " "), spec.passwordEnv))
		return nil
	}

	// Skip integrations so connecting has no login-time side effects (kubeconfig/env rewrites).
	ctx := auth.ContextWithSkipIntegrations(context.Background())

	creds, err := authenticateForTokenFn(ctx, &atmosConfig.Auth, atmosConfig.CliConfigPath, opts.Identity)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrRDSConnectFailed, err)
	}

	endpoint := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	token, _, err := getRDSTokenFn(ctx, creds, endpoint, opts.Region, opts.Username)
	if err != nil {
		return err
	}

	clientEnv := buildClientEnv(spec, token, caPath, atmosConfig.Env)

	log.Debug("Connecting to RDS", "engine", spec.name, "endpoint", endpoint, "user", opts.Username, "identity", opts.Identity)
	ui.Info(fmt.Sprintf("Connecting to %s (%s) as %s...", endpoint, spec.name, opts.Username))

	// Return the runner error verbatim so its exit-code (ExitCodeError) and command-not-found
	// (ErrCommandNotFound) propagate unchanged.
	return runClientFn(clientArgs, clientEnv)
}

// applyIntegration fills unset options from a named aws/rds integration in the auth config.
func applyIntegration(opts *connectOptions, authConfig *schema.AuthConfig) error {
	integ, ok := authConfig.Integrations[opts.Integration]
	if !ok {
		return fmt.Errorf("%w: no integration named %q", errUtils.ErrRDSIntegrationConfig, opts.Integration)
	}
	if integ.Kind != integrations.KindAWSRDS {
		return fmt.Errorf("%w: integration %q is kind %q, want aws/rds", errUtils.ErrRDSIntegrationConfig, opts.Integration, integ.Kind)
	}
	if integ.Spec == nil || integ.Spec.Database == nil {
		return fmt.Errorf("%w: integration %q has no spec.database", errUtils.ErrRDSIntegrationConfig, opts.Integration)
	}

	db := integ.Spec.Database
	// Flags (already set) win over the integration's declared metadata.
	if opts.Host == "" {
		opts.Host = db.Host
	}
	if opts.Port == 0 {
		opts.Port = db.Port
	}
	if opts.Username == "" {
		opts.Username = db.Username
	}
	if opts.Region == "" {
		opts.Region = db.Region
	}
	if opts.Engine == "" {
		opts.Engine = db.Engine
	}
	if opts.Database == "" {
		opts.Database = db.Database
	}
	if opts.Identity == "" && integ.Via != nil {
		opts.Identity = integ.Via.Identity
	}
	return nil
}

// validateConnectOptions ensures the required connection inputs are present and in range.
func validateConnectOptions(opts connectOptions) error {
	switch {
	case opts.Host == "":
		return fmt.Errorf("%w: --host is required (or reference an aws/rds integration)", errUtils.ErrRDSConnectFailed)
	case opts.Port == 0:
		return fmt.Errorf("%w: --port is required", errUtils.ErrRDSConnectFailed)
	case opts.Port < 1 || opts.Port > 65535:
		return fmt.Errorf("%w: --port must be between 1 and 65535, got %d", errUtils.ErrRDSConnectFailed, opts.Port)
	case opts.Username == "":
		return fmt.Errorf("%w: --username is required", errUtils.ErrRDSConnectFailed)
	case opts.Region == "":
		return fmt.Errorf("%w: --region is required", errUtils.ErrRDSConnectFailed)
	}
	return nil
}

// resolveEngine selects the client spec from an explicit engine or, failing that, the port.
func resolveEngine(engine string, port int) (engineSpec, error) {
	if engine == "" {
		switch port {
		case 5432:
			engine = "postgres"
		case 3306:
			engine = "mysql"
		default:
			return engineSpec{}, fmt.Errorf("%w: cannot infer engine from port %d; pass --engine postgres|mysql", errUtils.ErrRDSEngineUnknown, port)
		}
	}
	spec, ok := engineSpecs[engine]
	if !ok {
		return engineSpec{}, fmt.Errorf("%w: %q (want postgres or mysql)", errUtils.ErrRDSEngineUnknown, engine)
	}
	return spec, nil
}

// buildClientArgs builds the client argv. The password is NEVER placed here — it goes in the env.
func buildClientArgs(spec engineSpec, opts connectOptions, caPath string) []string {
	port := strconv.Itoa(opts.Port)
	if spec.name == "postgres" {
		args := []string{spec.bin, "--host=" + opts.Host, "--port=" + port, "--username=" + opts.Username}
		if opts.Database != "" {
			args = append(args, "--dbname="+opts.Database)
		}
		return args
	}

	// mysql: IAM auth requires the cleartext plugin; VERIFY_IDENTITY mirrors psql verify-full.
	args := []string{spec.bin, "--host=" + opts.Host, "--port=" + port, "--user=" + opts.Username, "--enable-cleartext-plugin", "--ssl-mode=VERIFY_IDENTITY"}
	if caPath != "" {
		args = append(args, "--ssl-ca="+caPath)
	}
	if opts.Database != "" {
		args = append(args, opts.Database)
	}
	return args
}

// buildClientEnv builds the child environment. The token is injected here (env only), never argv.
func buildClientEnv(spec engineSpec, token, caPath string, globalEnv map[string]string) []string {
	env := envpkg.MergeGlobalEnv(os.Environ(), globalEnv)
	env = envpkg.UpdateEnvVar(env, spec.passwordEnv, token)
	if spec.name == "postgres" {
		env = envpkg.UpdateEnvVar(env, "PGSSLMODE", "verify-full")
		if caPath != "" {
			env = envpkg.UpdateEnvVar(env, "PGSSLROOTCERT", caPath)
		}
	}
	return env
}

func init() {
	rdsConnectParser = flags.NewStandardParser(
		flags.WithStringFlag("host", "", "", "RDS/Aurora endpoint hostname (required unless an integration is named)"),
		flags.WithIntFlag("port", "", 0, "Database port (required)"),
		flags.WithStringFlag("username", "u", "", "Database user name (required)"),
		flags.WithStringFlag("region", "", "", "AWS region of the database endpoint (required)"),
		flags.WithIdentityFlag(),
		flags.WithStringFlag("engine", "", "", "Database engine: postgres or mysql (inferred from the port when omitted)"),
		flags.WithValidValues("engine", "postgres", "mysql"),
		flags.WithStringFlag("database", "d", "", "Database name to connect to"),
		flags.WithStringFlag("ca-bundle", "", "", "Path to a CA bundle for TLS verification (defaults to the host trust store)"),
		flags.WithBoolFlag("print-command", "", false, "Print the resolved client command (token redacted) instead of connecting"),
		flags.WithEnvVars("host", "ATMOS_AWS_RDS_HOST"),
		flags.WithEnvVars("port", "ATMOS_AWS_RDS_PORT"),
		flags.WithEnvVars("username", "ATMOS_AWS_RDS_USERNAME"),
		flags.WithEnvVars("region", "ATMOS_AWS_RDS_REGION"),
		flags.WithEnvVars("engine", "ATMOS_AWS_RDS_ENGINE"),
		flags.WithEnvVars("database", "ATMOS_AWS_RDS_DATABASE"),
		flags.WithEnvVars("ca-bundle", "ATMOS_AWS_RDS_CA_BUNDLE"),
		// Namespace keys to rds-connect.* to avoid the shared-global-Viper bare-key collision.
		flags.WithViperPrefix("rds-connect"),
	)

	rdsConnectParser.RegisterFlags(connectCmd)

	if err := rdsConnectParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	RdsCmd.AddCommand(connectCmd)
}
