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

	// TODO: Process vendor config and check for updates.
	// This is a placeholder - will be implemented with vendor_version_check.go.
	fmt.Printf("Checking for vendor updates in %s...\n", foundVendorConfigFile)
	fmt.Printf("Flags: check=%v, pull=%v, component=%s, tags=%v, outdated=%v\n",
		flags.Check, flags.Pull, flags.Component, flags.Tags, flags.Outdated)

	// TODO: Implement actual update logic.
	// 1. Process imports and get sources.
	// 2. Filter sources by component/tags.
	// 3. Check for updates using Git.
	// 4. Display results (TUI).
	// 5. Update YAML files if not --check.
	// 6. Execute vendor pull if --pull.

	// Use vendorConfig to avoid "declared and not used" error.
	_ = vendorConfig

	return errUtils.ErrNotImplemented
}

// executeComponentVendorUpdate handles vendor update for component.yaml files.
func executeComponentVendorUpdate(atmosConfig *schema.AtmosConfiguration, flags *updateFlags) error {
	defer perf.Track(atmosConfig, "vendor.executeComponentVendorUpdate")()

	// TODO: Implement component vendor update.
	// When implemented, use flags.ComponentType (default: "terraform").
	fmt.Printf("Checking for updates in component.yaml for component %s (type: %s)...\n",
		flags.Component, flags.ComponentType)

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
