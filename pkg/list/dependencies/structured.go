package dependencies

import (
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// componentRef identifies a component in a stack in structured output.
type componentRef struct {
	Component string `json:"component" yaml:"component"`
	Stack     string `json:"stack" yaml:"stack"`
}

// dependencyEntry is the structured (JSON/YAML) representation of one component
// and its direct dependency relationships. DependsOn/RequiredBy are nil-pointers
// (omitted) when the requested direction does not include them, and non-nil
// pointers to potentially-empty slices when the direction is included. This
// distinguishes a leaf/root node (empty array []) from an excluded direction
// (field absent), preserving the contract described in this file.
type dependencyEntry struct {
	Component  string          `json:"component" yaml:"component"`
	Stack      string          `json:"stack" yaml:"stack"`
	DependsOn  *[]componentRef `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	RequiredBy *[]componentRef `json:"required_by,omitempty" yaml:"required_by,omitempty"`
}

// renderStructured produces JSON or YAML output listing each top-level component
// with its direct dependencies and/or dependents.
func renderStructured(graph *dependency.Graph, tops []*dependency.Node, opts Options) (string, error) {
	defer perf.Track(nil, "dependencies.renderStructured")()

	entries := make([]dependencyEntry, 0, len(tops))
	for _, node := range tops {
		entry := dependencyEntry{Component: node.Component, Stack: node.Stack}
		if opts.Direction == DirectionForward || opts.Direction == DirectionBoth {
			refs := refsFor(graph, node.Dependencies)
			entry.DependsOn = &refs
		}
		if opts.Direction == DirectionReverse || opts.Direction == DirectionBoth {
			refs := refsFor(graph, node.Dependents)
			entry.RequiredBy = &refs
		}
		entries = append(entries, entry)
	}

	switch opts.Format {
	case string(format.FormatJSON):
		return u.ConvertToJSON(entries)
	case string(format.FormatYAML):
		return u.ConvertToYAML(entries)
	default:
		return "", fmt.Errorf("%w: %q", errUtils.ErrInvalidFormat, opts.Format)
	}
}

// refsFor resolves a list of node IDs to sorted componentRefs, skipping IDs that
// are not present in the graph. It always returns a non-nil slice so that an
// empty result serialises as [] rather than null, allowing callers to distinguish
// a leaf/root node (empty array) from an excluded direction (nil pointer field).
func refsFor(graph *dependency.Graph, ids []string) []componentRef {
	refs := make([]componentRef, 0, len(ids))
	for _, id := range ids {
		node, exists := graph.GetNode(id)
		if !exists {
			continue
		}
		refs = append(refs, componentRef{Component: node.Component, Stack: node.Stack})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Stack != refs[j].Stack {
			return refs[i].Stack < refs[j].Stack
		}
		return refs[i].Component < refs[j].Component
	})
	return refs
}
