package exec

// nonTemplatedComponentSections lists the component sections that Atmos computes from sources
// outside the stack manifests. Today that is `component_info`, whose `terraform_config`
// sub-section is the Terraform source code parsed by `terraform-config-inspect` (variable and
// output names, types, and descriptions).
//
// Those values are Terraform code, not Atmos configuration, and they may legitimately contain
// `Go` template delimiters. A GCP resource name format such as
// `projects/{{project}}/locations/{{location}}/services/{{name}}` in a Terraform `description`
// used to abort every Atmos command with `function "project" not defined`.
// Atmos templating belongs to the abstraction above Terraform modules, so these sections are
// excluded from template rendering. See https://github.com/cloudposse/atmos/issues/2145.
//
// The sections are still part of the template *context*, so stack manifests can keep
// referencing `{{ .component_info.component_path }}` and friends.
var nonTemplatedComponentSections = []string{componentInfoKey}

// splitNonTemplatedSections returns a shallow copy of `componentSection` without the sections
// that must not be rendered as `Go` templates, along with the sections that were left out so
// that `restoreNonTemplatedSections` can put them back after template processing.
//
// The input map is never modified. When there is nothing to exclude, the input map is returned
// as-is together with a nil `excluded` map.
func splitNonTemplatedSections(componentSection map[string]any) (filtered, excluded map[string]any) {
	for _, section := range nonTemplatedComponentSections {
		value, ok := componentSection[section]
		if !ok {
			continue
		}

		if excluded == nil {
			excluded = make(map[string]any, len(nonTemplatedComponentSections))
		}

		excluded[section] = value
	}

	if excluded == nil {
		return componentSection, nil
	}

	filtered = make(map[string]any, len(componentSection))

	for key, value := range componentSection {
		if _, skip := excluded[key]; skip {
			continue
		}

		filtered[key] = value
	}

	return filtered, excluded
}

// restoreNonTemplatedSections copies the sections removed by `splitNonTemplatedSections` back
// into the processed component section. It is a no-op when nothing was excluded.
func restoreNonTemplatedSections(componentSection, excluded map[string]any) {
	for section, value := range excluded {
		componentSection[section] = value
	}
}
