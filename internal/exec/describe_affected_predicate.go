package exec

import (
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	listfilter "github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// affectedPredicate decides whether an affected component or dependent is kept by `--filter`. The
// argument is the item's JSON-shaped map, the same shape `--query` evaluates against, so users write
// the same paths (for example `.settings.github.actions_enabled`) in both.
type affectedPredicate func(item map[string]any) (bool, error)

// newAffectedPredicate builds the `--filter` predicate from a YQ expression, using the same truthiness
// rules as `list instances --filter`. An empty expression returns a nil predicate, which callers treat as
// "keep everything".
//
// The expression is probed once against an empty item so a syntax error is reported even when the
// affected list happens to be empty, instead of being silently skipped.
func newAffectedPredicate(atmosConfig *schema.AtmosConfiguration, expr string) (affectedPredicate, error) {
	if expr == "" {
		return nil, nil
	}

	f, err := listfilter.NewYQPredicateFilter(expr, atmosConfig)
	if err != nil {
		return nil, err
	}
	if _, err := f.Matches(map[string]any{}); err != nil {
		return nil, err
	}
	return f.Matches, nil
}

// pruneAffectedByPredicate applies the `--filter` predicate to the affected forest. The predicate is
// evaluated on every top-level item and on every dependent at every depth. An item that fails is dropped
// together with its whole dependents subtree, even when a dependent would pass on its own: unlike the
// `--tags` / `--labels` selectors, nothing is promoted into the top-level list.
//
// A nil predicate returns the input unchanged. Otherwise the result is a new slice whose items and nested
// dependent lists are copies, so mutating the result's items or dependents never touches the input (and
// vice versa). The `settings` and other map sections of the items are shared with the input and must be
// treated as read-only. `included_in_dependents` is recomputed because a dropped parent no longer
// includes its dependents.
func pruneAffectedByPredicate(affected []schema.Affected, pred affectedPredicate) ([]schema.Affected, error) {
	if pred == nil {
		return affected, nil
	}

	out := make([]schema.Affected, 0, len(affected))
	hasDependents := false
	for i := range affected {
		keep, err := affectedMatches(pred, &affected[i])
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}

		item := affected[i]
		item.Dependents, err = pruneDependentsByPredicate(affected[i].Dependents, pred)
		if err != nil {
			return nil, err
		}
		hasDependents = hasDependents || len(item.Dependents) > 0
		out = append(out, item)
	}

	if hasDependents {
		for i := range out {
			out[i].IncludedInDependents = processIncludedInDependenciesForAffected(&out, out[i].StackSlug, i)
		}
	}
	return out, nil
}

// pruneDependentsByPredicate returns a copy of the dependents tree without the dependents that fail the
// predicate and without anything beneath them. A nil input stays nil.
func pruneDependentsByPredicate(dependents []schema.Dependent, pred affectedPredicate) ([]schema.Dependent, error) {
	if dependents == nil {
		return nil, nil
	}

	out := make([]schema.Dependent, 0, len(dependents))
	for i := range dependents {
		keep, err := affectedMatches(pred, &dependents[i])
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}

		d := dependents[i]
		d.Dependents, err = pruneDependentsByPredicate(dependents[i].Dependents, pred)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// affectedMatches evaluates the predicate against the JSON-shaped map of an affected component or dependent.
func affectedMatches(pred affectedPredicate, item any) (bool, error) {
	m, err := affectedItemToMap(item)
	if err != nil {
		return false, err
	}
	return pred(m)
}

// affectedItemToMap converts an affected component or dependent to the map `--query` would see, by way of
// its JSON encoding, so field names follow the json tags users already see in the command output.
func affectedItemToMap(item any) (map[string]any, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to encode an affected item for --filter: %w", errUtils.ErrInvalidConfig, err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: failed to decode an affected item for --filter: %w", errUtils.ErrInvalidConfig, err)
	}
	return m, nil
}

// resolveAffectedFilter rejects `--filter` combined with `--upload`. The Atmos Pro upload is always the
// unfiltered affected set, so a filter would shape the printed output while silently uploading
// everything. This mirrors resolveAffectedSelectors for `--tags` / `--labels`.
//
// A filter that came from ATMOS_DESCRIBE_AFFECTED_FILTER is ambient rather than a request for this run,
// so with `--upload` it is dropped with a warning; only a `--filter` typed on the command line is an error.
func resolveAffectedFilter(a *DescribeAffectedCmdArgs) error {
	if a.Filter == "" || !a.Upload {
		return nil
	}

	if a.FilterEnvVar != "" {
		ui.Warningf("Ignoring %s: --upload always uploads the unfiltered affected set", a.FilterEnvVar)
		a.Filter = ""
		a.FilterEnvVar = ""
		return nil
	}

	return errUtils.Build(fmt.Errorf("%w: --filter is not supported with --upload (the upload is always unfiltered)", errUtils.ErrInvalidFlag)).
		WithHint("Run `atmos describe affected --upload` without --filter, and apply --filter in a separate `describe affected` step for the matrix.").
		WithHint("To override a filter supplied through ATMOS_DESCRIBE_AFFECTED_FILTER for this run, pass --filter=.").
		Err()
}

// rejectQueryWithMatrix fails when `--query` is combined with `--format=matrix`. A query can reshape its
// input into anything, so a matrix cannot be built from its result; `--filter` selects the same
// components without changing their shape and works with every format.
func rejectQueryWithMatrix(a *DescribeAffectedCmdArgs) error {
	if a.Format != "matrix" || a.Query == "" {
		return nil
	}

	return errUtils.Build(fmt.Errorf("%w: --query is not supported with --format=matrix, use --filter instead", errUtils.ErrInvalidFlag)).
		WithHint("Use --filter to select which affected components appear in the matrix.").
		WithHint("Example: --include-settings --filter '.settings.github.actions_enabled // false' --format=matrix").
		Err()
}
