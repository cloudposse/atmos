package exec

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// typedDependent is dependentFixture for a Terraform dependent, which is what the affected forest keys on.
func typedDependent(component, ci string, children ...schema.Dependent) schema.Dependent {
	d := dependentFixture(component, ci, children...)
	d.ComponentType = "terraform"
	return d
}

// topLevelAffected builds a live top-level affected entry for the "dev" stack.
func topLevelAffected(component string, dependents ...schema.Dependent) schema.Affected {
	return schema.Affected{
		Component:     component,
		ComponentType: "terraform",
		Stack:         "dev",
		StackSlug:     "dev-" + component,
		Affected:      affectedReasonStackVars,
		AffectedAll:   []string{affectedReasonStackVars},
		Dependents:    dependents,
	}
}

// forestStacks builds the resolved-stacks map with a Terraform metadata section per component of "dev".
func forestStacks(ciByComponent map[string]string) map[string]any {
	terraform := map[string]any{}
	for component, ci := range ciByComponent {
		terraform[component] = map[string]any{"metadata": map[string]any{"labels": map[string]any{"ci": ci}}}
	}
	return map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": terraform}}}
}

func affectedComponents(affected []schema.Affected) []string {
	out := make([]string, 0, len(affected))
	for i := range affected {
		out = append(out, affected[i].Component)
	}
	return out
}

// Compile-time sentinels so a rename of the fields these tests rely on fails the build here.
var (
	_ = schema.Affected{Affected: "", AffectedAll: nil, Dependents: nil, IncludedInDependents: false, Deleted: false}
	_ = schema.Dependent{Metadata: nil, Dependents: nil, IncludedInDependents: false}
	_ = AffectedFilter{DeferSelectors: true}
)

var autoOnly = AffectedFilter{Labels: map[string]string{"ci": "auto"}}

