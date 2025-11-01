package exec

import (
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	ErrVendorConfigNotExist       = errors.New("vendor config file does not exist")
	ErrExecuteVendorDiffCmd       = errors.New("'atmos vendor diff' is not implemented yet")
	ErrValidateComponentFlag      = errors.New("incompatible flags: --component and --tags")
	ErrValidateComponentStackFlag = errors.New("incompatible flags: --component and --stack")
	ErrValidateEverythingFlag     = errors.New("incompatible flags: --everything with other filters")
	ErrMissingVendorComponent     = errors.New("vendor component flag missing")
)

// ExecuteVendorPullCmd executes `vendor pull` commands.
func ExecuteVendorPullCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteVendorPullCmd")()

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
	defer perf.Track(nil, "exec.ExecuteVendorPullCommand")()

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
		return errUtils.Build(ErrValidateComponentStackFlag).
			WithHint("Remove either '--component' or '--stack' flag").
			WithExitCode(2).
			Err()
	}

	if flg.Component != "" && len(flg.Tags) > 0 {
		return errUtils.Build(ErrValidateComponentFlag).
			WithHint("Remove either '--component' or '--tags' flag").
			WithExitCode(2).
			Err()
	}

	if flg.Everything && (flg.Component != "" || flg.Stack != "" || len(flg.Tags) > 0) {
		return errUtils.Build(ErrValidateEverythingFlag).
			WithHint("Use '--everything' alone without '--component', '--stack', or '--tags'").
			WithExitCode(2).
			Err()
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
	if !vendorConfigExists && flg.Everything {
		return errUtils.Build(ErrVendorConfigNotExist).
			WithExplanationf("The '--everything' flag requires a %s file", cfg.AtmosVendorConfigFileName).
			WithHintf("Create a `%s` file in your project root", cfg.AtmosVendorConfigFileName).
			WithHint("Or use `--component` flag to vendor a specific component: `atmos vendor pull -c <component>`").
			WithHint("See https://atmos.tools/core-concepts/vendoring for vendor configuration").
			WithExitCode(1).
			Err()
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

	if len(args) > 0 {
		err := fmt.Errorf("%w", ErrMissingVendorComponent)
		err = errors.WithHintf(err, "Did you mean `atmos vendor pull -c %s`?", args[0])
		err = errors.WithHint(err, "Component name should be specified with `--component` (shorthand: `-c`) flag")
		return err
	}
	err = fmt.Errorf("%w", ErrMissingVendorComponent)
	err = errors.WithHint(err, "Use `--component` flag to vendor a specific component: `atmos vendor pull -c <component>`")
	err = errors.WithHint(err, "Or use `--everything` flag to vendor all components defined in `vendor.yaml`")
	return err
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
