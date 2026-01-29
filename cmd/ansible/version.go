// https://docs.ansible.com/ansible/latest/cli/ansible.html

package ansible

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
func runVersion(cmd *cobra.Command, args []string) error {
	// For version command, we just need to execute ansible --version directly.
	// No need for full Atmos config or stack processing.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
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