func TestApplySelectorsToAffectedForest(t *testing.T) {
	t.Run("a dropped parent is replaced by its matching dependent at the parent's position", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("vpc"),
			topLevelAffected("iam", typedDependent("app", "auto"), typedDependent("batch", "manual")),
			topLevelAffected("web"),
		}
		stacks := forestStacks(map[string]string{"vpc": "auto", "iam": "manual", "web": "auto"})

		got := applySelectorsToAffectedForest(affected, autoOnly, stacks)

		assert.Equal(t, []string{"vpc", "app", "web"}, affectedComponents(got))
		promoted := got[1]
		assert.Equal(t, affectedReasonDependent, promoted.Affected)
		assert.Equal(t, []string{affectedReasonDependent}, promoted.AffectedAll)
		assert.Equal(t, "dev-app", promoted.StackSlug)
		assert.Equal(t, "terraform", promoted.ComponentType)
		assert.Equal(t, affectedReasonStackVars, got[0].Affected, "kept items keep their own reason")
	})

	t.Run("a dependent that is already top-level is not duplicated and keeps its reason", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("iam", typedDependent("app", "auto")),
			topLevelAffected("app"),
		}
		stacks := forestStacks(map[string]string{"iam": "manual", "app": "auto"})

		got := applySelectorsToAffectedForest(affected, autoOnly, stacks)

		require.Len(t, got, 1)
		assert.Equal(t, "app", got[0].Component)
		assert.Equal(t, affectedReasonStackVars, got[0].Affected)
	})

	t.Run("two dropped parents sharing a dependent promote it once", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("iam", typedDependent("app", "auto")),
			topLevelAffected("ops", typedDependent("app", "auto")),
		}
		stacks := forestStacks(map[string]string{"iam": "manual", "ops": "manual"})

		got := applySelectorsToAffectedForest(affected, autoOnly, stacks)

		assert.Equal(t, []string{"app"}, affectedComponents(got))
	})

	t.Run("a kept top-level item keeps a matching dependent nested below a pruned one", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("base", typedDependent("middle", "manual", typedDependent("leaf", "auto"))),
		}
		stacks := forestStacks(map[string]string{"base": "auto"})

		got := applySelectorsToAffectedForest(affected, autoOnly, stacks)

		require.Len(t, got, 1)
		assert.Equal(t, "base", got[0].Component)
		assert.Equal(t, []string{"dev-leaf"}, dependentSlugs(got[0].Dependents))
	})

	t.Run("deleted items are kept without a metadata lookup", func(t *testing.T) {
		deleted := schema.Affected{
			Component: "gone", ComponentType: "terraform", Stack: "dev", StackSlug: "dev-gone",
			Affected: affectedReasonDeleted, AffectedAll: []string{affectedReasonDeleted},
			Deleted: true, DeletionType: deletionTypeComponent,
		}

		got := applySelectorsToAffectedForest([]schema.Affected{deleted}, autoOnly, map[string]any{})

		require.Len(t, got, 1)
		assert.True(t, got[0].Deleted)
		assert.Equal(t, deletionTypeComponent, got[0].DeletionType)
		assert.Equal(t, affectedReasonDeleted, got[0].Affected)
	})

	t.Run("a live item missing from the stacks, or without metadata, is dropped", func(t *testing.T) {
		stacks := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{
			"bare": map[string]any{"vars": map[string]any{}},
		}}}}

		got := applySelectorsToAffectedForest([]schema.Affected{topLevelAffected("ghost"), topLevelAffected("bare")}, autoOnly, stacks)

		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("an ExcludeLocked-only filter is the identity", func(t *testing.T) {
		affected := []schema.Affected{topLevelAffected("iam", typedDependent("app", "manual")), topLevelAffected("vpc")}

		got := applySelectorsToAffectedForest(affected, AffectedFilter{ExcludeLocked: true}, nil)

		assert.Equal(t, []string{"iam", "vpc"}, affectedComponents(got))
		assert.Equal(t, []string{"dev-app"}, dependentSlugs(got[0].Dependents), "dependents are not pruned")
	})

	t.Run("a promoted item has its nested dependents pruned and its metadata cleared", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("iam", typedDependent("app", "auto", typedDependent("x", "manual"), typedDependent("y", "auto"))),
		}
		stacks := forestStacks(map[string]string{"iam": "manual"})

		got := applySelectorsToAffectedForest(affected, autoOnly, stacks)

		require.Len(t, got, 1)
		assert.Equal(t, "app", got[0].Component)
		require.Equal(t, []string{"dev-y"}, dependentSlugs(got[0].Dependents))
		assert.Nil(t, got[0].Dependents[0].Metadata)
	})

	t.Run("metadata is cleared from the dependents of kept items", func(t *testing.T) {
		affected := []schema.Affected{topLevelAffected("vpc", typedDependent("app", "auto"))}

		got := applySelectorsToAffectedForest(affected, autoOnly, forestStacks(map[string]string{"vpc": "auto"}))

		require.Len(t, got, 1)
		require.Len(t, got[0].Dependents, 1)
		assert.Nil(t, got[0].Dependents[0].Metadata)
	})

	t.Run("included_in_dependents is recomputed", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("vpc", typedDependent("web", "auto")),
			topLevelAffected("iam", typedDependent("web", "auto")),
			topLevelAffected("web"),
		}
		stacks := forestStacks(map[string]string{"vpc": "auto", "iam": "manual", "web": "auto"})

		got := applySelectorsToAffectedForest(affected, autoOnly, stacks)

		require.Equal(t, []string{"vpc", "web"}, affectedComponents(got))
		assert.False(t, got[0].IncludedInDependents)
		assert.True(t, got[1].IncludedInDependents, "web is a dependent of vpc")
	})

	t.Run("the result is never nil", func(t *testing.T) {
		assert.NotNil(t, applySelectorsToAffectedForest([]schema.Affected{topLevelAffected("vpc")}, autoOnly, forestStacks(map[string]string{"vpc": "manual"})))
	})
}

