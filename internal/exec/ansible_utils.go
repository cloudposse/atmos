package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetAnsiblePlaybookFromSettings returns an Ansible playbook name from the `settings.ansible.playbook` section in the Atmos component manifest.
func GetAnsiblePlaybookFromSettings(settings *schema.AtmosSectionMapType) (string, error) {
	if settings == nil {
		return "", nil
	}

	var ansibleSection schema.AtmosSectionMapType
	var ansiblePlaybook string
	var ok bool

	if ansibleSection, ok = (*settings)[cfg.AnsibleSectionName].(map[string]any); !ok {
		return "", nil
	}
	if ansiblePlaybook, ok = ansibleSection[cfg.AnsiblePlaybookSectionName].(string); !ok {
		return "", nil
	}
	return ansiblePlaybook, nil
}

// GetAnsibleInventoryFromSettings returns an Ansible inventory from the `settings.ansible.inventory` section in the Atmos component manifest.
func GetAnsibleInventoryFromSettings(settings *schema.AtmosSectionMapType) (string, error) {
	if settings == nil {
		return "", nil
	}

	var ansibleSection schema.AtmosSectionMapType
	var ansibleInventory string
	var ok bool

	if ansibleSection, ok = (*settings)[cfg.AnsibleSectionName].(map[string]any); !ok {
		return "", nil
	}
	if ansibleInventory, ok = ansibleSection[cfg.AnsibleInventorySectionName].(string); !ok {
		return "", nil
	}
	return ansibleInventory, nil
}
