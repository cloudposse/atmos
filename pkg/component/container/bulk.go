package container

import (
	"context"
	"errors"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/ui"
)

// singleExecutor runs one lifecycle verb against a single resolved component.
type singleExecutor func(ctx context.Context, info *schema.ConfigAndStacksInfo) error

// bulkExecutors maps each bulk-capable verb to its single-component executor.
// It is a package var so tests can stub the per-item action, and its key set
// defines which verbs support bulk operation.
var bulkExecutors = map[string]singleExecutor{
	"build":   ExecuteBuild,
	"push":    ExecutePush,
	"pull":    ExecutePull,
	"up":      ExecuteUp,
	"start":   ExecuteStart,
	"restart": ExecuteRestart,
	"stop":    ExecuteStop,
	"rm":      ExecuteRm,
	"down":    ExecuteDown,
}

// reverseOrderVerbs operate on targets in reverse sorted order so dependents are
// torn down before their dependencies. Container components have no dependency
// graph yet, so reverse-sorted order is the stable, documented contract.
var reverseOrderVerbs = map[string]bool{
	"down": true,
	"stop": true,
	"rm":   true,
}

// Seams for testing the interactive picker without a TTY.
var (
	promptForValue          = flags.PromptForValue
	promptForMultipleValues = flags.PromptForMultipleValues
)

// isBulkVerb reports whether a verb supports bulk (multi-component) operation.
func isBulkVerb(verb string) bool {
	_, ok := bulkExecutors[verb]
	return ok
}

// shouldRunBulk reports whether a bulk-capable verb invocation should fan out to
// multiple components: `--all`, `--tags`, or `--labels` was passed, or no
// component was given.
func shouldRunBulk(verb string, info *schema.ConfigAndStacksInfo) bool {
	return isBulkVerb(verb) && (info.All || len(info.Tags) > 0 || len(info.Labels) > 0 || info.ComponentFromArg == "")
}

// ExecuteBulk resolves the set of target container components and runs the verb
// against each one, continuing on error and aggregating failures into a single
// summary. Targets come from `--all` (all components, optionally scoped by
// --stack) or from an interactive picker (stack, then components) when no
// component was given.
func ExecuteBulk(ctx context.Context, info *schema.ConfigAndStacksInfo, verb string) error {
	defer perf.Track(nil, "container.ExecuteBulk")()

	exec, ok := bulkExecutors[verb]
	if !ok {
		return fmt.Errorf("%w: %q does not support bulk operation", errUtils.ErrComponentExecutionFailed, verb)
	}

	// A component argument and --all/--tags/--labels are mutually exclusive.
	if (info.All || len(info.Tags) > 0 || len(info.Labels) > 0) && info.ComponentFromArg != "" {
		return errUtils.Build(errUtils.ErrContainerComponentWithAll).
			WithCausef("component %q given with --all/--tags/--labels", info.ComponentFromArg).
			WithHint("Drop the component argument to operate on a selected set of components, or drop --all/--tags/--labels to operate on just that component.").
			Err()
	}

	targets, err := resolveBulkTargets(info, verb)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		ui.Info("No container components selected")
		return nil
	}

	return runBulk(ctx, info, verb, exec, targets)
}

// resolveBulkTargets discovers the container components to operate on, ordered
// for the verb. With `--all` it returns every (non-abstract) component already
// scoped by --stack; otherwise it prompts interactively for a stack and a set of
// components.
func resolveBulkTargets(info *schema.ConfigAndStacksInfo, verb string) ([]instanceRow, error) {
	info.ComponentType = cfg.ContainerComponentType
	atmosConfig, err := initCliConfig(*info, true)
	if err != nil {
		return nil, emptyListOrError(err)
	}

	stacksMap, err := describeStacks(
		&atmosConfig, info.Stack, nil,
		[]string{cfg.ContainerComponentType}, nil,
		false, false, false, false, nil, nil,
	)
	if err != nil {
		return nil, emptyListOrError(err)
	}

	rows := collectContainerInstances(stacksMap) // sorted, abstract excluded.

	if info.All || len(info.Tags) > 0 || len(info.Labels) > 0 {
		return orderTargets(filterByTagsAndLabels(rows, info), verb), nil
	}
	return selectTargetsInteractively(rows, info, verb)
}

