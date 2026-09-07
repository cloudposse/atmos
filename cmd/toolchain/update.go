package toolchain

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

var updateParser *flags.StandardParser

var updateCmd = &cobra.Command{
	Use:   "update [tool...]",
	Short: "Update tools to their newest available version",
	Long: `Update one or more tools to their newest available version.

If no tools are given, every tool in .tool-versions is updated.

A tool pinned to "latest" is re-resolved to the newest version. A tool pinned
to an exact version is replaced with the newest available version and
reinstalled. Tools pinned to pr:/sha:/ref: are skipped — update never
overrides an explicit source selection like this, even ref:<name>, which can
move over time; use "add" to change one explicitly.`,
	Args: cobra.ArbitraryArgs,
	Example: `  atmos toolchain update
  atmos toolchain update terraform
  atmos toolchain update terraform kubectl --dry-run`,
	RunE:          runUpdate,
	SilenceUsage:  true, // Don't show usage on error.
	SilenceErrors: true, // Don't show errors twice.
}

func init() {
	updateParser = flags.NewStandardParser(
		flags.WithBoolFlag("dry-run", "", false, "Report what would change without installing anything"),
		flags.WithIntFlag(maxConcurrencyFlagName, "", 0, "Maximum number of tools to update concurrently (default 4)"),
		flags.WithEnvVars("dry-run", "ATMOS_TOOLCHAIN_UPDATE_DRY_RUN"),
		flags.WithEnvVars(maxConcurrencyFlagName, maxConcurrencyEnvVar),
	)
	updateParser.RegisterFlags(updateCmd)
	if err := updateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runUpdate(cmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	if err := updateParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Use IsBoolFlagExplicitlySet rather than v.GetBool: both "update" and
	// "exec" register a "dry-run" flag on the shared global Viper instance
	// under the same key, so v.BindEnv calls for one command's env var can
	// overwrite the other's binding for that key. IsBoolFlagExplicitlySet
	// checks this parser's own registry/env vars directly, avoiding the
	// cross-command collision.
	_, dryRun := updateParser.IsBoolFlagExplicitlySet(cmd, "dry-run")
	maxConcurrency, err := resolveInstallMaxConcurrency(cmd)
	if err != nil {
		return err
	}

	return toolchain.RunUpdate(args, toolchain.UpdateOptions{
		DryRun:         dryRun,
		MaxConcurrency: maxConcurrency,
	})
}

// UpdateCommandProvider implements the CommandProvider interface.
type UpdateCommandProvider struct{}

func (u *UpdateCommandProvider) GetCommand() *cobra.Command {
	return updateCmd
}

func (u *UpdateCommandProvider) GetName() string {
	return "update"
}

func (u *UpdateCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (u *UpdateCommandProvider) GetFlagsBuilder() flags.Builder {
	return updateParser
}

func (u *UpdateCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (u *UpdateCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