func TestTopLevelAffectedMetadata(t *testing.T) {
	stacks := forestStacks(map[string]string{"vpc": "auto"})
	a := topLevelAffected("vpc")

	assert.Equal(t, map[string]any{"labels": map[string]any{"ci": "auto"}}, topLevelAffectedMetadata(&a, stacks))

	missing := topLevelAffected("missing")
	assert.Nil(t, topLevelAffectedMetadata(&missing, stacks))
	assert.Nil(t, topLevelAffectedMetadata(&a, nil))

	helm := topLevelAffected("vpc")
	helm.ComponentType = "helmfile"
	assert.Nil(t, topLevelAffectedMetadata(&helm, stacks), "the component type is part of the lookup")
}

func TestAffectedKey(t *testing.T) {
	assert.Equal(t, affectedKey("vpc", "dev", "terraform"), affectedKey("vpc", "dev", "terraform"))
	assert.NotEqual(t, affectedKey("vpc", "dev", "terraform"), affectedKey("vpc", "dev", "helmfile"))
	assert.NotEqual(t, affectedKey("vpc", "dev", "terraform"), affectedKey("vpc", "prod", "terraform"))
	assert.NotEqual(t, affectedKey("vpc", "dev", "terraform"), affectedKey("app", "dev", "terraform"))
	assert.NotEqual(t, affectedKey("a", "b-c", "t"), affectedKey("a-b", "c", "t"), "fields cannot run together")
}

// TestDependentToAffected_CopiesEverySharedField guards the 1:1 copy: every Dependent field except the
// transient Metadata (and the nested/included flags handled explicitly) must appear on the Affected.
func TestDependentToAffected_CopiesEverySharedField(t *testing.T) {
	d := schema.Dependent{
		Component: "app", ComponentType: "terraform", ComponentPath: "components/terraform/app",
		Namespace: "ns", Tenant: "tn", Environment: "ue1", Stage: "dev",
		Stack: "dev", StackSlug: "dev-app", SpaceliftStack: "dev-app-sl", AtlantisProject: "dev-app-at",
		Dependents:           []schema.Dependent{{Component: "child", Stack: "dev"}},
		IncludedInDependents: true,
		Settings:             schema.AtmosSectionMapType{"k": "v"},
		Metadata:             schema.AtmosSectionMapType{"labels": map[string]any{"ci": "auto"}},
	}

	got := dependentToAffected(&d, affectedReasonDependent)

	assert.Equal(t, affectedReasonDependent, got.Affected)
	assert.Equal(t, []string{affectedReasonDependent}, got.AffectedAll)
	assert.False(t, got.Deleted)

	// Every Dependent field must be accounted for, by name, on the resulting Affected.
	dependentType := reflect.TypeOf(d)
	affectedValue := reflect.ValueOf(got)
	dependentValue := reflect.ValueOf(d)
	for i := range dependentType.NumField() {
		name := dependentType.Field(i).Name
		if name == "Metadata" {
			continue
		}
		field := affectedValue.FieldByName(name)
		require.True(t, field.IsValid(), "schema.Affected has no field %s that schema.Dependent has; copy it in dependentToAffected", name)
		assert.True(t, reflect.DeepEqual(dependentValue.Field(i).Interface(), field.Interface()), "field %s must be copied unchanged", name)
	}
}

