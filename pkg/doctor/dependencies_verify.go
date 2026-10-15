package doctor

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/dependency"
)

// verifyDependencies completes the verification of a dependency migration. The migration adopts the
// modern required semantics on purpose, but it must not change which components anything depends on:
// the runtime prefers `dependencies.components` over `settings.depends_on`, so moving a declaration
// can silently replace a legacy declaration that a component inherits or declares itself.
func (e *Engine) verifyDependencies(ctx context.Context, o *Options, r, candidate *Report, candidates []candidate) error {
	if err := e.verifyDependencyTargets(ctx, o, candidates); err != nil {
		return err
	}
	return recordDependencyVerification(r, candidate, o.DryRun)
}

// verifyDependencyTargets compares the dependency targets the runtime selects for every component
// before and after the candidate content replaces the sources.
func (e *Engine) verifyDependencyTargets(ctx context.Context, o *Options, candidates []candidate) error {
	reporter, ok := e.Stacks.(DependencyTargetReporter)
	if !ok {
		return fmt.Errorf("%w: the stack validator cannot report dependency targets", ErrVerification)
	}
	options := *o.rediscoveryOptions()
	options.Fix = false
	options.DryRun = false
	before, err := e.dependencyTargets(ctx, &options, reporter)
	if err != nil {
		return err
	}
	overlay := sourceOverlay{FileSystem: e.FS, files: map[string][]byte{}}
	for _, c := range candidates {
		overlay.files[c.doc.path] = c.content
	}
	candidateEngine := *e
	candidateEngine.FS = overlay
	candidateEngine.Observe = nil
	after, err := candidateEngine.dependencyTargets(ctx, &options, reporter)
	if err != nil {
		return err
	}
	return compareDependencyTargets(before, after)
}

// dependencyTargets rediscovers the project and asks the validator for the dependency targets of its stacks.
func (e *Engine) dependencyTargets(ctx context.Context, o *Options, reporter DependencyTargetReporter) (map[dependency.ComponentRef][]dependency.ComponentRef, error) {
	p, err := e.discover(ctx, o, &Report{})
	if err != nil {
		return nil, err
	}
	if !e.canCheckStacks(p) {
		return nil, fmt.Errorf("%w: dependency targets cannot be compared because %s", ErrVerification, e.stackBlocker(p))
	}
	config, err := staticConfig(p)
	if err != nil {
		return nil, fmt.Errorf("%w: dependency targets cannot be compared: %w", ErrVerification, err)
	}
	inputs, err := e.staticInputs(p, config)
	if err != nil {
		return nil, err
	}
	targets, err := reporter.DependencyTargets(ctx, config, inputs)
	if targets == nil && err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: dependency targets cannot be compared: %w", ErrVerification, err)
	}
	// Declarations that cannot be parsed are omitted from the result and reported by the dependency check.
	return targets, nil
}

// compareDependencyTargets fails when a component present on both sides depends on different targets.
func compareDependencyTargets(before, after map[dependency.ComponentRef][]dependency.ComponentRef) error {
	var changes []string
	for _, owner := range sortedComponentRefs(before) {
		targetsAfter, present := after[owner]
		if !present {
			continue
		}
		if !slices.Equal(before[owner], targetsAfter) {
			changes = append(changes, fmt.Sprintf("changes the effective dependencies of %s in %s: %s → %s", owner.Component, owner.Stack, describeTargets(before[owner], owner.Stack), describeTargets(targetsAfter, owner.Stack)))
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrVerification, strings.Join(changes, "; "))
}

func sortedComponentRefs(targets map[dependency.ComponentRef][]dependency.ComponentRef) []dependency.ComponentRef {
	owners := make([]dependency.ComponentRef, 0, len(targets))
	for owner := range targets {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(i, j int) bool {
		if owners[i].Stack != owners[j].Stack {
			return owners[i].Stack < owners[j].Stack
		}
		return owners[i].Component < owners[j].Component
	})
	return owners
}

// describeTargets lists targets by component name, adding the stack only where it differs from the owner's.
func describeTargets(targets []dependency.ComponentRef, ownerStack string) string {
	if len(targets) == 0 {
		return "none"
	}
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		name := target.Component
		if target.Stack != ownerStack {
			name += " (" + target.Stack + ")"
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
