package list

import (
	"slices"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/schema"
)

func resolveInstancesEvaluationPaths(ac *schema.AtmosConfiguration, columns []column.Config, opts *InstancesCommandOptions) [][]string {
	sections := resolveInstancesEvalSections(ac, columns, opts)
	if sections == nil {
		return nil
	}
	paths := column.RequiredPaths(columns)
	if paths == nil {
		return nil
	}
	paths = append(paths, []string{"metadata"})
	if slices.Contains(sections, "settings") {
		paths = append(paths, []string{"settings"})
	}
	if opts.closureRequested() {
		paths = append(paths, []string{"dependencies"}, []string{"settings", "depends_on"})
	}
	return paths
}