func TestFlattenAffectedDependents(t *testing.T) {
	t.Run("dependents already at the top level are not repeated", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("vpc", typedDependent("app", ""), typedDependent("db", "")),
			topLevelAffected("app"),
		}

		got := flattenAffectedDependents(affected)

		assert.Equal(t, []string{"vpc", "db", "app"}, affectedComponents(got))
		assert.Equal(t, affectedReasonDependent, got[1].Affected)
		assert.Equal(t, affectedReasonStackVars, got[2].Affected, "the top-level app keeps its own reason")
	})

	t.Run("a dependent shared by two parents appears once", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("a", typedDependent("x", "")),
			topLevelAffected("b", typedDependent("x", "")),
		}

		got := flattenAffectedDependents(affected)

		assert.Equal(t, []string{"a", "x", "b"}, affectedComponents(got))
	})

	t.Run("nested dependents follow their parent in pre-order", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected(
				"vpc",
				typedDependent("iam", "", typedDependent("app", "")),
				typedDependent("web", ""),
			),
		}

		got := flattenAffectedDependents(affected)

		assert.Equal(t, []string{"vpc", "iam", "app", "web"}, affectedComponents(got))
		for i := range got {
			assert.NotNil(t, got[i].Dependents)
			assert.Empty(t, got[i].Dependents)
			assert.False(t, got[i].IncludedInDependents)
		}
	})

	t.Run("lifted entries carry the dependent's own fields", func(t *testing.T) {
		dep := typedDependent("app", "")
		dep.ComponentPath = "components/terraform/app"
		dep.Settings = schema.AtmosSectionMapType{"k": "v"}

		got := flattenAffectedDependents([]schema.Affected{topLevelAffected("vpc", dep)})

		require.Len(t, got, 2)
		assert.Equal(t, "components/terraform/app", got[1].ComponentPath)
		assert.Equal(t, schema.AtmosSectionMapType{"k": "v"}, got[1].Settings)
		assert.Equal(t, []string{affectedReasonDependent}, got[1].AffectedAll)
	})

	t.Run("deleted items pass through untouched", func(t *testing.T) {
		deleted := schema.Affected{
			Component: "gone", ComponentType: "terraform", Stack: "dev", StackSlug: "dev-gone",
			Affected: affectedReasonDeleted, AffectedAll: []string{affectedReasonDeleted},
			Deleted: true, DeletionType: deletionTypeComponent, Dependents: []schema.Dependent{},
		}

		got := flattenAffectedDependents([]schema.Affected{deleted})

		require.Len(t, got, 1)
		assert.Equal(t, deleted, got[0])
	})

	t.Run("flattening twice gives the same result", func(t *testing.T) {
		affected := []schema.Affected{
			topLevelAffected("vpc", typedDependent("iam", "", typedDependent("app", ""))),
			topLevelAffected("web"),
		}

		once := flattenAffectedDependents(affected)
		twice := flattenAffectedDependents(once)

		assert.Equal(t, once, twice)
	})

	t.Run("an item without dependents gets an empty non-nil list", func(t *testing.T) {
		got := flattenAffectedDependents([]schema.Affected{topLevelAffected("vpc")})

		require.Len(t, got, 1)
		assert.NotNil(t, got[0].Dependents)
		assert.Empty(t, got[0].Dependents)
	})

	t.Run("nil and empty input yield a non-nil empty slice", func(t *testing.T) {
		assert.NotNil(t, flattenAffectedDependents(nil))
		assert.Empty(t, flattenAffectedDependents(nil))
		assert.Empty(t, flattenAffectedDependents([]schema.Affected{}))
	})

	t.Run("the input is not modified", func(t *testing.T) {
		affected := []schema.Affected{topLevelAffected("vpc", typedDependent("app", ""))}

		got := flattenAffectedDependents(affected)
		got[0].Component = "mutated"

		assert.Equal(t, "vpc", affected[0].Component)
		assert.Equal(t, []string{"dev-app"}, dependentSlugs(affected[0].Dependents))
	})

	t.Run("a dependency cycle terminates", func(t *testing.T) {
		// a -> b -> a: b's child a is already seen (top level), so the walk stops there.
		a := typedDependent("a", "")
		b := typedDependent("b", "", a)
		got := flattenAffectedDependents([]schema.Affected{topLevelAffected("a", b)})

		assert.Equal(t, []string{"a", "b"}, affectedComponents(got))
	})
}

