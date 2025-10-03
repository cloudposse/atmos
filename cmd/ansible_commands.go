package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

func getAnsibleCommands() []*cobra.Command {
	return []*cobra.Command{
		{
			Use:   "run",
			Short: "Run ansible-playbook for a component",
			Long: `The 'atmos ansible run' command executes 'ansible-playbook' for the specified component in a stack.

What it does:
- Processes the Atmos stack to resolve variables, settings, and environment
- Writes variables to '<context>-<component>.ansible.vars.yaml' in the component directory
- Runs 'ansible-playbook -e @<varfile> [args] <playbook>' in the component working directory

Defaults:
- Uses 'settings.ansible.playbook' as the playbook if not provided via trailing args
- Honors 'components.ansible.command' override from atmos.yaml; defaults to 'ansible-playbook'

Examples:
  atmos ansible run web -s tenant/ue2/dev -- --check
  atmos ansible run web -s tenant/ue2/dev -- -l app -t setup
  atmos ansible run web -s tenant/ue2/dev -- site.yml -e env=dev`,
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "inventory",
			Short: "Run ansible-inventory in component context",
			Long: `The 'atmos ansible inventory' command runs 'ansible-inventory' from the component directory.

What it does:
- Processes the Atmos stack to resolve environment and settings
- Uses 'settings.ansible.inventory' if defined; otherwise defaults to 'inventory'
- Defaults to '--list' when no trailing arguments are provided

Examples:
  atmos ansible inventory web -s tenant/ue2/dev
  atmos ansible inventory web -s tenant/ue2/dev -- --graph
  atmos ansible inventory web -s tenant/ue2/dev -- -i inventories/prod --list`,
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "galaxy",
			Short: "Run ansible-galaxy in component context",
			Long: `The 'atmos ansible galaxy' command runs 'ansible-galaxy' from the component directory.

What it does:
- Processes the Atmos stack to resolve environment
- If no_args are provided, defaults to 'install -r requirements.yml'

Examples:
  atmos ansible galaxy web -s tenant/ue2/dev
  atmos ansible galaxy web -s tenant/ue2/dev -- install -r requirements.yml
  atmos ansible galaxy web -s tenant/ue2/dev -- collection install community.general`,
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "doc",
			Short: "Run ansible-doc in component context",
			Long: `The 'atmos ansible doc' command runs 'ansible-doc' from the component directory.

Examples:
  atmos ansible doc web -s tenant/ue2/dev -- -l
  atmos ansible doc web -s tenant/ue2/dev -- ping`,
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "config",
			Short: "Run ansible-config in component context",
			Long: `The 'atmos ansible config' command runs 'ansible-config' from the component directory.

Examples:
  atmos ansible config web -s tenant/ue2/dev -- dump
  atmos ansible config web -s tenant/ue2/dev -- view DEFAULT_ROLES_PATH`,
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:   "vault",
			Short: "Run ansible-vault in component context",
			Long: `The 'atmos ansible vault' command runs 'ansible-vault' from the component directory.

Examples:
  atmos ansible vault web -s tenant/ue2/dev -- encrypt group_vars/all/vault.yml
  atmos ansible vault web -s tenant/ue2/dev -- edit group_vars/all/vault.yml
  atmos ansible vault web -s tenant/ue2/dev -- view group_vars/all/vault.yml`,
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
		{
			Use:         "version",
			Short:       "Show ansible-playbook version",
			Long:        "Displays the current version of ansible-playbook installed on the system.",
			Annotations: map[string]string{
				"nativeCommand": "true",
			},
		},
	}
}

// attachAnsibleCommands attaches static Ansible commands to ansibleCmd
func attachAnsibleCommands(parentCmd *cobra.Command) {
	commands := getAnsibleCommands()

	for _, c := range commands {
		c.FParseErrWhitelist.UnknownFlags = true
		c.DisableFlagParsing = true
		c.ValidArgsFunction = ComponentsArgCompletion

		switch c.Use {
		case "run":
			c.RunE = func(cmd *cobra.Command, args []string) error {
				return ansibleRun(parentCmd, cmd, "run", args)
			}
		case "version":
			c.RunE = func(cmd *cobra.Command, args []string) error { return ansibleRun(parentCmd, cmd, "version", args) }
		case "inventory":
			c.RunE = func(cmd *cobra.Command, args []string) error {
				handleHelpRequest(parentCmd, args)
				info := getConfigAndStacksInfo("ansible", parentCmd, append([]string{"inventory"}, args...))
				info.CliArgs = []string{"ansible", "inventory"}
				return e.ExecuteAnsibleInventory(&info)
			}
		case "galaxy":
			c.RunE = func(cmd *cobra.Command, args []string) error {
				handleHelpRequest(parentCmd, args)
				info := getConfigAndStacksInfo("ansible", parentCmd, append([]string{"galaxy"}, args...))
				info.CliArgs = []string{"ansible", "galaxy"}
				return e.ExecuteAnsibleGalaxy(&info)
			}
		case "doc":
			c.RunE = func(cmd *cobra.Command, args []string) error {
				handleHelpRequest(parentCmd, args)
				info := getConfigAndStacksInfo("ansible", parentCmd, append([]string{"doc"}, args...))
				info.CliArgs = []string{"ansible", "doc"}
				return e.ExecuteAnsibleDoc(&info)
			}
		case "config":
			c.RunE = func(cmd *cobra.Command, args []string) error {
				handleHelpRequest(parentCmd, args)
				info := getConfigAndStacksInfo("ansible", parentCmd, append([]string{"config"}, args...))
				info.CliArgs = []string{"ansible", "config"}
				return e.ExecuteAnsibleConfig(&info)
			}
		case "vault":
			c.RunE = func(cmd *cobra.Command, args []string) error {
				handleHelpRequest(parentCmd, args)
				info := getConfigAndStacksInfo("ansible", parentCmd, append([]string{"vault"}, args...))
				info.CliArgs = []string{"ansible", "vault"}
				return e.ExecuteAnsibleVault(&info)
			}
		}

		parentCmd.AddCommand(c)
	}
}
