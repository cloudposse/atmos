package exec

import (
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mergeComponentProvision merges a component's `provision` section from all inheritance layers,
// with the atmos.yaml `settings.provision` block layered in as the lowest-precedence base.
//
// Priority (lowest to highest):
// settings.provision (atmos.yaml global default) → global (e.g. terraform.provision) →
// base component → component → overrides.
//
// The atmos.yaml `settings.provision` block is the documented global default for workdir
// provisioning (see /stacks/components/provision/workdir#global-defaults). It is not part of the
// stack-processed sections, so it is injected here as the lowest-precedence base — any
// component/stack-level `provision.workdir` value (including an explicit `enabled: false`)
// overrides it. See #3197.
func mergeComponentProvision(
	atmosConfig *schema.AtmosConfiguration,
	mergeConfig *schema.AtmosConfiguration,
	opts *ComponentProcessorOptions,
	result *ComponentProcessorResult,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.mergeComponentProvision")()

	provisionSections := []map[string]any{
		opts.GlobalProvisionSection,
		result.BaseComponentProvisionSection,
		result.ComponentProvision,
		result.ComponentOverridesProvision,
	}
	if defaults := globalWorkdirProvisionDefaults(atmosConfig); defaults != nil {
		provisionSections = append([]map[string]any{defaults}, provisionSections...)
	}
	return m.Merge(mergeConfig, provisionSections)
}

// globalWorkdirProvisionDefaults builds the `provision` map contributed by the atmos.yaml
// `settings.provision.workdir` block (the documented global default), or nil when nothing is
// set. Only the `workdir` sub-section is projected — the documented global default for workdir
// provisioning — and only keys the user actually set are included, so an unset global has no
// effect and never introduces `enabled: false` as a spurious base.
func globalWorkdirProvisionDefaults(atmosConfig *schema.AtmosConfiguration) map[string]any {
	if atmosConfig == nil {
		return nil
	}

	workdir := atmosConfig.Settings.Provision.Workdir
	workdirDefaults := map[string]any{}
	if workdir.Enabled {
		workdirDefaults["enabled"] = true
	}
	if workdir.TTL != "" {
		workdirDefaults["ttl"] = workdir.TTL
	}

	if len(workdirDefaults) == 0 {
		return nil
	}
	return map[string]any{"workdir": workdirDefaults}
}