func TestValidateDescribeAffectedArgs(t *testing.T) {
	valid := func() DescribeAffectedCmdArgs {
		return DescribeAffectedCmdArgs{Format: "json", ErrorMode: "warn"}
	}

	tests := []struct {
		name        string
		mutate      func(a *DescribeAffectedCmdArgs)
		wantErr     error
		wantHints   int
		wantFlatten bool
	}{
		{name: "defaults are valid", mutate: func(*DescribeAffectedCmdArgs) {}},
		{name: "invalid format", mutate: func(a *DescribeAffectedCmdArgs) { a.Format = "xml" }, wantErr: ErrInvalidFormat},
		{name: "repo path conflicts with ref", mutate: func(a *DescribeAffectedCmdArgs) { a.RepoPath, a.Ref = "/tmp/repo", "main" }, wantErr: ErrRepoPathConflict},
		{name: "invalid error mode", mutate: func(a *DescribeAffectedCmdArgs) { a.ErrorMode = "loud" }, wantErr: ErrInvalidErrorMode},
		{name: "malformed labels", mutate: func(a *DescribeAffectedCmdArgs) { a.LabelsRaw = "nope" }, wantErr: errUtils.ErrInvalidFlag},
		{
			name:      "flatten without include-dependents",
			mutate:    func(a *DescribeAffectedCmdArgs) { a.Flatten = true },
			wantErr:   errUtils.ErrInvalidFlag,
			wantHints: 1,
		},
		{
			name:        "flatten with include-dependents",
			mutate:      func(a *DescribeAffectedCmdArgs) { a.Flatten, a.IncludeDependents = true, true },
			wantFlatten: true,
		},
		{
			name: "flatten with upload is rejected even though upload implies include-dependents",
			mutate: func(a *DescribeAffectedCmdArgs) {
				a.Flatten, a.Upload, a.IncludeDependents = true, true, true
			},
			wantErr:   errUtils.ErrInvalidFlag,
			wantHints: 1,
		},
		{
			name: "env-sourced flatten is dropped with upload",
			mutate: func(a *DescribeAffectedCmdArgs) {
				a.Flatten, a.FlattenEnvVar, a.Upload, a.IncludeDependents = true, "ATMOS_DESCRIBE_AFFECTED_FLATTEN", true, true
			},
		},
		{
			name: "env-sourced flatten is dropped without include-dependents",
			mutate: func(a *DescribeAffectedCmdArgs) {
				a.Flatten, a.FlattenEnvVar = true, "ATMOS_DESCRIBE_AFFECTED_FLATTEN"
			},
		},
		{
			name: "env-sourced flatten is kept with include-dependents",
			mutate: func(a *DescribeAffectedCmdArgs) {
				a.Flatten, a.FlattenEnvVar, a.IncludeDependents = true, "ATMOS_DESCRIBE_AFFECTED_FLATTEN", true
			},
			wantFlatten: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := valid()
			tt.mutate(&args)

			err := validateDescribeAffectedArgs(&args)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if tt.wantHints > 0 {
					assert.Len(t, cockroachErrors.GetAllHints(err), tt.wantHints)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFlatten, args.Flatten)
		})
	}
}

func TestSetDescribeAffectedFlagValueInCliArgs_Flatten(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "")

	flags := newDescribeAffectedFlagSet()
	require.NoError(t, flags.Set("flatten", "true"))
	require.NoError(t, flags.Set("include-dependents", "true"))
	args := DescribeAffectedCmdArgs{CLIConfig: &schema.AtmosConfiguration{}}

	SetDescribeAffectedFlagValueInCliArgs(flags, &args)

	assert.True(t, args.Flatten)
	assert.Empty(t, args.FlattenEnvVar)
}

// TestFindAffected_DeferSelectors_DeletedStillFiltered verifies that deferring the selectors leaves live
// components unfiltered but still matches deleted components against their BASE metadata.
func TestFindAffected_DeferSelectors_DeletedStillFiltered(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}
	remote, current := deletedSelectorFixture()
	// Keep HEAD's "dev" stack empty so that "kept-manual" and every other BASE component count as deleted.
	current["dev"] = map[string]any{"components": map[string]any{"terraform": map[string]any{
		"live-manual": map[string]any{"metadata": map[string]any{"component": "live", "labels": map[string]any{"ci": "manual"}}},
	}}}

	filter := AffectedFilter{Labels: map[string]string{"ci": "auto"}, DeferSelectors: true}
	affected, err := findAffected(&current, &remote, atmosConfig, nil, false, false, "", filter, "")
	require.NoError(t, err)

	var live, deleted []string
	for i := range affected {
		if affected[i].Deleted {
			deleted = append(deleted, affected[i].Component)
		} else {
			live = append(live, affected[i].Component)
		}
	}
	assert.Equal(t, []string{"live-manual"}, live, "a live component is not filtered while the selectors are deferred")
	assert.ElementsMatch(t, []string{"vpc-auto", "old-auto", "empty-auto"}, deleted, "deleted components are still matched on their BASE metadata")
}

