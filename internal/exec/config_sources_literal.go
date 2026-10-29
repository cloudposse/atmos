package exec

import (
	"maps"
	"slices"

	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

// restoreConfigSourceLiterals decodes provenance values without evaluating them or
// mutating the typed maps and dependency slices that can reference cached source data.
func restoreConfigSourceLiterals(value any) any {
	sources, ok := value.(schema.ConfigSources)
	if !ok {
		return literalsource.Restore(value)
	}
	result := maps.Clone(sources)
	for section, items := range sources {
		restored := maps.Clone(items)
		for name, item := range items {
			item.FinalValue = literalsource.Restore(item.FinalValue)
			item.StackDependencies = slices.Clone(item.StackDependencies)
			for i := range item.StackDependencies {
				item.StackDependencies[i].VariableValue = literalsource.Restore(item.StackDependencies[i].VariableValue)
			}
			restored[name] = item
		}
		result[section] = restored
	}
	return result
}
