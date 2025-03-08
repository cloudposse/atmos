package exec

import (
	"fmt"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	ErrExecuteVendorDiffCmd       = errors.New("'atmos vendor diff' is not implemented yet")
	ErrValidateComponentFlag      = errors.New("either '--component' or '--tags' flag can be provided, but not both")
	ErrValidateComponentStackFlag = errors.New("either '--component' or '--stack' flag can be provided, but not both")
	ErrValidateEverythingFlag     = errors.New("'--everything' flag cannot be combined with '--component', '--stack', or '--tags' flags")
)

// ExecuteVendorPullCmd executes `vendor pull` commands.
func ExecuteVendorPullCmd(cmd *cobra.Command, args []string) error {
	return ExecuteVendorPullCommand(cmd, args)
}

// ExecuteVendorDiffCmd executes `vendor diff` commands.
func ExecuteVendorDiffCmd(cmd *cobra.Command, args []string) error {
	return ErrExecuteVendorDiffCmd
}

type VendorFlags struct {
	DryRun        bool
	Component     string
	Stack         string
	Tags          []string
	Everything    bool
	ComponentType string
}

// ExecuteVendorPullCommand executes `atmos vendor` commands.
func ExecuteVendorPullCommand(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	processStacks := flags.Changed("stack")

	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	vendorFlags, err := parseVendorFlags(flags)
	if err != nil {
		return err
	}

	if err := validateVendorFlags(&vendorFlags); err != nil {
		return err
	}

	if vendorFlags.Stack != "" {
		return ExecuteStackVendorInternal(vendorFlags.Stack, vendorFlags.DryRun)
	}

	return handleVendorConfig(&atmosConfig, &vendorFlags, args)
}

func parseVendorFlags(flags *pflag.FlagSet) (VendorFlags, error) {
	vendorFlags := VendorFlags{}
	var err error

	if vendorFlags.DryRun, err = flags.GetBool("dry-run"); err != nil {
		return vendorFlags, err
	}

	if vendorFlags.Component, err = flags.GetString("component"); err != nil {
		return vendorFlags, err
	}

	if vendorFlags.Stack, err = flags.GetString("stack"); err != nil {
		return vendorFlags, err
	}

	tagsCsv, err := flags.GetString("tags")
	if err != nil {
		return vendorFlags, err
	}
	if tagsCsv != "" {
		vendorFlags.Tags = strings.Split(tagsCsv, ",")
	}

	if vendorFlags.Everything, err = flags.GetBool("everything"); err != nil {
		return vendorFlags, err
	}

	// Set default for 'everything' if no specific flags are provided
	setDefaultEverythingFlag(flags, &vendorFlags)

	// Handle 'type' flag only if it exists
	if flags.Lookup("type") != nil {
		if vendorFlags.ComponentType, err = flags.GetString("type"); err != nil {
			return vendorFlags, err
		}
	}

	return vendorFlags, nil
}

// Helper function to set the default for 'everything' if no specific flags are provided.
func setDefaultEverythingFlag(flags *pflag.FlagSet, vendorFlags *VendorFlags) {
	if !vendorFlags.Everything && !flags.Changed("everything") &&
		vendorFlags.Component == "" && vendorFlags.Stack == "" && len(vendorFlags.Tags) == 0 {
		vendorFlags.Everything = true
	}
}

func validateVendorFlags(flg *VendorFlags) error {
	if flg.Component != "" && flg.Stack != "" {
		return ErrValidateComponentStackFlag
	}

	if flg.Component != "" && len(flg.Tags) > 0 {
		return ErrValidateComponentFlag
	}

	if flg.Everything && (flg.Component != "" || flg.Stack != "" || len(flg.Tags) > 0) {
		return ErrValidateEverythingFlag
	}

	return nil
}

func handleVendorConfig(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags, args []string) error {
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		cfg.AtmosVendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}
	ErrVendorConfigNotExist := errors.New("the '--everything' flag is set, but vendor config file does not exist")
	if !vendorConfigExists && flg.Everything {
		return fmt.Errorf("%w: %s", ErrVendorConfigNotExist, cfg.AtmosVendorConfigFileName)
	}
	if vendorConfigExists {
		return ExecuteAtmosVendorInternal(&executeVendorOptions{
			vendorConfigFileName: foundVendorConfigFile,
			dryRun:               flg.DryRun,
			atmosConfig:          atmosConfig,
			atmosVendorSpec:      vendorConfig.Spec,
			component:            flg.Component,
			tags:                 flg.Tags,
		})
	}

	if flg.Component != "" {
		return handleComponentVendor(atmosConfig, flg)
	}

	return formatVendorError(args)
}

func handleComponentVendor(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	componentType := flg.ComponentType
	if componentType == "" {
		componentType = "terraform"
	}

	config, path, err := ReadAndProcessComponentVendorConfigFile(
		atmosConfig,
		flg.Component,
		componentType,
	)
	if err != nil {
		return err
	}

	return ExecuteComponentVendorInternal(
		atmosConfig,
		&config.Spec,
		flg.Component,
		path,
		flg.DryRun,
	)
}

func formatVendorError(args []string) error {
	msg := "to vendor a component, the '--component' (shorthand '-c') flag needs to be specified.\n" +
		"Example: atmos vendor pull -c <component>"

	if len(args) > 0 {
		msg += fmt.Sprintf("\nDid you mean 'atmos vendor pull -c %s'?", args[0])
	}

	return errors.New(msg)
}
