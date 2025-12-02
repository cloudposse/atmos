package vendoring

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PullParams contains parameters for the Pull operation.
type PullParams struct {
	Component     string
	Stack         string
	ComponentType string
	DryRun        bool
	Tags          string
	Everything    bool
}

// vendorFlags is the internal representation of vendor flags.
type vendorFlags struct {
	DryRun        bool
	Component     string
	Stack         string
	Tags          []string
	Everything    bool
	ComponentType string
}

// Pull executes the vendor pull operation with typed params.
func Pull(atmosConfig *schema.AtmosConfiguration, params *PullParams) error {
	defer perf.Track(atmosConfig, "vendor.Pull")()

	// Convert params to internal vendorFlags format.
	var tags []string
	if params.Tags != "" {
		tags = strings.Split(params.Tags, ",")
	}

	flg := &vendorFlags{
		DryRun:        params.DryRun,
		Component:     params.Component,
		Stack:         params.Stack,
		Tags:          tags,
		Everything:    params.Everything,
		ComponentType: params.ComponentType,
	}

	// Set default for 'everything' if no specific flags are provided.
	if !flg.Everything && flg.Component == "" && flg.Stack == "" && len(flg.Tags) == 0 {
		flg.Everything = true
	}

	// Validate flags.
	if err := validateVendorFlags(flg); err != nil {
		return err
	}

	// Handle stack vendor.
	if flg.Stack != "" {
		return ExecuteStackVendorInternal(flg.Stack, flg.DryRun)
	}

	// Handle config-based vendor.
	return handleVendorConfig(atmosConfig, flg, nil)
}

func validateVendorFlags(flg *vendorFlags) error {
	if flg.Component != "" && flg.Stack != "" {
		return errUtils.ErrValidateComponentStackFlag
	}

	if flg.Component != "" && len(flg.Tags) > 0 {
		return errUtils.ErrValidateComponentFlag
	}

	if flg.Everything && (flg.Component != "" || flg.Stack != "" || len(flg.Tags) > 0) {
		return errUtils.ErrValidateEverythingFlag
	}

	return nil
}

func handleVendorConfig(atmosConfig *schema.AtmosConfiguration, flg *vendorFlags, args []string) error {
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		cfg.AtmosVendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}
	if !vendorConfigExists && flg.Everything {
		return fmt.Errorf("%w: %s", errUtils.ErrVendorConfigNotExist, cfg.AtmosVendorConfigFileName)
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
		return fmt.Errorf("%w\n%s", errUtils.ErrVendorMissingComponent, q)
	}
	return errUtils.ErrVendorMissingComponent
}

func handleComponentVendor(atmosConfig *schema.AtmosConfiguration, flg *vendorFlags) error {
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
