package aws

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/cloud/aws/credentialprocess"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Testing seams: package-level indirection so tests can stub config loading and auth manager construction.
var (
	// Loads Atmos CLI configuration.
	initCliConfigFn = cfg.InitCliConfig

	// Constructs the AuthManager used to produce credentials.
	newAuthManagerFn = auth.NewDefaultManager
)

//go:embed markdown/atmos_aws_credential_process_usage.md
var credentialProcessUsageMarkdown string

// credentialProcessParser handles flags for the credential-process command.
var credentialProcessParser = newCredentialProcessParser()

func newCredentialProcessParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithIdentityFlag(),
		flags.WithStringFlag(credentialprocess.MinValidityFlagName, "", credentialprocess.FormatMinValidity(credentialprocess.DefaultMinValidity),
			"Reuse cached credentials only if they remain valid for at least this long (for example, 15m)"),
		flags.WithEnvVars(credentialprocess.MinValidityFlagName, credentialprocess.MinValidityEnvVar),
	)
}

// credentialProcessCmd prints AWS process-credential JSON for an Atmos identity.
var credentialProcessCmd = &cobra.Command{
	Use:   "credential-process",
	Short: "Print AWS credentials for an identity in the credential_process format",
	Long: "Print AWS credentials for an Atmos identity as the JSON document defined by the AWS SDKs for " +
		"'credential_process', so the AWS CLI, SDKs, and any tool that reads ~/.aws/config can source " +
		"credentials from Atmos Auth.\n\n" +
		"Reuses cached credentials that stay valid for at least --min-validity.\n\n" +
		"The identity must be named with --identity (or ATMOS_IDENTITY) or marked as the default identity. " +
		"The interactive identity selector is never shown, because the AWS CLI captures this command's output.",
	Example: credentialProcessUsageMarkdown,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	// Suppress usage on errors since the AWS CLI and SDKs invoke this automatically.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return executeCredentialProcess(cmd, viper.GetViper())
	},
}

// executeCredentialProcess runs the credential-process command using the given Viper instance.
func executeCredentialProcess(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "aws.executeCredentialProcess")()

	if err := credentialProcessParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	minValidity, err := credentialprocess.ParseMinValidity(v.GetString(credentialprocess.MinValidityFlagName))
	if err != nil {
		return err
	}

	atmosConfig, err := initCliConfigFn(flags.BuildConfigAndStacksInfo(cmd, v), false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}

	mgr, err := newAuthManagerFn(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return writeCredentialProcess(ctx, cmd, v, mgr, minValidity)
}

// writeCredentialProcess resolves the identity, produces the credential document, and writes it to stdout.
func writeCredentialProcess(ctx context.Context, cmd *cobra.Command, v *viper.Viper, mgr types.AuthManager, minValidity time.Duration) error {
	// Flag, then ATMOS_IDENTITY (via Viper), then the configured default identity.
	identityName, err := credentialprocess.ResolveIdentity(mgr, flags.ParseGlobalFlags(cmd, v).Identity.Value())
	if err != nil {
		return err
	}

	out, err := credentialprocess.Produce(ctx, mgr, identityName, credentialprocess.WithMinValidity(minValidity))
	if err != nil {
		return err
	}

	// The credentials are the purpose of this command, so masking them would make the output unusable.
	// codeql[go/clear-text-logging]: intentional credential output for the AWS credential_process protocol.
	return data.WriteUnmasked(credentialprocess.Render(out))
}

func init() {
	credentialProcessParser.RegisterFlags(credentialProcessCmd)

	// The shared --identity help text advertises interactive selection, which this command never offers.
	if f := credentialProcessCmd.Flags().Lookup(cfg.IdentityFlagName); f != nil {
		f.Usage = "Identity to print credentials for (default: ATMOS_IDENTITY, then the default identity); required when no default identity is configured"
	}

	if err := credentialProcessParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