// matrixDependentsExec builds an executor for the matrix tests: the checkout strategy computes affected
// components from the in-memory stacks (with the filter Execute built), and the dependents stubs attach
// `app-auto` (an auto dependent) to every live affected component the way the real resolution would.
func matrixDependentsExec(atmosConfig *schema.AtmosConfiguration, current, remote map[string]any) *describeAffectedExec {
	d := &describeAffectedExec{atmosConfig: atmosConfig}
	d.IsTTYSupportForStdout = func() bool { return false }
	d.executeDescribeAffectedWithTargetRefCheckout = func(
		_ *schema.AtmosConfiguration, _, _, _ string, _, _ bool, _ string, _, _ bool,
		_ []string, filter AffectedFilter, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		affected, err := findAffected(&current, &remote, atmosConfig, nil, false, false, "", filter, "")
		return affected, nil, nil, "", err
	}
	attach := func(affected *[]schema.Affected) {
		for i := range *affected {
			(*affected)[i].Dependents = []schema.Dependent{{
				Component: "app-auto", ComponentType: "terraform", Stack: "dev", StackSlug: "dev-app-auto",
			}}
		}
	}
	d.addDependentsToAffected = func(_ *schema.AtmosConfiguration, affected *[]schema.Affected, _, _, _ bool, _ []string, _ string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) error {
		attach(affected)
		return nil
	}
	d.addDependentsToAffectedWithFilter = func(_ *schema.AtmosConfiguration, affected *[]schema.Affected, opts *dependentsOptions) error {
		attach(affected)
		opts.stacks = current
		for i := range *affected {
			attachDependentMetadata((*affected)[i].Dependents, current)
		}
		return nil
	}
	return d
}

// TestExecute_MatrixFormat_IncludeDependents covers the matrix output with --include-dependents: a matching
// dependent of an excluded parent reaches the matrix, and --flatten lifts dependents into it.
func TestExecute_MatrixFormat_IncludeDependents(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}
	app := map[string]any{"metadata": map[string]any{"component": "app", "labels": map[string]any{"ci": "auto"}}}
	current := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{
		"app-auto":   app,
		"iam-manual": map[string]any{"metadata": map[string]any{"component": "iam", "labels": map[string]any{"ci": "manual"}}},
	}}}}
	// app-auto is unchanged in BASE, so only iam-manual (new in HEAD) is affected on its own.
	remote := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{"app-auto": app}}}}
	d := matrixDependentsExec(atmosConfig, current, remote)

	run := func(t *testing.T, args DescribeAffectedCmdArgs) string {
		t.Helper()
		outputFile := filepath.Join(t.TempDir(), "github_output")
		args.Format = "matrix"
		args.GithubOutputFile = outputFile
		args.CLIConfig = atmosConfig
		args.IncludeDependents = true
		require.NoError(t, validateDescribeAffectedArgs(&args))
		require.NoError(t, d.Execute(&args))
		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		return string(content)
	}

	t.Run("labels=ci=auto promotes the matching dependent of the excluded manual component", func(t *testing.T) {
		content := run(t, DescribeAffectedCmdArgs{LabelsRaw: "ci=auto", Format: "matrix", ErrorMode: "warn"})
		assert.Contains(t, content, "app-auto")
		assert.NotContains(t, content, "iam-manual")
		assert.Contains(t, content, "count=1")
	})

	t.Run("without a selector only the affected component is in the matrix", func(t *testing.T) {
		content := run(t, DescribeAffectedCmdArgs{Format: "matrix", ErrorMode: "warn"})
		assert.Contains(t, content, "iam-manual")
		assert.NotContains(t, content, "app-auto")
		assert.Contains(t, content, "count=1")
	})

	t.Run("flatten adds the dependents to the matrix", func(t *testing.T) {
		content := run(t, DescribeAffectedCmdArgs{Flatten: true, Format: "matrix", ErrorMode: "warn"})
		assert.Contains(t, content, "iam-manual")
		assert.Contains(t, content, "app-auto")
		assert.Contains(t, content, "count=2")
	})
}

