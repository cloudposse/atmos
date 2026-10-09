package dependencies

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// componentInfoSectionKey and componentTypeKey locate the component type inside
// sections produced by describe-component.
const (
	componentInfoSectionKey = "component_info"
	componentTypeKey        = "component_type"
)

// selectedComponentTools returns the bare executable names a component run
// selects from the project's tool versions: the component's executable, plus the
// `helm` companion for helmfile components.
//
// The executable comes from the component's `command` section, then from
// `components.<type>.command` in atmos.yaml, then from the type's default. An
// executable given as a path selects nothing because no toolchain tool can
// provide a specific file. Native Kubernetes components render and apply
// in-process and select nothing; native Helm components use the Helm SDK and
// helm-binary passthrough, so they select `helm`.
func selectedComponentTools(atmosConfig *schema.AtmosConfiguration, componentType string, section map[string]any) []string {
	switch componentType {
	case cfg.KubernetesComponentType:
		return nil
	case cfg.HelmComponentType:
		return []string{"helm"}
	}

	var selected []string
	if executable := componentExecutable(atmosConfig, componentType, section); executable != "" && !isPathLike(executable) {
		selected = append(selected, normalizeExecutable(executable))
	}
	if componentType == cfg.HelmfileComponentType {
		selected = append(selected, "helm")
	}
	return selected
}

// componentExecutable resolves the executable of a component: the `command`
// section, then the atmos.yaml `components.<type>.command`, then the type default.
func componentExecutable(atmosConfig *schema.AtmosConfiguration, componentType string, section map[string]any) string {
	if command, ok := section[cfg.CommandSectionName].(string); ok && command != "" {
		return command
	}
	if atmosConfig != nil {
		if command := configuredCommand(&atmosConfig.Components, componentType); command != "" {
			return command
		}
	}
	return defaultExecutable(componentType)
}

func configuredCommand(components *schema.Components, componentType string) string {
	switch componentType {
	case cfg.TerraformComponentType:
		return components.Terraform.Command
	case cfg.HelmfileComponentType:
		return components.Helmfile.Command
	case cfg.PackerComponentType:
		return components.Packer.Command
	case cfg.AnsibleComponentType:
		return components.Ansible.Command
	}
	return ""
}

// defaultExecutable is the executable a component type runs when nothing
// overrides it.
func defaultExecutable(componentType string) string {
	switch componentType {
	case cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType, cfg.AnsibleComponentType:
		return componentType
	}
	return ""
}

// componentTypeFromSections reads component_info.component_type from sections
// produced by describe-component.
func componentTypeFromSections(sections map[string]any) string {
	info, ok := sections[componentInfoSectionKey].(map[string]any)
	if !ok {
		return ""
	}
	componentType, _ := info[componentTypeKey].(string)
	return componentType
}
