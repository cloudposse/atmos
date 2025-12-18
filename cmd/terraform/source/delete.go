package source

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var deleteParser *flags.StandardParser

var deleteCmd = &cobra.Command{
	Use:   "delete <component>",
	Short: "Remove vendored source directory",
	Long: `Delete the vendored source directory for a terraform component.

This command removes the component directory that was created by 'atmos terraform source pull'.
Requires --force flag for safety.`,
	Example: `  # Delete vendored source
  atmos terraform source delete vpc --stack dev --force`,
	Args: cobra.ExactArgs(1),
	RunE: executeDeleteCommand,
}

func init() {
	deleteCmd.DisableFlagParsing = false

	deleteParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithBoolFlag("force", "f", false, "Force deletion without confirmation"),
	)

	deleteParser.RegisterFlags(deleteCmd)

	if err := deleteParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeDeleteCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "source.delete.RunE")()

	component := args[0]

	// Parse flags and get delete options.
	deleteOpts, err := parseDeleteFlags(cmd)
	if err != nil {
		return err
	}

	// Initialize config and get component info with global flags.
	atmosConfig, componentConfig, err := initDeleteContext(component, deleteOpts.Stack, &deleteOpts.GlobalFlags)
	if err != nil {
		return err
	}

	// Determine and delete the target directory.
	return deleteSourceDirectory(atmosConfig, component, componentConfig, deleteOpts.Stack)
}

// parseDeleteFlags parses delete command flags and validates them.
func parseDeleteFlags(cmd *cobra.Command) (*deleteOptions, error) {
	v := viper.GetViper()
	if err := deleteParser.BindFlagsToViper(cmd, v); err != nil {
		return nil, err
	}

	globalFlags := flags.ParseGlobalFlags(cmd, v)
	stack := v.GetString("stack")
	if stack == "" {
		return nil, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			Err()
	}

	force := v.GetBool("force")
	if !force {
		return nil, errUtils.Build(errUtils.ErrForceRequired).
			WithExplanation("Deletion requires --force flag for safety").
			WithHint("Use --force to confirm deletion").
			Err()
	}

	return &deleteOptions{Stack: stack, GlobalFlags: globalFlags}, nil
}

// initDeleteContext initializes config and retrieves component configuration.
func initDeleteContext(component, stack string, globalFlags *global.Flags) (*schema.AtmosConfiguration, map[string]any, error) {
	// Build config info with global flag values.
	configInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
	}

	// Wire global flags to config info if provided.
	if globalFlags != nil {
		configInfo.AtmosBasePath = globalFlags.BasePath
		configInfo.AtmosConfigFilesFromArg = globalFlags.Config
		configInfo.AtmosConfigDirsFromArg = globalFlags.ConfigPath
		configInfo.ProfilesFromArg = globalFlags.Profile
	}

	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	if err != nil {
		return nil, nil, errUtils.Build(errUtils.ErrFailedToInitConfig).WithCause(err).Err()
	}

	componentConfig, err := DescribeComponent(component, stack)
	if err != nil {
		return nil, nil, errUtils.Build(errUtils.ErrDescribeComponent).
			WithCause(err).
			WithContext("component", component).
			WithContext("stack", stack).
			Err()
	}

	if !source.HasSource(componentConfig) {
		return nil, nil, errUtils.Build(errUtils.ErrMetadataSourceMissing).
			WithContext("component", component).
			WithContext("stack", stack).
			WithHint("Only components with source can be deleted via this command").
			Err()
	}

	return &atmosConfig, componentConfig, nil
}

// deleteSourceDirectory deletes the vendored source directory.
func deleteSourceDirectory(atmosConfig *schema.AtmosConfiguration, component string, componentConfig map[string]any, _ string) error {
	targetDir, err := source.DetermineTargetDirectory(atmosConfig, "terraform", component, componentConfig)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to determine target directory").
			Err()
	}

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		_ = ui.Warning(fmt.Sprintf("Directory does not exist: %s", targetDir))
		return nil
	}

	_ = ui.Info(fmt.Sprintf("Deleting directory: %s", targetDir))
	if err := os.RemoveAll(targetDir); err != nil {
		return errUtils.Build(errUtils.ErrRemoveDirectory).
			WithCause(err).
			WithContext("path", targetDir).
			Err()
	}

	_ = ui.Success(fmt.Sprintf("Successfully deleted: %s", targetDir))
	return nil
}

// deleteOptions holds parsed delete command options.
type deleteOptions struct {
	Stack       string
	GlobalFlags global.Flags
}