// TestFinalizeAffectedDependents covers the shared finalizer directly with stubbed dependents resolution.
func TestFinalizeAffectedDependents(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	plainCalls, filterCalls := 0, 0
	resolvers := dependentsResolvers{
		plain: func(_ *schema.AtmosConfiguration, affected *[]schema.Affected, _, _, _ bool, _ []string, _ string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) error {
			plainCalls++
			(*affected)[0].Dependents = []schema.Dependent{typedDependent("app", "")}
			return nil
		},
		withFilter: func(_ *schema.AtmosConfiguration, affected *[]schema.Affected, opts *dependentsOptions) error {
			filterCalls++
			(*affected)[0].Dependents = []schema.Dependent{typedDependent("app", "auto")}
			opts.stacks = forestStacks(map[string]string{"vpc": "manual"})
			return nil
		},
	}

	t.Run("empty input resolves nothing", func(t *testing.T) {
		plainCalls, filterCalls = 0, 0
		var affected []schema.Affected
		require.NoError(t, finalizeAffectedDependents(atmosConfig, &affected, &AffectedDependentsOptions{}, resolvers))
		assert.Zero(t, plainCalls+filterCalls)
	})

	t.Run("without selectors the plain resolver is used", func(t *testing.T) {
		plainCalls, filterCalls = 0, 0
		affected := []schema.Affected{topLevelAffected("vpc")}
		require.NoError(t, finalizeAffectedDependents(atmosConfig, &affected, &AffectedDependentsOptions{}, resolvers))
		assert.Equal(t, 1, plainCalls)
		assert.Zero(t, filterCalls)
		assert.Equal(t, []string{"dev-app"}, dependentSlugs(affected[0].Dependents))
	})

	t.Run("with selectors the filtering resolver is used and the forest is pruned", func(t *testing.T) {
		plainCalls, filterCalls = 0, 0
		affected := []schema.Affected{topLevelAffected("vpc")}
		require.NoError(t, finalizeAffectedDependents(atmosConfig, &affected, &AffectedDependentsOptions{Filter: autoOnly}, resolvers))
		assert.Zero(t, plainCalls)
		assert.Equal(t, 1, filterCalls)
		assert.Equal(t, []string{"app"}, affectedComponents(affected), "the manual parent is replaced by its auto dependent")
	})

	t.Run("flatten runs after resolution", func(t *testing.T) {
		affected := []schema.Affected{topLevelAffected("vpc")}
		require.NoError(t, finalizeAffectedDependents(atmosConfig, &affected, &AffectedDependentsOptions{Flatten: true}, resolvers))
		assert.Equal(t, []string{"vpc", "app"}, affectedComponents(affected))
	})

	t.Run("a resolver error is returned", func(t *testing.T) {
		boom := cockroachErrors.New("boom")
		failing := dependentsResolvers{
			plain: func(*schema.AtmosConfiguration, *[]schema.Affected, bool, bool, bool, []string, string, auth.AuthManager, bool, DescribeStacksErrorOptions) error {
				return boom
			},
			withFilter: func(*schema.AtmosConfiguration, *[]schema.Affected, *dependentsOptions) error { return boom },
		}
		for _, opts := range []*AffectedDependentsOptions{{}, {Filter: autoOnly}} {
			affected := []schema.Affected{topLevelAffected("vpc")}
			require.ErrorIs(t, finalizeAffectedDependents(atmosConfig, &affected, opts, failing), boom)
		}
	})
}
