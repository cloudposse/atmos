package exec

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	ErrVendorConfigNotExist       = errors.New("the '--everything' flag is set, but vendor config file does not exist")
	ErrExecuteVendorDiffCmd       = errors.New("'atmos vendor diff' is not implemented yet")
	ErrValidateComponentFlag      = errors.New("either '--component' or '--tags' flag can be provided, but not both")
	ErrValidateComponentStackFlag = errors.New("either '--component' or '--stack' flag can be provided, but not both")
	ErrValidateEverythingFlag     = errors.New("'--everything' flag cannot be combined with '--component', '--stack', or '--tags' flags")
	ErrMissingComponent           = errors.New("to vendor a component, the '--component' (shorthand '-c') flag needs to be specified.\n" +
		"Example: atmos vendor pull -c <component>")
)

// ExecuteVendorPullCmd executes `vendor pull` commands.
func ExecuteVendorPullCmd(opts *flags.StandardOptions) error {
	defer perf.Track(nil, "exec.ExecuteVendorPullCmd")()

	return ExecuteVendorPullCommand(opts)
}

// ExecuteVendorDiffCmd executes `vendor diff` commands.
func ExecuteVendorDiffCmd(opts *flags.StandardOptions) error {
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
func ExecuteVendorPullCommand(opts *flags.StandardOptions) error {
	defer perf.Track(nil, "exec.ExecuteVendorPullCommand")()

	// Populate ConfigAndStacksInfo with stack from opts to preserve CLI overrides
	info := schema.ConfigAndStacksInfo{
		Stack: opts.Stack,
	}
	processStacks := opts.Stack != ""

	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	// Convert opts to VendorFlags
	vendorFlags := VendorFlags{
		DryRun:        opts.DryRun,
		Component:     opts.Component,
		Stack:         opts.Stack,
		Everything:    opts.Everything,
		ComponentType: opts.Type,
	}

	// Parse tags from comma-separated string
	if opts.Tags != "" {
		vendorFlags.Tags = strings.Split(opts.Tags, ",")
	}

	// Set default for 'everything' if no specific flags are provided
	if !vendorFlags.Everything && vendorFlags.Component == "" && vendorFlags.Stack == "" && len(vendorFlags.Tags) == 0 {
		vendorFlags.Everything = true
	}

	if err := validateVendorFlags(&vendorFlags); err != nil {
		return err
	}

	if vendorFlags.Stack != "" {
		return ExecuteStackVendorInternal(vendorFlags.Stack, vendorFlags.DryRun)
	}

	return handleVendorConfig(&atmosConfig, &vendorFlags, []string{})
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

	if len(args) > 0 {
		q := fmt.Sprintf("Did you mean 'atmos vendor pull -c %s'?", args[0])
		return fmt.Errorf("%w\n%s", ErrMissingComponent, q)
	}
	return ErrMissingComponent
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
