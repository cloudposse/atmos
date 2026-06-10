// Ansible playbook CLI docs: https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html.

package ansible

import (
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
)

// playbookCmd represents the `atmos ansible playbook` command.
var playbookCmd = &cobra.Command{
	Use:     "playbook",
	Aliases: []string{"pb"},
	Args:    validatePlaybookArgs,
	Short:   "Run Ansible playbooks for configuration management.",
	Long: `This command runs an Ansible playbook, applying configuration changes to target hosts.

Example usage:
  atmos ansible playbook <component> --stack <stack> [options]
  atmos ansible playbook <component> --stack <stack> --playbook <playbook.yml> [options]
  atmos ansible playbook <component> -s <stack> -p <playbook.yml> [options]

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html
`,
	// FParseErrWhitelist allows unknown flags to pass through to ansible-playbook.
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE:               runPlaybook,
}

// validatePlaybookArgs requires exactly one component argument, counting only
// the positional arguments that appear before a "--" separator.
//
// Everything after "--" is forwarded verbatim to ansible-playbook
// (e.g. `atmos ansible playbook <component> -s <stack> -- --check --tags foo`),
// so those tokens must not be counted as Atmos positional arguments. The
// previous cobra.ExactArgs(1) validator counted them, so any invocation using
// "--" failed validation; the root UsageFunc then rendered that failure as the
// misleading "Unknown command <component>" error. cobra.ArgsLenAtDash() returns
// -1 when no "--" is present, in which case the full argument slice is checked.
func validatePlaybookArgs(cmd *cobra.Command, args []string) error {
	componentArgs := args
	if dashIndex := cmd.ArgsLenAtDash(); dashIndex >= 0 {
		componentArgs = args[:dashIndex]
	}
	if len(componentArgs) != 1 {
		return fmt.Errorf("%w (received %d)", errUtils.ErrInvalidComponentArgument, len(componentArgs))
	}
	return nil
}

// runPlaybook executes the ansible playbook command.
func runPlaybook(cmd *cobra.Command, args []string) error {
	// Initialize config and stacks info.
	info := initConfigAndStacksInfo(cmd, "playbook", args)

	// Get ansible-specific flags.
	cliFlags := getAnsibleFlags(cmd)

	// Get the ansible component provider from the registry.
	// The provider is guaranteed to be registered via pkg/component/ansible/ansible.go's init(),
	// which is invoked when the package is imported in cmd/root.go.
	provider := component.MustGetProvider("ansible")

	// Build execution context for the component provider.
	ctx := &component.ExecutionContext{
		ComponentType:       "ansible",
		Component:           info.ComponentFromArg,
		Stack:               info.Stack,
		Command:             "ansible",
		SubCommand:          "playbook",
		ConfigAndStacksInfo: info,
		Args:                args,
		Flags: map[string]any{
			"playbook":  cliFlags.Playbook,
			"inventory": cliFlags.Inventory,
		},
	}

	// Execute via component registry.
	return provider.Execute(ctx)
}
