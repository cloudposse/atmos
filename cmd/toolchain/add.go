package toolchain

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/ui"
)

var addParser *flags.StandardParser

var addCmd = &cobra.Command{
	Use:   "add <tool[@version]>...",
	Short: "Add tools to .tool-versions file",
	Long: `Add one or more tools and versions to the .tool-versions file.
If version is omitted, defaults to "latest".

By default, adding a version to a tool that's already configured appends it
alongside the existing version(s) rather than replacing the default. Use
--default to replace the default version instead.`,
	Args:          cobra.MinimumNArgs(1),
	RunE:          runAdd,
	SilenceUsage:  true, // Don't show usage on error.
	SilenceErrors: true, // Don't show errors twice.
}

func init() {
	addParser = flags.NewStandardParser(
		flags.WithBoolFlag("default", "", false, "Replace the tool's default version instead of appending"),
		flags.WithEnvVars("default", "ATMOS_TOOLCHAIN_DEFAULT"),
	)
	addParser.RegisterFlags(addCmd)
	if err := addParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runAdd(cmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	if err := addParser.BindFlagsToViper(cmd, v); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrFlagBinding, err)
	}
	setAsDefault := v.GetBool("default")

	if setAsDefault && len(args) > 1 {
		ui.Warning("--default flag is ignored when adding multiple tools")
		setAsDefault = false
	}

	for _, arg := range args {
		tool, version, err := toolchain.ParseToolVersionArg(arg)
		if err != nil {
			return fmt.Errorf("%w: failed to parse '%s': %w", errUtils.ErrToolVersionsFileOperation, arg, err)
		}
		// Default to "latest" if no version specified.
		if version == "" {
			version = "latest"
		}
		if err := toolchain.AddToolVersion(tool, version, setAsDefault); err != nil {
			// WithCause (not a second %w) extracts and re-attaches hints/details from err
			// before wrapping -- a double-%w fmt.Errorf here would silently discard any
			// hint ValidateVersionSpec attaches (e.g. the range/constraint rejection hint),
			// since cockroachdb/errors' GetAllHints can't traverse a Go 1.20 multi-wrap.
			return errUtils.Build(errUtils.ErrToolVersionsFileOperation).
				WithCause(fmt.Errorf("failed to add '%s': %w", arg, err)).
				Err()
		}
	}
	return nil
}

// AddCommandProvider implements the CommandProvider interface.
type AddCommandProvider struct{}

func (a *AddCommandProvider) GetCommand() *cobra.Command {
	return addCmd
}

func (a *AddCommandProvider) GetName() string {
	return "add"
}

func (a *AddCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (a *AddCommandProvider) GetFlagsBuilder() flags.Builder {
	return addParser
}

func (a *AddCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (a *AddCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
