// Ansible CLI docs: https://docs.ansible.com/ansible/latest/cli/ansible.html.

package ansible

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// versionCmd represents the `atmos ansible version` command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Ansible version information.",
	Long: `This command shows the Ansible version, config file location, and module search path.

Example usage:
  atmos ansible version

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible.html
`,
	RunE: runVersion,
}

// runVersion executes the ansible version command.
func runVersion(cmd *cobra.Command, _ []string) error {
	// Parse global flags to honor config selection flags.
	v := viper.GetViper()
	globalFlags := flags.ParseGlobalFlags(cmd, v)
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}

	// For version command, we just need to execute ansible --version directly.
	// No need for full Atmos config or stack processing.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	// Get ansible command from config, defaulting to "ansible".
	command := atmosConfig.Components.Ansible.Command
	if command == "" {
		command = "ansible"
	}

	// Execute ansible --version directly.
	return e.ExecuteShellCommand(
		atmosConfig,
		command,
		[]string{"--version"},
		"",    // dir
		nil,   // env
		false, // dryRun
		"",    // redirectStdError
	)
}
