package taskgraph

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RefsFromDependencies converts a schema.Dependencies' Commands/Workflows entries into Refs,
// the shared conversion needed by both custom-command and workflow call sites so neither
// package has to duplicate this mapping. Takes deps by value (not *schema.Dependencies) to keep
// every call site's fluent `x.Dependencies.OrEmpty()` chaining intact -- OrEmpty returns a value,
// and a pointer parameter here would force every caller to introduce an intermediate variable.
//
//nolint:gocritic // see comment above; deps is a small config value, not a hot-path allocation.
func RefsFromDependencies(deps schema.Dependencies) []Ref {
	defer perf.Track(nil, "taskgraph.RefsFromDependencies")()

	refs := make([]Ref, 0, len(deps.Commands)+len(deps.Workflows))
	for i := range deps.Commands {
		refs = append(refs, refFromUnitDependency(KindCommand, &deps.Commands[i]))
	}
	for i := range deps.Workflows {
		refs = append(refs, refFromUnitDependency(KindWorkflow, &deps.Workflows[i]))
	}
	return refs
}

func refFromUnitDependency(kind string, d *schema.UnitDependency) Ref {
	return Ref{
		Kind:  kind,
		Name:  d.Name,
		File:  d.File,
		Flags: d.Flags,
		Args:  d.Args,
		Fail:  d.Fail,
	}
}
