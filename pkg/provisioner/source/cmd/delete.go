package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// DeleteCommand creates a delete command for the given component type.
func DeleteCommand(config *Config) *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithBoolFlag("force", "f", false, "Force deletion without confirmation"),
	)

	cmd := &cobra.Command{
		Use:   "delete <component>",
		Short: fmt.Sprintf("Remove vendored %s source directory", config.TypeLabel),
		Long: fmt.Sprintf(`Delete the vendored source directory for a %s component.

This command removes the component directory that was created by 'atmos %s source pull'.
Requires --force flag for safety.`, config.TypeLabel, config.ComponentType),
		Example: fmt.Sprintf(`  # Delete vendored source
  atmos %s source delete vpc --stack dev --force`, config.ComponentType),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeDelete(cmd, args, config, parser)
		},
	}

	cmd.DisableFlagParsing = false
	parser.RegisterFlags(cmd)

	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	return cmd
}

// deleteOptions holds parsed delete command options.
type deleteOptions struct {
	Stack       string
	GlobalFlags global.Flags
}

func executeDelete(cmd *cobra.Command, args []string, config *Config, parser *flags.StandardParser) error {
	defer perf.Track(nil, fmt.Sprintf("source.%s.delete.RunE", config.ComponentType))()

	component := args[0]

	// Parse flags and get delete options.
	deleteOpts, err := parseDeleteFlags(cmd, parser)
	if err != nil {
		return err
	}

	// Initialize config and get component info with global flags.
	atmosConfig, componentConfig, err := initDeleteContext(component, deleteOpts.Stack, &deleteOpts.GlobalFlags)
	if err != nil {
		return err
	}

	// Determine and delete the target directory.
	return deleteSourceDirectory(atmosConfig, config.ComponentType, component, componentConfig)
}

// parseDeleteFlags parses delete command flags and validates them.
func parseDeleteFlags(cmd *cobra.Command, parser *flags.StandardParser) (*deleteOptions, error) {
	v := viper.GetViper()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
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

	atmosConfig, err := initCliConfigFunc(configInfo, false)
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
		return nil, nil, errUtils.Build(errUtils.ErrSourceMissing).
			WithContext("component", component).
			WithContext("stack", stack).
			WithHint("Only components with source can be deleted via this command").
			Err()
	}

	return &atmosConfig, componentConfig, nil
}

// deleteSourceDirectory deletes the vendored source directory.
func deleteSourceDirectory(atmosConfig *schema.AtmosConfiguration, componentType, component string, componentConfig map[string]any) error {
	targetDir, err := source.DetermineTargetDirectory(atmosConfig, componentType, component, componentConfig)
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
