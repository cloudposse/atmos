package lifecycle

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
)

const (
	reasonLocked        = "locked; excluded"
	reasonDeployDeleted = "deleted definition; infrastructure teardown requires --allow-destroy and deleted-definition recovery"
	reasonDestroyGone   = "removed from configuration; `atmos stack destroy` only destroys components that are still defined"
)

func deletedReason(reverse bool) string {
	if reverse {
		return reasonDestroyGone
	}
	return reasonDeployDeleted
}

// isLocked reports whether the component declares metadata.locked.
func isLocked(n *dependency.Node) bool {
	metadata, _ := n.Metadata["metadata"].(map[string]any)
	locked, _ := metadata["locked"].(bool)
	return locked
}

// checkSelection validates the selected set against the full graph. Deploy
// requires every selected component's required prerequisites to be operable;
// destroy requires that no surviving component still needs a destroyed one.
// Required-prerequisite checks do not apply to destroy, which removes
// consumers before the components they use.
func checkSelection(g *dependency.Graph, commands map[string]*Handler, selected map[string]bool, reverse bool, p *Plan) error {
	if reverse {
		return checkSurvivingConsumers(g, commands, selected, p)
	}
	return checkRequiredPrerequisites(g, commands, selected)
}

// checkRequiredPrerequisites fails when a selected component requires a
// component whose kind cannot run the operation. A required target of a
// supported kind that is simply not selected is assumed to be deployed already.
func checkRequiredPrerequisites(g *dependency.Graph, commands map[string]*Handler, selected map[string]bool) error {
	for _, id := range keys(selected) {
		n := g.Nodes[id]
		for _, target := range n.Dependencies {
			t := g.Nodes[target]
			if commands[t.Type] == nil && !n.OptionalDependencies[target] {
				return fmt.Errorf("%w: %s requires operation on %s", errUtils.ErrUnsupportedComponentType, Label(n), Label(t))
			}
		}
	}
	return nil
}

// checkSurvivingConsumers fails when a component outside the destroy set
// requires a component inside it. Optional dependents do not block, and
// consumers of kinds without a destroy operation are reported as notices.
func checkSurvivingConsumers(g *dependency.Graph, commands map[string]*Handler, destroyed map[string]bool, p *Plan) error {
	var pairs []string
	for _, id := range keys(g.Nodes) {
		consumer := g.Nodes[id]
		if destroyed[id] {
			continue
		}
		for _, target := range consumer.Dependencies {
			if !destroyed[target] || consumer.OptionalDependencies[target] {
				continue
			}
			t := g.Nodes[target]
			if commands[consumer.Type] == nil {
				p.exclude(consumer, "not destroyed (component kind does not define this operation) but requires "+Label(t))
				continue
			}
			pairs = append(pairs, Label(consumer)+" -> "+Label(t))
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	sort.Strings(pairs)
	err := fmt.Errorf("%w: %s", errUtils.ErrSurvivingConsumers, strings.Join(pairs, "; "))
	return errUtils.Build(err).
		WithHint("Add `--include-dependents` to destroy the consumers first, or widen `--stack` to cover them").
		Err()
}

// sortNotices orders notices deterministically and drops duplicates.
func (p *Plan) sortNotices() {
	sort.Slice(p.Notices, func(i, j int) bool {
		a, b := p.Notices[i], p.Notices[j]
		return NodeID(a.Kind, a.Stack, a.Component)+a.Reason < NodeID(b.Kind, b.Stack, b.Component)+b.Reason
	})
	p.Notices = slices.Compact(p.Notices)
}

// readableCycle rewrites a dependency-cycle error in terms of component labels.
// Other errors, and cycles it cannot locate, are returned unchanged.
func readableCycle(g *dependency.Graph, err error) error {
	if !errors.Is(err, dependency.ErrCircularDependency) {
		return err
	}
	cycle := findCycle(g)
	if len(cycle) == 0 {
		return err
	}
	labels := make([]string, len(cycle))
	for i, id := range cycle {
		labels[i] = Label(g.Nodes[id])
	}
	return fmt.Errorf("%w: %s (each component depends on the next)", dependency.ErrCircularDependency, strings.Join(labels, " -> "))
}

// findCycle returns one dependency cycle as node IDs, closed on its first
// element, using a deterministic depth-first search.
func findCycle(g *dependency.Graph) []string {
	const (
		unvisited = iota
		active
		done
	)
	state := make(map[string]int, len(g.Nodes))
	var path []string
	var visit func(id string) []string
	visit = func(id string) []string {
		state[id] = active
		path = append(path, id)
		deps := slices.Clone(g.Nodes[id].Dependencies)
		sort.Strings(deps)
		for _, dep := range deps {
			if g.Nodes[dep] == nil {
				continue
			}
			switch state[dep] {
			case active:
				start := slices.Index(path, dep)
				return append(slices.Clone(path[start:]), dep)
			case unvisited:
				if found := visit(dep); found != nil {
					return found
				}
			}
		}
		path = path[:len(path)-1]
		state[id] = done
		return nil
	}
	for _, id := range keys(g.Nodes) {
		if state[id] == unvisited {
			if found := visit(id); found != nil {
				return found
			}
		}
	}
	return nil
}
