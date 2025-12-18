package vendoring

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// updateFlags holds flags specific to vendor update command.
type updateFlags struct {
	Check         bool
	Pull          bool
	Component     string
	Tags          []string
	ComponentType string
	Outdated      bool
}

// Update executes the vendor update operation with typed params.
func Update(atmosConfig *schema.AtmosConfiguration, params *UpdateParams) error {
	defer perf.Track(atmosConfig, "vendor.Update")()

	// Convert params to internal updateFlags format.
	var tags []string
	if params.Tags != "" {
		tags = splitAndTrim(params.Tags, ",")
	}

	flags := &updateFlags{
		Check:         params.Check,
		Pull:          params.Pull,
		Component:     params.Component,
		Tags:          tags,
		ComponentType: params.ComponentType,
		Outdated:      params.Outdated,
	}

	// Execute vendor update.
	return executeVendorUpdate(atmosConfig, flags)
}

// executeVendorUpdate performs the vendor update logic.
func executeVendorUpdate(atmosConfig *schema.AtmosConfiguration, flags *updateFlags) error {
	defer perf.Track(atmosConfig, "vendor.executeVendorUpdate")()

	// Determine the vendor config file path.
	vendorConfigFileName := cfg.AtmosVendorConfigFileName
	if atmosConfig.Vendor.BasePath != "" {
		vendorConfigFileName = atmosConfig.Vendor.BasePath
	}

	// Read the main vendor config.
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		vendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}

	if !vendorConfigExists {
		// Try component vendor config if no main vendor config.
		if flags.Component != "" {
			return executeComponentVendorUpdate(atmosConfig, flags)
		}
		return fmt.Errorf("%w: %s", errUtils.ErrVendorConfigNotFound, vendorConfigFileName)
	}

	// NOTE: Vendor update functionality is planned for a future release.
	// This PR establishes the foundation (command structure, provider interfaces, YAML updater)
	// while vendor diff provides immediate value for reviewing changes before updating.
	//
	// TODO: Implement actual update logic:
	// 1. Process imports and get sources from vendorConfig.
	// 2. Filter sources by component/tags using flags.
	// 3. Check for updates using Git provider (version comparison).
	// 4. Display results via TUI (show current vs. available versions).
	// 5. Update YAML files using updateYAMLVersion if not --check.
	// 6. Execute vendor pull if --pull flag is set.

	// Use vendorConfig and foundVendorConfigFile to avoid "declared and not used" error.
	// These will be used once the update logic is implemented.
	_ = vendorConfig
	_ = foundVendorConfigFile

	return errUtils.ErrNotImplemented
}

// executeComponentVendorUpdate handles vendor update for component.yaml files.
// NOTE: Component-level vendor update is planned for a future release.
func executeComponentVendorUpdate(atmosConfig *schema.AtmosConfiguration, flags *updateFlags) error {
	defer perf.Track(atmosConfig, "vendor.executeComponentVendorUpdate")()

	// TODO: Implement component vendor update:
	// 1. Read component.yaml from components/{flags.ComponentType}/{flags.Component}/component.yaml.
	// 2. Extract version and source information.
	// 3. Check for updates using Git provider.
	// 4. Update component.yaml if newer version available.
	_ = flags

	return errUtils.ErrNotImplemented
}

// splitAndTrim splits a string by delimiter and trims whitespace from each element.
func splitAndTrim(s, delimiter string) []string {
	if s == "" {
		return nil
	}

	parts := []string{}
	for _, part := range strings.Split(s, delimiter) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	return parts
}
