package generate

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
)

// filesParser handles flag parsing for files command.
var filesParser *flags.StandardParser

// filesCmd generates files for terraform components from the generate section.
var filesCmd = &cobra.Command{
	Use:   "files [component]",
	Short: "Generate files for Terraform components from the generate section",
	Long: `Generate additional configuration files for Terraform components based on
the generate section in stack configuration.

When called with a component argument, generates files for that component.
When called with --all, generates files for all components across stacks.

The generate section in stack configuration supports:
- Map values: serialized based on file extension (.json, .yaml, .hcl, .tf)
- String values: written as literal templates with Go template support`,
	Args:               cobra.MaximumNArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use Viper to respect precedence (flag > env > config > default).
		v := viper.GetViper()

		// Bind files-specific flags to Viper.
		if err := filesParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values from Viper.
		stack := v.GetString("stack")
		all := v.GetBool("all")
		stacksCsv := v.GetString("stacks")
		componentsCsv := v.GetString("components")
		dryRun := v.GetBool("dry-run")
		clean := v.GetBool("clean")

		// Validate: component requires stack, --all excludes component.
		if len(args) > 0 && all {
			return fmt.Errorf("%w: cannot specify both component and --all", errUtils.ErrInvalidFlag)
		}
		if len(args) > 0 && stack == "" {
			return fmt.Errorf("%w: --stack is required when specifying a component", errUtils.ErrInvalidFlag)
		}
		if len(args) == 0 && !all {
			return fmt.Errorf("%w: either specify a component or use --all", errUtils.ErrInvalidFlag)
		}

		// Parse CSV values.
		var stacks []string
		if stacksCsv != "" {
			stacks = strings.Split(stacksCsv, ",")
		}

		var components []string
		if componentsCsv != "" {
			components = strings.Split(componentsCsv, ",")
		}

		// Get global flags from Viper (includes base-path, config, config-path, profile).
		globalFlags := flags.ParseGlobalFlags(cmd, v)

		// Build ConfigAndStacksInfo from global flags to honor config selection flags.
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			AtmosBasePath:           globalFlags.BasePath,
			AtmosConfigFilesFromArg: globalFlags.Config,
			AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
			ProfilesFromArg:         globalFlags.Profile,
		}

		// Initialize Atmos configuration.
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Create adapter and service.
		adapter := tfgenerate.NewExecAdapter(e.ProcessStacksForGenerate, e.FindStacksMapForGenerate)
		service := tfgenerate.NewService(adapter)

		// Execute based on mode.
		if all {
			return service.ExecuteForAll(&atmosConfig, stacks, components, dryRun, clean)
		}

		component := args[0]
		return service.ExecuteForComponent(&atmosConfig, component, stack, dryRun, clean)
	},
}

func init() {
	// Create parser with files-specific flags using functional options.
	filesParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Stack name (required for single component)"),
		flags.WithBoolFlag("all", "", false, "Process all components in all stacks"),
		flags.WithStringFlag("stacks", "", "", "Filter stacks (glob pattern, requires --all)"),
		flags.WithStringFlag("components", "", "", "Filter components (comma-separated, requires --all)"),
		flags.WithBoolFlag("dry-run", "", false, "Show what would be generated without writing"),
		flags.WithBoolFlag("clean", "", false, "Delete generated files instead of creating"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("stacks", "ATMOS_STACKS"),
		flags.WithEnvVars("components", "ATMOS_COMPONENTS"),
	)

	// Register flags with the command.
	filesParser.RegisterFlags(filesCmd)

	// Bind flags to Viper for environment variable support.
	if err := filesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register with parent GenerateCmd.
	GenerateCmd.AddCommand(filesCmd)
}
