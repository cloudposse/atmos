package toolchain

import (
	"os"
	"path/filepath"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

var (
	DefaultToolVersionsFilePath = ".tool-versions"
	DefaultInstallPath          = ".tools"
)

var atmosConfig *schema.AtmosConfiguration

func SetAtmosConfig(config *schema.AtmosConfiguration) {
	defer perf.Track(nil, "toolchain.SetAtmosConfig")()

	atmosConfig = config
}

// GetToolVersionsFilePath returns the path to the tool-versions file.
func GetToolVersionsFilePath() string {
	defer perf.Track(nil, "toolchain.GetToolVersionsFilePath")()

	if atmosConfig == nil || atmosConfig.Toolchain.VersionsFile == "" {
		return DefaultToolVersionsFilePath
	}
	return atmosConfig.Toolchain.VersionsFile
}

// GetInstallPath returns the path where tools are installed.
// By default, it uses XDG Data directory (~/.local/share/atmos/toolchain on Linux/macOS).
// Falls back to .tools if XDG directory cannot be created.
func GetInstallPath() string {
	defer perf.Track(nil, "toolchain.GetInstallPath")()

	// If explicitly configured, use that path
	if atmosConfig != nil && atmosConfig.Toolchain.InstallPath != "" {
		return atmosConfig.Toolchain.InstallPath
	}

	// Try to use XDG-compliant data directory
	dataDir, err := xdg.GetXDGDataDir("toolchain", 0o755)
	if err == nil && dataDir != "" {
		return dataDir
	}

	// Fallback to local .tools directory
	log.Debug("XDG data dir unavailable, falling back to .tools", "error", err)

	// Try using current directory .tools
	cwd, err := os.Getwd()
	if err == nil {
		return filepath.Join(cwd, DefaultInstallPath)
	}

	// Last resort: just return the constant
	return DefaultInstallPath
}
