package exec

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	ErrNoVendorSourcesFound       = errors.New("no vendor.yaml found and no component.yaml manifests were discovered under any component type")
	ErrValidateComponentFlag      = errors.New("either '--component' or '--tags' flag can be provided, but not both")
	ErrValidateComponentStackFlag = errors.New("either '--component' or '--stack' flag can be provided, but not both")
	ErrValidateEverythingFlag     = errors.New("'--everything' flag cannot be combined with '--component', '--stack', or '--tags' flags")
	ErrMissingComponent           = errors.New("to vendor a component, the '--component' (shorthand '-c') flag needs to be specified.\n" +
		"Example: atmos vendor pull -c <component>")
)

// ExecuteVendorPullCmd executes `vendor pull` commands.
func ExecuteVendorPullCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteVendorPullCmd")()

	return ExecuteVendorPullCommand(cmd, args)
}

type VendorFlags struct {
	DryRun        bool
	Component     string
	Stack         string
	Tags          []string
	Everything    bool
	ComponentType string
	RefreshLock   bool
	// TypeChanged is true only when --type was explicitly passed, distinguishing "sweep only this
	// type" from "no --type given, sweep every component type" for handleVendorPullSweep.
	TypeChanged bool
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
	if flags.Lookup("refresh-lock") != nil {
		if vendorFlags.RefreshLock, err = flags.GetBool("refresh-lock"); err != nil {
			return vendorFlags, err
		}
	}

	// Set default for 'everything' if no specific flags are provided
	setDefaultEverythingFlag(flags, &vendorFlags)

	// Handle 'type' flag only if it exists
	if flags.Lookup("type") != nil {
		if vendorFlags.ComponentType, err = flags.GetString("type"); err != nil {
			return vendorFlags, err
		}
		vendorFlags.TypeChanged = flags.Changed("type")
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
	if !vendorConfigExists && flg.Everything {
		return handleVendorPullSweep(atmosConfig, flg)
	}
	if vendorConfigExists {
		return ExecuteAtmosVendorInternal(&executeVendorOptions{
			vendorConfigFileName: foundVendorConfigFile,
			dryRun:               flg.DryRun,
			refreshLock:          flg.RefreshLock,
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
		flg.RefreshLock,
	)
}
