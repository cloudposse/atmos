// Ansible CLI docs: https://docs.ansible.com/ansible/latest/cli/ansible.html.

package ansible

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/component"
	ansibleComp "github.com/cloudposse/atmos/pkg/component/ansible"
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

	// Get the ansible component provider from the registry.
	provider, ok := component.GetProvider("ansible")
	if !ok {
		// Fallback to direct execution if provider not found.
		return ansibleComp.ExecuteVersion(&configAndStacksInfo)
	}

	// Build execution context for the component provider.
	ctx := &component.ExecutionContext{
		ComponentType:       "ansible",
		Command:             "ansible",
		SubCommand:          "version",
		ConfigAndStacksInfo: configAndStacksInfo,
	}

	// Execute via component registry.
	return provider.Execute(ctx)
}
