package workdir

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/schema"
)

// atmosConfigPtr will be set by SetAtmosConfig before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the workdir command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// workdirCmd represents the workdir command.
var workdirCmd = &cobra.Command{
	Use:   "workdir",
	Short: "Manage component working directories",
	Long:  `List, describe, show, and clean component working directories.`,
}

func init() {
	// Mark this subcommand as experimental.
	workdirCmd.Annotations = map[string]string{"experimental": "true"}

	// Add subcommands.
	workdirCmd.AddCommand(listCmd)
	workdirCmd.AddCommand(describeCmd)
	workdirCmd.AddCommand(showCmd)
	workdirCmd.AddCommand(cleanCmd)
}

// GetWorkdirCommand returns the workdir command for parent registration.
func GetWorkdirCommand() *cobra.Command {
	return workdirCmd
}

// buildConfigAndStacksInfo creates a ConfigAndStacksInfo struct from global flags.
// This ensures that config selection flags (--base-path, --config, --config-path, --profile)
// are properly honored when initializing CLI config.
func buildConfigAndStacksInfo(cmd *cobra.Command, v *viper.Viper) schema.ConfigAndStacksInfo {
	globalFlags := flags.ParseGlobalFlags(cmd, v)
	return buildConfigAndStacksInfoFromFlags(&globalFlags)
}

// buildConfigAndStacksInfoFromFlags creates a ConfigAndStacksInfo struct from parsed global flags.
func buildConfigAndStacksInfoFromFlags(globalFlags *global.Flags) schema.ConfigAndStacksInfo {
	if globalFlags == nil {
		return schema.ConfigAndStacksInfo{}
	}
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}
