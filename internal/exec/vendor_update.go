package exec

import (
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteVendorUpdateCmd executes `vendor update` commands.
func ExecuteVendorUpdateCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteVendorUpdateCmd")()

	// Initialize Atmos configuration
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	// Vendor update doesn't use stack flag
	processStacks := false

	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	// Parse vendor update flags
	updateFlags, err := parseVendorUpdateFlags(cmd)
	if err != nil {
		return err
	}

	// Execute vendor update
	return executeVendorUpdate(&atmosConfig, updateFlags)
}

// VendorUpdateFlags holds flags specific to vendor update command.
type VendorUpdateFlags struct {
	Check         bool
	Pull          bool
	Component     string
	Tags          []string
	ComponentType string
	Outdated      bool
}

// parseVendorUpdateFlags parses flags from the vendor update command.
func parseVendorUpdateFlags(cmd *cobra.Command) (*VendorUpdateFlags, error) {
	flags := cmd.Flags()

	checkOnly, err := flags.GetBool("check")
	if err != nil {
		return nil, err
	}

	pull, err := flags.GetBool("pull")
	if err != nil {
		return nil, err
	}

	component, err := flags.GetString("component")
	if err != nil {
		return nil, err
	}

	tagsCsv, err := flags.GetString("tags")
	if err != nil {
		return nil, err
	}

	var tags []string
	if tagsCsv != "" {
		tags = splitAndTrim(tagsCsv, ",")
	}

	componentType, err := flags.GetString("type")
	if err != nil {
		return nil, err
	}

	outdated, err := flags.GetBool("outdated")
	if err != nil {
		return nil, err
	}

	return &VendorUpdateFlags{
		Check:         checkOnly,
		Pull:          pull,
		Component:     component,
		Tags:          tags,
		ComponentType: componentType,
		Outdated:      outdated,
	}, nil
}

// executeVendorUpdate performs the vendor update logic.
func executeVendorUpdate(atmosConfig *schema.AtmosConfiguration, flags *VendorUpdateFlags) error {
	defer perf.Track(atmosConfig, "exec.executeVendorUpdate")()

	// Determine the vendor config file path
	vendorConfigFileName := cfg.AtmosVendorConfigFileName
	if atmosConfig.Vendor.BasePath != "" {
		vendorConfigFileName = atmosConfig.Vendor.BasePath
	}

	// Read the main vendor config
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		vendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}

	if !vendorConfigExists {
		// Try component vendor config if no main vendor config
		if flags.Component != "" {
			return executeComponentVendorUpdate(atmosConfig, flags)
		}
		return fmt.Errorf("%w: %s", errUtils.ErrVendorConfigNotFound, vendorConfigFileName)
	}

	// TODO: Process vendor config and check for updates
	// This is a placeholder - will be implemented with vendor_version_check.go
	fmt.Printf("Checking for vendor updates in %s...\n", foundVendorConfigFile)
	fmt.Printf("Flags: check=%v, pull=%v, component=%s, tags=%v, outdated=%v\n",
		flags.Check, flags.Pull, flags.Component, flags.Tags, flags.Outdated)

	// TODO: Implement actual update logic
	// 1. Process imports and get sources
	// 2. Filter sources by component/tags
	// 3. Check for updates using Git
	// 4. Display results (TUI)
	// 5. Update YAML files if not --check
	// 6. Execute vendor pull if --pull

	// Use vendorConfig to avoid "declared and not used" error
	_ = vendorConfig

	return errUtils.ErrNotImplemented
}

// executeComponentVendorUpdate handles vendor update for component.yaml files.
func executeComponentVendorUpdate(atmosConfig *schema.AtmosConfiguration, flags *VendorUpdateFlags) error {
	defer perf.Track(atmosConfig, "exec.executeComponentVendorUpdate")()

	// TODO: Implement component vendor update
	// When implemented, use flags.ComponentType (default: "terraform")
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
	for _, part := range splitString(s, delimiter) {
		trimmed := trimString(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s, delimiter string) []string {
	result := []string{}
	current := ""
	for _, char := range s {
		if string(char) == delimiter {
			result = append(result, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	result = append(result, current)
	return result
}

//nolint:revive // Simple string trim implementation.
func trimString(s string) string {
	start := 0
	end := len(s)

	// Trim leading whitespace
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	// Trim trailing whitespace
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}
