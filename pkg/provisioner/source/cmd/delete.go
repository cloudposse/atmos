package cmd

import (
	"errors"
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
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// DeleteCommand creates a delete command for the given component type.
func DeleteCommand(config *Config) *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithBoolFlag("force", "f", false, "Force deletion without confirmation"),
	)

	cmd := &cobra.Command{
		Use:   "delete [component]",
		Short: fmt.Sprintf("Remove vendored %s source directory", config.TypeLabel),
		Long: fmt.Sprintf(`Delete the vendored source directory for a %s component.

This command removes the component directory that was created by 'atmos %s source pull'.

If component is not specified, prompts interactively for selection.`, config.TypeLabel, config.ComponentType),
		Example: fmt.Sprintf(`  # Delete vendored source
  atmos %s source delete vpc --stack dev --force

  # Interactive: prompts for component and stack
  atmos %s source delete`, config.ComponentType, config.ComponentType),
		Args: cobra.RangeArgs(0, 1),
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
	Force       bool
	GlobalFlags global.Flags
}

func executeDelete(cmd *cobra.Command, args []string, config *Config, parser *flags.StandardParser) error {
	defer perf.Track(nil, fmt.Sprintf("source.%s.delete.RunE", config.ComponentType))()

	// Get component from args or prompt.
	var component string
	if len(args) > 0 {
		component = args[0]
	} else {
		var err error
		component, err = PromptForComponent(cmd)
		if err := HandlePromptError(err, "component"); err != nil {
			return err
		}
	}

	// Validate component is provided.
	if component == "" {
		return errUtils.Build(errUtils.ErrInvalidPositionalArgs).
			WithExplanation("component argument is required").
			Err()
	}

	// Parse flags and get delete options (with prompting).
	deleteOpts, err := parseDeleteFlags(cmd, parser, component)
	if err != nil {
		return err
	}

	// Initialize config and get component info with global flags.
	atmosConfig, componentConfig, err := initDeleteContext(component, deleteOpts.Stack, &deleteOpts.GlobalFlags)
	if err != nil {
		return err
	}

	// Determine and delete the target directory.
	return deleteSourceDirectory(atmosConfig, config.ComponentType, component, componentConfig, deleteOpts.Force)
}

// parseDeleteFlags parses delete command flags and validates them.
func parseDeleteFlags(cmd *cobra.Command, parser *flags.StandardParser, component string) (*deleteOptions, error) {
	v := viper.GetViper()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return nil, err
	}

	globalFlags := flags.ParseGlobalFlags(cmd, v)
	stack := v.GetString("stack")

	// Prompt for stack if not provided.
	if stack == "" {
		var err error
		stack, err = PromptForStack(cmd, component)
		if err := HandlePromptError(err, "stack"); err != nil {
			return nil, err
		}
	}

	// Validate stack is provided.
	if stack == "" {
		return nil, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			Err()
	}

	return &deleteOptions{
		Stack:       stack,
		Force:       v.GetBool("force"),
		GlobalFlags: globalFlags,
	}, nil
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
func deleteSourceDirectory(atmosConfig *schema.AtmosConfiguration, componentType, component string, componentConfig map[string]any, force bool) error {
	targetDir, err := source.DetermineTargetDirectory(atmosConfig, componentType, component, componentConfig)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithContext("component", component).
			Err()
	}

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		_ = ui.Warning(fmt.Sprintf("Directory does not exist: %s", targetDir))
		return nil
	}

	// Prompt for confirmation unless --force.
	confirmed, err := flags.PromptForConfirmation(fmt.Sprintf("Delete directory: %s?", targetDir), force)
	if err != nil {
		if errors.Is(err, errUtils.ErrInteractiveNotAvailable) {
			_ = ui.Warning("Use --force to delete in non-interactive mode")
		}
		return err
	}
	if !confirmed {
		_ = ui.Info("Deletion cancelled")
		return nil
	}

	// Delete with spinner.
	return spinner.ExecWithSpinner(
		fmt.Sprintf("Deleting %s", targetDir),
		fmt.Sprintf("Deleted %s", targetDir),
		func() error {
			if err := os.RemoveAll(targetDir); err != nil {
				return errUtils.Build(errUtils.ErrRemoveDirectory).
					WithCause(err).
					WithContext("path", targetDir).
					Err()
			}
			return nil
		},
	)
}
