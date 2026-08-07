package toolchain

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

var lockParser *flags.StandardParser

var lockCmd = &cobra.Command{
	Use:   "lock [tool...]",
	Short: "Generate or refresh toolchain.lock.yaml without reinstalling tools",
	Long: `Download and verify each configured tool's artifact to compute a real checksum
and write/update its toolchain.lock.yaml entry, without extracting or installing the
binary into the toolchain tree.

If no tools are given, every tool in .tool-versions is locked. This is the way to
populate or refresh the lock file for tools that are already installed, without
forcing a full reinstall with --reinstall.`,
	Args: cobra.ArbitraryArgs,
	Example: `  atmos toolchain lock
  atmos toolchain lock terraform
  atmos toolchain lock terraform kubectl --max-concurrency 2`,
	RunE:          runLock,
	SilenceUsage:  true, // Don't show usage on error.
	SilenceErrors: true, // Don't show errors twice.
}

func init() {
	lockParser = flags.NewStandardParser(
		flags.WithIntFlag(maxConcurrencyFlagName, "", 0, "Maximum number of tools to lock concurrently (default 4)"),
		flags.WithEnvVars(maxConcurrencyFlagName, maxConcurrencyEnvVar),
	)
	lockParser.RegisterFlags(lockCmd)
	if err := lockParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runLock(cmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	if err := lockParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	maxConcurrency, err := resolveInstallMaxConcurrency(cmd)
	if err != nil {
		return err
	}

	return toolchain.RunLock(args, toolchain.LockOptions{
		MaxConcurrency: maxConcurrency,
	})
}

// LockCommandProvider implements the CommandProvider interface.
type LockCommandProvider struct{}

func (l *LockCommandProvider) GetCommand() *cobra.Command {
	return lockCmd
}

func (l *LockCommandProvider) GetName() string {
	return "lock"
}

func (l *LockCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (l *LockCommandProvider) GetFlagsBuilder() flags.Builder {
	return lockParser
}

func (l *LockCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (l *LockCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
