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

If component is not specified, prompts interactively for selection.`, config.TypeLabel, config.CLI()),
		Example: fmt.Sprintf(`  # Delete vendored source
  atmos %s source delete vpc --stack dev --force

  # Interactive: prompts for component and stack
  atmos %s source delete`, config.CLI(), config.CLI()),
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
		var promptErr error
		component, promptErr = PromptForComponent(cmd)
		if err := HandlePromptError(promptErr, "component"); err != nil {
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

	// Initialize config and get component info with global flags. Dry-run performs the same
	// validation as the real run (component exists, has source, destination resolvable).
	atmosConfig, componentConfig, err := loadSourceComponent(component, deleteOpts.Stack, &deleteOpts.GlobalFlags, deleteMissingSourceHint)
	if err != nil {
		return err
	}

	// Determine and delete the target directory.
	return deleteSourceDirectory(atmosConfig, deleteRequest{
		componentType:   config.ComponentType,
		component:       component,
		stack:           deleteOpts.Stack,
		componentConfig: componentConfig,
		force:           deleteOpts.Force,
		dryRun:          sourceDryRun(cmd),
		globalFlags:     &deleteOpts.GlobalFlags,
	})
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
		var promptErr error
		stack, promptErr = PromptForStack(cmd, component)
		if err := HandlePromptError(promptErr, "stack"); err != nil {
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

// deleteMissingSourceHint is the hint shown when `source delete` targets a component without source.
const deleteMissingSourceHint = "Only components with source can be deleted via this command"

// loadSourceComponent initializes config and retrieves the component configuration, requiring
// that the component declares `source:`. It is shared by the real and dry-run paths of
// `source pull` and `source delete` so both reject the same inputs.
func loadSourceComponent(component, stack string, globalFlags *global.Flags, missingSourceHint string) (*schema.AtmosConfiguration, map[string]any, error) {
	atmosConfig, err := initCliConfigFunc(sourceConfigInfo(component, stack, globalFlags), false)
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
			WithHint(missingSourceHint).
			Err()
	}

	return &atmosConfig, componentConfig, nil
}

// sourceConfigInfo builds the config-loading input for a component, wiring the CLI global flags
// (--base-path, --config, --config-path, --profile) when provided.
func sourceConfigInfo(component, stack string, globalFlags *global.Flags) schema.ConfigAndStacksInfo {
	configInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
	}
	if globalFlags != nil {
		configInfo.AtmosBasePath = globalFlags.BasePath
		configInfo.AtmosConfigFilesFromArg = globalFlags.Config
		configInfo.AtmosConfigDirsFromArg = globalFlags.ConfigPath
		configInfo.ProfilesFromArg = globalFlags.Profile
	}
	return configInfo
}

// deleteRequest bundles the inputs of one `source delete` invocation.
type deleteRequest struct {
	componentType   string
	component       string
	stack           string
	componentConfig map[string]any
	force           bool
	dryRun          bool
	globalFlags     *global.Flags
}

// deleteSourceDirectory deletes the vendored source directory.
//
// The directory is the one the runtime provisions into (see source.ResolveTarget), not one derived
// from the instance name. A directory that a component without `source:` owns, or that the source
// provisioner did not create, is never deleted. A dry run performs every check and skips only the
// confirmation prompt and the deletion.
func deleteSourceDirectory(atmosConfig *schema.AtmosConfiguration, req deleteRequest) error {
	target, err := source.ResolveTarget(atmosConfig, req.componentType, req.component, req.componentConfig)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithContext("component", req.component).
			Err()
	}
	targetDir := target.Dir

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		ui.Warning(fmt.Sprintf("Directory does not exist: %s", targetDir))
		return nil
	}

	if err := requireConfirmationPossible(req); err != nil {
		return err
	}

	if err := checkDeletable(atmosConfig, req, targetDir); err != nil {
		return err
	}

	if req.dryRun {
		ui.Info(fmt.Sprintf("Dry run: would delete %s (source for %s in stack %s)", targetDir, req.component, req.stack))
		return nil
	}

	// Prompt for confirmation unless --force.
	confirmed, err := flags.PromptForConfirmation(fmt.Sprintf("Delete directory: %s?", targetDir), req.force)
	if err != nil {
		if errors.Is(err, errUtils.ErrInteractiveNotAvailable) {
			ui.Warning("Use --force to delete in non-interactive mode")
		}
		return err
	}
	if !confirmed {
		ui.Info("Deletion cancelled")
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

// requireConfirmationPossible reports a non-interactive session without --force before the
// ownership checks, so the established "use --force" contract comes first. A dry run never prompts.
func requireConfirmationPossible(req deleteRequest) error {
	if req.force || req.dryRun || flags.ConfirmationAvailable() {
		return nil
	}
	ui.Warning("Use --force to delete in non-interactive mode")
	return errUtils.ErrInteractiveNotAvailable
}

// checkDeletable loads the stack's components of this type and applies the source package's
// deletion guard to targetDir.
func checkDeletable(atmosConfig *schema.AtmosConfiguration, req deleteRequest, targetDir string) error {
	// Describing stacks needs a configuration with the stack files resolved, which the lighter
	// configuration used to describe a single component does not carry.
	stackConfig, err := initCliConfigForPrompt(sourceConfigInfo(req.component, req.stack, req.globalFlags), true)
	if err != nil {
		return errUtils.Build(errUtils.ErrFailedToInitConfig).
			WithCause(err).
			WithExplanation("Cannot verify that no other component owns the directory, so it will not be deleted").
			Err()
	}
	stacksMap, err := executeDescribeStacksFunc(
		&stackConfig,
		req.stack,
		nil,
		[]string{req.componentType},
		nil,
		false, false, false, false,
		nil, nil,
	)
	if err != nil {
		return errUtils.Build(errUtils.ErrExecuteDescribeStacks).
			WithCause(err).
			WithExplanation("Cannot verify that no other component owns the directory, so it will not be deleted").
			WithContext("stack", req.stack).
			Err()
	}
	return source.CheckDeletable(atmosConfig, req.componentType, targetDir, stackComponentsOfType(stacksMap, req.stack, req.componentType))
}

// stackComponentsOfType returns the `components.<type>` map of a stack from a describe-stacks result.
func stackComponentsOfType(stacksMap map[string]any, stack, componentType string) map[string]any {
	stackData, ok := stacksMap[stack].(map[string]any)
	if !ok {
		return nil
	}
	components, ok := stackData["components"].(map[string]any)
	if !ok {
		return nil
	}
	typed, _ := components[componentType].(map[string]any)
	return typed
}
