package ansible

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// AnsibleFlags returns a registry with flags specific to Ansible commands.
// Includes common flags plus Ansible-specific flags.
func AnsibleFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "ansible.AnsibleFlags")()

	registry := flags.CommonFlags()
	registerAnsibleSpecificFlags(registry)
	return registry
}

// registerAnsibleSpecificFlags adds Ansible-specific flags to the registry.
func registerAnsibleSpecificFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        "playbook",
		Shorthand:   "p",
		Default:     "",
		Description: "Ansible playbook to execute",
		EnvVars:     []string{"ATMOS_ANSIBLE_PLAYBOOK"},
	})

	registry.Register(&flags.StringFlag{
		Name:        "inventory",
		Shorthand:   "i",
		Default:     "",
		Description: "Ansible inventory source",
		EnvVars:     []string{"ATMOS_ANSIBLE_INVENTORY"},
	})
}

// WithAnsibleFlags returns a flags.Option that adds all Ansible-specific flags.
func WithAnsibleFlags() flags.Option {
	defer perf.Track(nil, "ansible.WithAnsibleFlags")()

	return flags.WithFlagRegistry(AnsibleFlags())
}
