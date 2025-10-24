package toolchain

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/toolchain/registry"
)

// Tool is a type alias for registry.Tool for backward compatibility.
// New code should import and use toolchain/registry.Tool directly.
type Tool = registry.Tool

// File is a type alias for registry.File for backward compatibility.
type File = registry.File

// Override is a type alias for registry.Override for backward compatibility.
type Override = registry.Override

// SupportedIf is a type alias for registry.SupportedIf for backward compatibility.
type SupportedIf = registry.SupportedIf

// ToolRegistry represents the structure of a tool registry YAML file.
// This is kept in toolchain for legacy reasons.
type ToolRegistry = registry.ToolRegistryFile

// AquaPackage is a type alias for registry.AquaPackage for backward compatibility.
type AquaPackage = registry.AquaPackage

// ChecksumConfig is a type alias for registry.ChecksumConfig for backward compatibility.
type ChecksumConfig = registry.ChecksumConfig

// VersionOverride is a type alias for registry.VersionOverride for backward compatibility.
type VersionOverride = registry.VersionOverride

// AquaRegistryFile is a type alias for registry.AquaRegistryFile for backward compatibility.
type AquaRegistryFile = registry.AquaRegistryFile

// LocalConfig is a type alias for registry.LocalConfig for backward compatibility.
type LocalConfig = registry.LocalConfig

// LocalTool is a type alias for registry.LocalTool for backward compatibility.
type LocalTool = registry.LocalTool

// LocalVersionConstraint is a type alias for registry.LocalVersionConstraint for backward compatibility.
type LocalVersionConstraint = registry.LocalVersionConstraint

// LocalConfigManager is a type alias for registry.LocalConfigManager for backward compatibility.
type LocalConfigManager = registry.LocalConfigManager

// toolToYAML converts a Tool struct to YAML string representation.
func toolToYAML(tool *Tool) (string, error) {
	yamlData, err := yaml.Marshal(tool)
	if err != nil {
		return "", err
	}
	return string(yamlData), nil
}

// getEvaluatedToolYAML creates a YAML representation with all templates processed.
func getEvaluatedToolYAML(tool *Tool, version string, installer *Installer) (string, error) {
	// Create a copy of the tool with processed templates
	evaluatedTool := *tool

	// Process asset/URL templates using the existing buildAssetURL function
	if tool.Asset != "" || tool.URL != "" {
		processedURL, err := installer.buildAssetURL(tool, version)
		if err != nil {
			return "", fmt.Errorf("failed to process asset/URL template: %w", err)
		}
		evaluatedTool.Asset = processedURL
		evaluatedTool.URL = processedURL
	}

	// Set the version field to show what version was used for evaluation
	evaluatedTool.Version = version

	// Marshal the evaluated tool to YAML
	yamlData, err := yaml.Marshal(evaluatedTool)
	if err != nil {
		return "", err
	}

	return string(yamlData), nil
}
