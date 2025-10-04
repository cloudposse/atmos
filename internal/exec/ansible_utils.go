package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetAnsiblePlaybookFromSettings returns an Ansible playbook name from the `settings.ansible.playbook`
// section in the Atmos component manifest.
func GetAnsiblePlaybookFromSettings(settings *schema.AtmosSectionMapType) (string, error) {
	if settings == nil {
		return "", nil
	}

	var ansibleSection schema.AtmosSectionMapType
	var playbook string
	var ok bool

	if ansibleSection, ok = (*settings)[cfg.AnsibleSectionName].(map[string]any); !ok {
		return "", nil
	}
	if playbook, ok = ansibleSection[cfg.AnsiblePlaybookSectionName].(string); !ok {
		return "", nil
	}
	return playbook, nil
}

// GetAnsibleInventoryFromSettings returns an Ansible inventory path/name from `settings.ansible.inventory`.
func GetAnsibleInventoryFromSettings(settings *schema.AtmosSectionMapType) (string, error) {
	if settings == nil {
		return "", nil
	}

	var ansibleSection schema.AtmosSectionMapType
	var inventory string
	var ok bool

	if ansibleSection, ok = (*settings)[cfg.AnsibleSectionName].(map[string]any); !ok {
		return "", nil
	}
	if inventory, ok = ansibleSection[cfg.AnsibleInventorySectionName].(string); !ok {
		return "", nil
	}
	return inventory, nil
}
