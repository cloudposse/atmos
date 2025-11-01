package toolchain

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
func GetInstallPath() string {
	defer perf.Track(nil, "toolchain.GetInstallPath")()

	if atmosConfig == nil || atmosConfig.Toolchain.InstallPath == "" {
		return DefaultInstallPath
	}
	return atmosConfig.Toolchain.InstallPath
}
