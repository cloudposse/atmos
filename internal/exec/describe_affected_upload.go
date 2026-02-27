package exec

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// StripAffectedForUpload removes fields from affected stacks that aren't used
// by Atmos Pro processing. This significantly reduces payload size
// (typically 70-75% reduction) to stay within serverless function payload limits.
//
// Fields kept:
//   - component: Stack identification
//   - stack: Stack identification
//   - included_in_dependents: Used in filtering logic
//   - dependents: Nested stack processing (recursively stripped)
//   - settings.pro: Workflow dispatch configuration
//
// Fields removed:
//   - settings.depends_on: Dependency graph (largest contributor to size)
//   - settings.github: Not used by downstream handlers
//   - component_type, component_path: Not used in downstream processing
//   - namespace, tenant, environment, stage: Redundant (encoded in stack name)
//   - stack_slug, affected: Not used in downstream processing
func StripAffectedForUpload(affected []schema.Affected) []schema.Affected {
	result := make([]schema.Affected, len(affected))
	for i, a := range affected {
		result[i] = stripAffected(a)
	}
	return result
}

func stripAffected(a schema.Affected) schema.Affected {
	return schema.Affected{
		Component:            a.Component,
		Stack:                a.Stack,
		IncludedInDependents: a.IncludedInDependents,
		Dependents:           stripDependents(a.Dependents),
		Settings:             stripSettings(a.Settings),
	}
}

func stripDependents(dependents []schema.Dependent) []schema.Dependent {
	if len(dependents) == 0 {
		return []schema.Dependent{}
	}
	result := make([]schema.Dependent, len(dependents))
	for i, d := range dependents {
		result[i] = stripDependent(d)
	}
	return result
}

func stripDependent(d schema.Dependent) schema.Dependent {
	return schema.Dependent{
		Component:            d.Component,
		Stack:                d.Stack,
		IncludedInDependents: d.IncludedInDependents,
		Dependents:           stripDependents(d.Dependents),
		Settings:             stripSettings(d.Settings),
	}
}

func stripSettings(settings schema.AtmosSectionMapType) schema.AtmosSectionMapType {
	if settings == nil {
		return nil
	}

	// Only keep settings.pro if it exists
	pro, hasPro := settings["pro"]
	if !hasPro {
		return nil
	}

	return schema.AtmosSectionMapType{
		"pro": pro,
	}
}