// filterByTagsAndLabels narrows rows to those matching info.Tags (any-match)
// and info.Labels (all-match). A no-op when neither is set.
func filterByTagsAndLabels(rows []instanceRow, info *schema.ConfigAndStacksInfo) []instanceRow {
	if len(info.Tags) == 0 && len(info.Labels) == 0 {
		return rows
	}

	var out []instanceRow
	for _, r := range rows {
		if len(info.Tags) > 0 && !tags.MatchesTags(r.tags, info.Tags, tags.TagModeAny) {
			continue
		}
		if len(info.Labels) > 0 && !tags.MatchesLabels(r.labels, info.Labels) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// selectTargetsInteractively prompts for a stack (unless one was given or only
// one is present) and then a set of components within it, all pre-selected.
func selectTargetsInteractively(rows []instanceRow, info *schema.ConfigAndStacksInfo, verb string) ([]instanceRow, error) {
	if len(rows) == 0 {
		return nil, nil
	}

	stack, err := resolveBulkStack(rows, info.Stack)
	if err != nil {
		return nil, err
	}

	inStack := filterByStack(rows, stack)
	if len(inStack) == 0 {
		return nil, fmt.Errorf("%w: no container components in stack %q", errUtils.ErrNoContainerComponentSelected, stack)
	}

	chosen, err := promptForMultipleValues("components", fmt.Sprintf("Choose components to %s in %s", verb, stack), componentNames(inStack))
	if err != nil {
		return nil, noSelectionError(err)
	}
	return orderTargets(filterByComponents(inStack, chosen), verb), nil
}

// resolveBulkStack returns the stack to operate on: the given stack if set, the
// only stack present, or an interactively-picked one when several exist.
func resolveBulkStack(rows []instanceRow, given string) (string, error) {
	if given != "" {
		return given, nil
	}
	stacks := distinctStacks(rows)
	if len(stacks) == 1 {
		return stacks[0], nil
	}
	stack, err := promptForValue("stack", "Choose a stack", stacks)
	if err != nil {
		return "", noSelectionError(err)
	}
	return stack, nil
}

// noSelectionError translates a "not interactive" prompt failure into a hinted,
// actionable error; other prompt errors (e.g. user abort) propagate unchanged.
func noSelectionError(err error) error {
	if errors.Is(err, errUtils.ErrInteractiveModeNotAvailable) {
		return errUtils.Build(errUtils.ErrNoContainerComponentSelected).
			WithHint("Specify a component (e.g. `atmos container up <component> --stack=<stack>`), or pass --all (optionally with --stack) to operate on all container components.").
			Err()
	}
	return err
}

// runBulk runs the verb against each target, continuing on error and joining all
// failures into a single returned error with a per-run summary.
func runBulk(ctx context.Context, info *schema.ConfigAndStacksInfo, verb string, exec singleExecutor, targets []instanceRow) error {
	var errs []error
	for _, t := range targets {
		// Per-target info: a single component in its own stack, never bulk.
		itemInfo := *info
		itemInfo.ComponentFromArg = t.component
		itemInfo.Component = t.component
		itemInfo.Stack = t.stack
		itemInfo.All = false

		if err := exec(ctx, &itemInfo); err != nil {
			ui.Errorf("%s/%s: %s failed: %v", t.stack, t.component, verb, err)
			errs = append(errs, fmt.Errorf("%s/%s: %w", t.stack, t.component, err))
		}
	}

	if len(errs) > 0 {
		ui.Errorf("%s: %d of %d container component(s) failed", verb, len(errs), len(targets))
		return errors.Join(errs...)
	}
	ui.Successf("%s: %d container component(s) succeeded", verb, len(targets))
	return nil
}

// orderTargets returns the targets ordered for the verb: reverse sorted for
// teardown verbs (down/stop/rm), sorted otherwise. Input rows are already sorted
// by (stack, component).
func orderTargets(rows []instanceRow, verb string) []instanceRow {
	if !reverseOrderVerbs[verb] {
		return rows
	}
	reversed := make([]instanceRow, len(rows))
	for i, r := range rows {
		reversed[len(rows)-1-i] = r
	}
	return reversed
}

// distinctStacks returns the sorted, de-duplicated stack names present in rows.
func distinctStacks(rows []instanceRow) []string {
	seen := map[string]struct{}{}
	var stacks []string
	for _, r := range rows {
		if _, ok := seen[r.stack]; ok {
			continue
		}
		seen[r.stack] = struct{}{}
		stacks = append(stacks, r.stack)
	}
	sort.Strings(stacks)
	return stacks
}

// filterByStack returns the rows belonging to the given stack.
func filterByStack(rows []instanceRow, stack string) []instanceRow {
	var out []instanceRow
	for _, r := range rows {
		if r.stack == stack {
			out = append(out, r)
		}
	}
	return out
}

// componentNames returns the component names of the given rows.
func componentNames(rows []instanceRow) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.component
	}
	return names
}

// filterByComponents returns the rows whose component is in the selected set.
func filterByComponents(rows []instanceRow, selected []string) []instanceRow {
	want := make(map[string]struct{}, len(selected))
	for _, s := range selected {
		want[s] = struct{}{}
	}
	var out []instanceRow
	for _, r := range rows {
		if _, ok := want[r.component]; ok {
			out = append(out, r)
		}
	}
	return out
}
