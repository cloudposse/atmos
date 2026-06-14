// Ansible playbook CLI docs: https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html.

package ansible

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/flags"
)

// playbookCmd represents the `atmos ansible playbook` command.
var playbookCmd = &cobra.Command{
	Use:     "playbook",
	Aliases: []string{"pb"},
	Short:   "Run Ansible playbooks for configuration management.",
	Long: `This command runs an Ansible playbook, applying configuration changes to target hosts.

Example usage:
  atmos ansible playbook <component> --stack <stack> [options]
  atmos ansible playbook <component> --stack <stack> --playbook <playbook.yml> [options]
  atmos ansible playbook <component> -s <stack> -p <playbook.yml> [options]
  atmos ansible playbook <component> -s <stack> -- --check --tags <tags>

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html
`,
	// FParseErrWhitelist allows unknown flags to pass through to ansible-playbook.
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE:               runPlaybook,
}

func init() {
	// Declare the positional-args contract through the flag handler. The
	// builder-generated validator is separator-aware: pass-through args after
	// "--" (forwarded to ansible-playbook) are not counted as positional args.
	argsBuilder := flags.NewPositionalArgsBuilder()
	argsBuilder.AddArg(&flags.PositionalArgSpec{
		Name:        "component",
		Description: "Ansible component",
		Required:    true,
		TargetField: "Component",
	})
	_, validator, usage := argsBuilder.Build()
	playbookCmd.Use = "playbook " + usage
	playbookCmd.Args = validator
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
