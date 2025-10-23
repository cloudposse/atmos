package toolchain

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	DefaultToolVersionsFilePath = ".tool-versions"
	DefaultToolsDir             = ".tools"
	DefaultToolsConfig          = ".tools-config"
)

// Define checkmark styles for use across the application.
var (
	checkMark = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D700")).SetString("✓")
	xMark     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).SetString("x")
)

var atmosConfig *schema.AtmosConfiguration

func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfig = config
}

// GetToolVersionsFilePath returns the path to the tool-versions file.
func GetToolVersionsFilePath() string {
	if atmosConfig == nil || atmosConfig.Toolchain.FilePath == "" {
		return DefaultToolVersionsFilePath
	}
	return atmosConfig.Toolchain.FilePath
}

// GetToolsDirPath returns the path to the tools directory.
func GetToolsDirPath() string {
	if atmosConfig == nil || atmosConfig.Toolchain.ToolsDir == "" {
		return DefaultToolsDir
	}
	return atmosConfig.Toolchain.ToolsDir
}

// GetToolsConfigFilePath returns the path to the tools configuration file.
func GetToolsConfigFilePath() string {
	if atmosConfig == nil || atmosConfig.Toolchain.ToolsConfigFile == "" {
		return DefaultToolsConfig
	}
	return atmosConfig.Toolchain.ToolsConfigFile
}
