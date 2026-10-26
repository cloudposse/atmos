package adapters

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
)

func TestValidateTerraformGraphMatchesExecution(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, targetPresent := range []bool{false, true} {
			component := map[string]any{}
			entries := []any{map[string]any{"component": "database"}}
			if modern {
				component["dependencies"] = map[string]any{"components": entries}
			} else {
				component["settings"] = map[string]any{"depends_on": entries}
			}
			components := map[string]any{"app": component}
			if targetPresent {
				components["database"] = map[string]any{}
			}
			stacks := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": components}}}
			_, executionErr := BuildTerraformGraph(stacks)
			validationErr := ValidateTerraformGraph(stacks, "")
			if executionErr == nil {
				require.NoError(t, validationErr)
			} else {
				require.EqualError(t, validationErr, executionErr.Error())
			}
			warnings := 0
			_, err := buildTerraformGraphWithWarnings(stacks, "", nil, nil, func(any, ...any) { warnings++ })
			if modern && !targetPresent {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if !modern && !targetPresent {
				require.Equal(t, 1, warnings)
			} else {
				require.Zero(t, warnings)
			}
		}
	}
}

func terraformStacksFixture(components map[string]any) map[string]any {
	return map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": components}}}
}

func modernDependencies(targets ...string) map[string]any {
	entries := make([]any, 0, len(targets))
	for _, target := range targets {
		entries = append(entries, map[string]any{"component": target})
	}
	return map[string]any{"dependencies": map[string]any{"components": entries}}
}

func TestValidateTerraformGraphReportsEveryMissingTarget(t *testing.T) {
	stacks := terraformStacksFixture(map[string]any{
		"app":      modernDependencies("missing-a", "missing-b", "missing-c"),
		"disabled": map[string]any{"metadata": map[string]any{"enabled": false}},
		"worker":   modernDependencies("disabled"),
	})

	err := ValidateTerraformGraph(stacks, "")

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrDependencyTargetNotFound)
	require.ErrorIs(t, err, errUtils.ErrDependencyTargetUnavailable)
	var found []dependency.DependencyTargetError
	for _, leaf := range flattenTestErrors(err) {
		var target *dependency.DependencyTargetError
		if errors.As(leaf, &target) {
			found = append(found, *target)
		}
	}
	require.Equal(t, []dependency.DependencyTargetError{
		{Component: "app", Stack: "dev", TargetComponent: "missing-a", TargetStack: "dev", Reason: dependency.TargetMissing},
		{Component: "app", Stack: "dev", TargetComponent: "missing-b", TargetStack: "dev", Reason: dependency.TargetMissing},
		{Component: "app", Stack: "dev", TargetComponent: "missing-c", TargetStack: "dev", Reason: dependency.TargetMissing},
		{Component: "worker", Stack: "dev", TargetComponent: "disabled", TargetStack: "dev", Reason: dependency.TargetDisabled},
	}, found)
}

// Execution stops at the first problem; only validation collects them all.
func TestBuildTerraformGraphStaysFailFast(t *testing.T) {
	stacks := terraformStacksFixture(map[string]any{"app": modernDependencies("missing-a", "missing-b")})

	_, err := BuildTerraformGraph(stacks)

	require.ErrorIs(t, err, errUtils.ErrDependencyTargetNotFound)
	require.Contains(t, err.Error(), "missing-a")
	require.NotContains(t, err.Error(), "missing-b")
}

func TestValidateTerraformGraphStillDetectsCyclesAlongsideMissingTargets(t *testing.T) {
	stacks := terraformStacksFixture(map[string]any{
		"a":    modernDependencies("b"),
		"b":    modernDependencies("a"),
		"solo": modernDependencies("ghost"),
	})

	err := ValidateTerraformGraph(stacks, "")

	require.ErrorIs(t, err, errUtils.ErrDependencyTargetNotFound)
	require.ErrorIs(t, err, dependency.ErrCircularDependency)
	var declaration *dependency.DependencyDeclarationError
	require.ErrorAs(t, err, &declaration)
	require.Contains(t, []string{"a", "b", "solo"}, declaration.Component)
}

func TestValidateTerraformGraphAttributesParseFailuresToTheComponent(t *testing.T) {
	stacks := terraformStacksFixture(map[string]any{"app": map[string]any{"dependencies": "not-a-map"}})

	err := ValidateTerraformGraph(stacks, "")

	var declaration *dependency.DependencyDeclarationError
	require.ErrorAs(t, err, &declaration)
	require.Equal(t, "app", declaration.Component)
	require.Equal(t, "dev", declaration.Stack)
	require.ErrorIs(t, err, errUtils.ErrInvalidDependenciesSection)
}

func flattenTestErrors(err error) []error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var leaves []error
		for _, child := range joined.Unwrap() {
			leaves = append(leaves, flattenTestErrors(child)...)
		}
		return leaves
	}
	if wrapped := errors.Unwrap(err); wrapped != nil {
		if _, isTarget := err.(*dependency.DependencyTargetError); !isTarget { //nolint:errorlint // The located error itself is wanted, not one it wraps.
			return flattenTestErrors(wrapped)
		}
	}
	return []error{err}
}

func TestTerraformDependencyTargetsFollowRuntimeSelection(t *testing.T) {
	stacks := terraformStacksFixture(map[string]any{
		"base": map[string]any{
			"metadata": map[string]any{"type": "abstract"},
			"settings": map[string]any{"depends_on": []any{"ignored"}},
		},
		"vpc": map[string]any{},
		"db":  map[string]any{},
		// The modern section wins over the legacy one, exactly as the scheduler selects.
		"app": map[string]any{
			"dependencies": map[string]any{"components": []any{map[string]any{"component": "vpc"}, map[string]any{"component": "db", "stack": "prod"}}},
			"settings":     map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "db"}}},
		},
		"worker":   map[string]any{"settings": map[string]any{"depends_on": []any{"db", map[string]any{"component": "vpc", "stack": "prod"}}}},
		"disabled": map[string]any{"metadata": map[string]any{"enabled": false}},
	})

	targets, err := TerraformDependencyTargets(stacks, "")

	require.NoError(t, err)
	require.Equal(t, map[dependency.ComponentRef][]dependency.ComponentRef{
		{Component: "app", Stack: "dev"}:    {{Component: "vpc", Stack: "dev"}, {Component: "db", Stack: "prod"}},
		{Component: "db", Stack: "dev"}:     nil,
		{Component: "vpc", Stack: "dev"}:    nil,
		{Component: "worker", Stack: "dev"}: {{Component: "db", Stack: "dev"}, {Component: "vpc", Stack: "prod"}},
	}, targets)
}

func TestTerraformDependencyTargetsReportsMalformedDeclarations(t *testing.T) {
	stacks := terraformStacksFixture(map[string]any{"app": map[string]any{"dependencies": "nope"}, "vpc": map[string]any{}})

	targets, err := TerraformDependencyTargets(stacks, "")

	var declaration *dependency.DependencyDeclarationError
	require.ErrorAs(t, err, &declaration)
	require.Equal(t, "app", declaration.Component)
	require.Contains(t, targets, dependency.ComponentRef{Component: "vpc", Stack: "dev"})
}
