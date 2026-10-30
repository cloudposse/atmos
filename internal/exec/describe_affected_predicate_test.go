package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels so a rename of the fields these tests rely on fails the build here.
var (
	_ = DescribeAffectedCmdArgs{Filter: "", FilterEnvVar: ""}
	_ = schema.Affected{Settings: nil, Dependents: nil}
	_ = schema.Dependent{Settings: nil, Dependents: nil}
)

const actionsEnabledFilter = ".settings.github.actions_enabled // false"

// githubSettings builds a `settings` section with settings.github.actions_enabled set.
func githubSettings(enabled bool) schema.AtmosSectionMapType {
	return schema.AtmosSectionMapType{"github": map[string]any{"actions_enabled": enabled}}
}

// settingsAffected builds a terraform affected entry carrying github settings.
func settingsAffected(component string, enabled bool, dependents ...schema.Dependent) schema.Affected {
	a := topLevelAffected(component, dependents...)
	a.ComponentPath = filepath.Join("components", "terraform", component)
	a.Settings = githubSettings(enabled)
	return a
}

// settingsDependent builds a terraform dependent carrying github settings.
func settingsDependent(component string, enabled bool, children ...schema.Dependent) schema.Dependent {
	d := typedDependent(component, "", children...)
	d.Settings = githubSettings(enabled)
	return d
}

// filterExecHarness runs describeAffectedExec.Execute against an in-memory affected list. Dependents
// are attached by the stubbed resolver from dependentsByComponent, and every value handed to the
// non-matrix printer is captured.
type filterExecHarness struct {
	t                *testing.T
	affected         []schema.Affected
	dependentsByComp map[string][]schema.Dependent
	printed          any
	checkoutCalls    int
	exec             describeAffectedExec
}

func newFilterExecHarness(t *testing.T, affected []schema.Affected, dependentsByComp map[string][]schema.Dependent) *filterExecHarness {
	t.Helper()

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	h := &filterExecHarness{t: t, affected: affected, dependentsByComp: dependentsByComp}
	h.exec = describeAffectedExec{atmosConfig: &schema.AtmosConfiguration{}}
	h.exec.IsTTYSupportForStdout = func() bool { return false }
	h.exec.executeDescribeAffectedWithTargetRefCheckout = func(
		_ *schema.AtmosConfiguration,
		_, _, _ string,
		_, _ bool,
		_ string, _, _ bool,
		_ []string, _ AffectedFilter,
		_ auth.AuthManager,
		_ bool,
		_ DescribeStacksErrorOptions,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		h.checkoutCalls++
		return h.affected, nil, nil, "", nil
	}
	h.exec.addDependentsToAffected = func(_ *schema.AtmosConfiguration, affected *[]schema.Affected, _, _, _ bool, _ []string, _ string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) error {
		for i := range *affected {
			(*affected)[i].Dependents = h.dependentsByComp[(*affected)[i].Component]
		}
		return nil
	}
	h.exec.printOrWriteToFile = func(_ *schema.AtmosConfiguration, _, _ string, v any) error {
		h.printed = v
		return nil
	}
	return h
}

// run executes with the given args and returns the error.
func (h *filterExecHarness) run(args DescribeAffectedCmdArgs) error {
	h.t.Helper()
	args.CLIConfig = h.exec.atmosConfig
	return h.exec.Execute(&args)
}

// matrixLine runs a matrix execution that writes to a temp file and returns the `matrix=` JSON value and the file path.
func (h *filterExecHarness) matrixLine(args DescribeAffectedCmdArgs) (string, string) {
	h.t.Helper()
	outputFile := filepath.Join(h.t.TempDir(), "github_output")
	args.Format = "matrix"
	args.GithubOutputFile = outputFile
	require.NoError(h.t, h.run(args))

	content, err := os.ReadFile(outputFile)
	require.NoError(h.t, err)
	for _, line := range strings.Split(string(content), "\n") {
		if value, ok := strings.CutPrefix(line, "matrix="); ok {
			return value, outputFile
		}
	}
	h.t.Fatalf("no matrix= line in %q", string(content))
	return "", outputFile
}

// printedComponents returns the top-level component names of the last non-matrix payload.
func (h *filterExecHarness) printedComponents() []string {
	h.t.Helper()
	affected, ok := h.printed.([]schema.Affected)
	require.True(h.t, ok, "printed payload is %T", h.printed)
	return affectedComponents(affected)
}

func matrixEntry(component string) string {
	return `{"stack":"dev","component":"` + component + `","component_path":"components/terraform/` + component + `","component_type":"terraform"}`
}

// TestMatrixOutputWithoutFilterIsUnchanged pins the baseline: with neither --filter nor --query the matrix is exactly the unfiltered list.
func TestMatrixOutputWithoutFilterIsUnchanged(t *testing.T) {
	h := newFilterExecHarness(t, []schema.Affected{settingsAffected("vpc", true), settingsAffected("eks", false)}, nil)

	got, _ := h.matrixLine(DescribeAffectedCmdArgs{})

	assert.Equal(t, `{"include":[`+matrixEntry("vpc")+`,`+matrixEntry("eks")+`]}`, got)
}

// TestFilter_MatrixAndJSON covers the reported scenario: --filter keeps the matrix aligned with the structured formats.
func TestFilter_MatrixAndJSON(t *testing.T) {
	tests := []struct {
		name     string
		affected []schema.Affected
		filter   string
		want     []string
	}{
		{
			name:     "filter matches none",
			affected: []schema.Affected{settingsAffected("test", false)},
			filter:   actionsEnabledFilter,
			want:     []string{},
		},
		{
			name:     "filter matches a subset",
			affected: []schema.Affected{settingsAffected("a", true), settingsAffected("b", false), settingsAffected("c", true)},
			filter:   actionsEnabledFilter,
			want:     []string{"a", "c"},
		},
		{
			name:     "filter matches all",
			affected: []schema.Affected{settingsAffected("a", true), settingsAffected("b", true)},
			filter:   actionsEnabledFilter,
			want:     []string{"a", "b"},
		},
		{
			name:     "empty filter is identity",
			affected: []schema.Affected{settingsAffected("a", false), settingsAffected("b", true)},
			filter:   "",
			want:     []string{"a", "b"},
		},
		{
			name:     "filter on a plain field",
			affected: []schema.Affected{settingsAffected("vpc", true), settingsAffected("eks", true)},
			filter:   `.component == "eks"`,
			want:     []string{"eks"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantEntries := make([]string, 0, len(tt.want))
			for _, c := range tt.want {
				wantEntries = append(wantEntries, matrixEntry(c))
			}
			wantMatrix := `{"include":[` + strings.Join(wantEntries, ",") + `]}`

			matrixHarness := newFilterExecHarness(t, tt.affected, nil)
			gotMatrix, _ := matrixHarness.matrixLine(DescribeAffectedCmdArgs{Filter: tt.filter, IncludeSettings: true})
			assert.Equal(t, wantMatrix, gotMatrix)

			for _, format := range []string{"json", "yaml"} {
				structured := newFilterExecHarness(t, tt.affected, nil)
				require.NoError(t, structured.run(DescribeAffectedCmdArgs{Format: format, Filter: tt.filter, IncludeSettings: true}))
				got := structured.printedComponents()
				if len(tt.want) == 0 {
					assert.Empty(t, got, format)
				} else {
					assert.Equal(t, tt.want, got, format)
				}
			}
		})
	}
}

// TestFilter_InvalidExpression returns an error and writes nothing.
func TestFilter_InvalidExpression(t *testing.T) {
	h := newFilterExecHarness(t, []schema.Affected{settingsAffected("vpc", true)}, nil)
	outputFile := filepath.Join(t.TempDir(), "github_output")

	err := h.run(DescribeAffectedCmdArgs{Format: "matrix", GithubOutputFile: outputFile, Filter: ".[ not valid"})

	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	_, statErr := os.Stat(outputFile)
	assert.True(t, os.IsNotExist(statErr), "no matrix file must be written for an invalid filter")
}

// TestFilter_WritesFilteredMatrixToOutputFiles covers explicit --output-file and CI auto-detected GITHUB_OUTPUT.
func TestFilter_WritesFilteredMatrixToOutputFiles(t *testing.T) {
	affected := []schema.Affected{settingsAffected("on", true), settingsAffected("off", false)}

	t.Run("explicit output file", func(t *testing.T) {
		h := newFilterExecHarness(t, affected, nil)
		_, file := h.matrixLine(DescribeAffectedCmdArgs{Filter: actionsEnabledFilter, IncludeSettings: true})

		content, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Contains(t, string(content), matrixEntry("on"))
		assert.NotContains(t, string(content), matrixEntry("off"))
		assert.Contains(t, string(content), "count=1")
	})

	t.Run("CI auto-detected GITHUB_OUTPUT", func(t *testing.T) {
		autoFile := filepath.Join(t.TempDir(), "auto_output")
		t.Setenv("GITHUB_OUTPUT", autoFile)
		h := newFilterExecHarness(t, affected, nil)
		h.exec.atmosConfig = &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}

		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "matrix", Filter: actionsEnabledFilter, IncludeSettings: true}))

		content, err := os.ReadFile(autoFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), matrixEntry("on"))
		assert.NotContains(t, string(content), matrixEntry("off"))
	})

	t.Run("an empty result still writes the empty matrix and a zero count", func(t *testing.T) {
		h := newFilterExecHarness(t, []schema.Affected{settingsAffected("off", false)}, nil)
		got, file := h.matrixLine(DescribeAffectedCmdArgs{Filter: actionsEnabledFilter, IncludeSettings: true})

		assert.Equal(t, `{"include":[]}`, got)
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Contains(t, string(content), "count=0")
	})
}

// TestPruneAffectedByPredicate_Isolation verifies the result and the input do not alias each other, in both directions.
func TestPruneAffectedByPredicate_Isolation(t *testing.T) {
	pred, err := newAffectedPredicate(&schema.AtmosConfiguration{}, `.component != "drop"`)
	require.NoError(t, err)

	input := []schema.Affected{
		topLevelAffected("keep", typedDependent("child", ""), typedDependent("drop", "")),
		topLevelAffected("drop"),
		topLevelAffected("last"),
	}

	got, err := pruneAffectedByPredicate(input, pred)
	require.NoError(t, err)
	require.Equal(t, []string{"keep", "last"}, affectedComponents(got))
	require.Len(t, got[0].Dependents, 1)
	assert.Equal(t, "child", got[0].Dependents[0].Component)

	// result -> input: mutating the result leaves the input untouched.
	got[0].Component = "mutated"
	got[0].Dependents[0].Component = "mutated"
	assert.Equal(t, "keep", input[0].Component)
	assert.Equal(t, []string{"child", "drop"}, []string{input[0].Dependents[0].Component, input[0].Dependents[1].Component})
	assert.Len(t, input, 3)

	// input -> result: mutating the input afterwards leaves a fresh result untouched.
	fresh, err := pruneAffectedByPredicate(input, pred)
	require.NoError(t, err)
	input[0].Dependents[0].Component = "changed-after"
	input[2].Component = "changed-after"
	assert.Equal(t, "child", fresh[0].Dependents[0].Component)
	assert.Equal(t, "last", fresh[1].Component)
}

// TestPruneAffectedByPredicate_NilPredicateIsIdentity keeps the no-filter path allocation-free and unchanged.
func TestPruneAffectedByPredicate_NilPredicateIsIdentity(t *testing.T) {
	input := []schema.Affected{topLevelAffected("a"), topLevelAffected("b")}

	got, err := pruneAffectedByPredicate(input, nil)

	require.NoError(t, err)
	assert.Equal(t, input, got)
}

// TestFilter_DependentsPruning covers the subtree rule: a failing item is dropped with all of its dependents.
func TestFilter_DependentsPruning(t *testing.T) {
	// Tree: a -> [b -> [c, d], e], f -> [g]. The filter drops by component name.
	build := func() map[string][]schema.Dependent {
		return map[string][]schema.Dependent{
			"a": {
				typedDependent("b", "", typedDependent("c", ""), typedDependent("d", "")),
				typedDependent("e", ""),
			},
			"f": {typedDependent("g", "")},
		}
	}
	top := func() []schema.Affected { return []schema.Affected{topLevelAffected("a"), topLevelAffected("f")} }

	dependentNames := func(deps []schema.Dependent) []string {
		names := make([]string, 0, len(deps))
		for i := range deps {
			names = append(names, deps[i].Component)
		}
		return names
	}

	t.Run("a failing nested dependent drops only its branch", func(t *testing.T) {
		h := newFilterExecHarness(t, top(), build())
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, Filter: `.component != "b"`}))

		got := h.printed.([]schema.Affected)
		require.Len(t, got, 2)
		assert.Equal(t, []string{"e"}, dependentNames(got[0].Dependents), "b and its children c, d are gone; sibling e stays")
		assert.Equal(t, []string{"g"}, dependentNames(got[1].Dependents))
	})

	t.Run("a failing dependent is dropped even when its child would pass on its own", func(t *testing.T) {
		h := newFilterExecHarness(t, top(), build())
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, Filter: `.component != "b" and .component != "d"`}))

		got := h.printed.([]schema.Affected)
		assert.Equal(t, []string{"e"}, dependentNames(got[0].Dependents))
	})

	t.Run("a failing top-level item drops its subtree and is not replaced by its passing dependents", func(t *testing.T) {
		h := newFilterExecHarness(t, top(), build())
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, Filter: `.component != "a"`}))

		got := h.printed.([]schema.Affected)
		assert.Equal(t, []string{"f"}, affectedComponents(got))
		assert.Equal(t, []string{"g"}, dependentNames(got[0].Dependents))
	})

	t.Run("without a failing ancestor nothing is pruned", func(t *testing.T) {
		h := newFilterExecHarness(t, top(), build())
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, Filter: `.component != "zzz"`}))

		got := h.printed.([]schema.Affected)
		assert.Equal(t, []string{"b", "e"}, dependentNames(got[0].Dependents))
		assert.Equal(t, []string{"c", "d"}, dependentNames(got[0].Dependents[0].Dependents))
	})

	t.Run("settings on dependents are visible to the filter", func(t *testing.T) {
		deps := map[string][]schema.Dependent{"a": {settingsDependent("on", true), settingsDependent("off", false)}}
		h := newFilterExecHarness(t, []schema.Affected{settingsAffected("a", true)}, deps)
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, IncludeSettings: true, Filter: actionsEnabledFilter}))

		got := h.printed.([]schema.Affected)
		require.Len(t, got, 1)
		assert.Equal(t, []string{"on"}, dependentNames(got[0].Dependents))
	})

	t.Run("flattening happens after pruning", func(t *testing.T) {
		h := newFilterExecHarness(t, top(), build())
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, Flatten: true, Filter: `.component != "b"`}))

		// c and d hang under the filtered-out b, so they must not be lifted into the flat list.
		assert.Equal(t, []string{"a", "e", "f", "g"}, h.printedComponents())
	})

	t.Run("flattening keeps passing branches", func(t *testing.T) {
		h := newFilterExecHarness(t, top(), build())
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, Flatten: true, Filter: `.component != "zzz"`}))

		assert.Equal(t, []string{"a", "b", "c", "d", "e", "f", "g"}, h.printedComponents())
	})

	t.Run("the matrix follows the pruned and flattened list", func(t *testing.T) {
		h := newFilterExecHarness(t, top(), build())
		got, _ := h.matrixLine(DescribeAffectedCmdArgs{IncludeDependents: true, Flatten: true, Filter: `.component != "a"`})

		// These fixtures carry no component path, so both entries have an empty one.
		entry := func(component string) string {
			return `{"stack":"dev","component":"` + component + `","component_path":"","component_type":"terraform"}`
		}
		assert.Equal(t, `{"include":[`+entry("f")+`,`+entry("g")+`]}`, got)
	})
}

// TestFilter_RecomputesIncludedInDependents verifies the flag is recomputed from the pruned forest.
func TestFilter_RecomputesIncludedInDependents(t *testing.T) {
	// "child" is both a top-level item and a dependent of "root". Dropping "root" leaves it top-level only.
	childTop := topLevelAffected("child")
	root := topLevelAffected("root")
	deps := map[string][]schema.Dependent{"root": {typedDependent("child", "")}}

	t.Run("kept parent keeps the flag", func(t *testing.T) {
		h := newFilterExecHarness(t, []schema.Affected{root, childTop}, deps)
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, Filter: `.component != "zzz"`}))

		got := h.printed.([]schema.Affected)
		require.Len(t, got, 2)
		assert.True(t, got[1].IncludedInDependents)
	})

	t.Run("dropped parent clears the flag", func(t *testing.T) {
		h := newFilterExecHarness(t, []schema.Affected{root, childTop}, deps)
		require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeDependents: true, Filter: `.component != "root"`}))

		got := h.printed.([]schema.Affected)
		require.Len(t, got, 1)
		assert.Equal(t, "child", got[0].Component)
		assert.False(t, got[0].IncludedInDependents)
	})
}

// TestValidateDescribeAffectedArgs_QueryWithMatrix is the regression test for the original report:
// --query combined with --format=matrix used to be silently ignored and produced the unfiltered matrix.
func TestValidateDescribeAffectedArgs_QueryWithMatrix(t *testing.T) {
	const originalQuery = `.[] | select((.settings.github.actions_enabled // false) == true) | [.]`

	t.Run("the original report is rejected with a hint to --filter", func(t *testing.T) {
		err := validateDescribeAffectedArgs(&DescribeAffectedCmdArgs{Format: "matrix", Query: originalQuery, ErrorMode: "warn"})

		require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
		assert.Contains(t, errUtils.Format(err, errUtils.FormatterConfig{}), "--filter")
	})

	t.Run("a matrix without a query is accepted", func(t *testing.T) {
		require.NoError(t, validateDescribeAffectedArgs(&DescribeAffectedCmdArgs{Format: "matrix", ErrorMode: "warn"}))
	})

	for _, format := range []string{"json", "yaml"} {
		t.Run("--query with "+format+" is unchanged", func(t *testing.T) {
			require.NoError(t, validateDescribeAffectedArgs(&DescribeAffectedCmdArgs{Format: format, Query: originalQuery, ErrorMode: "warn"}))
		})
	}
}

// TestFilter_RunsBeforeQuery verifies the query sees only the filtered list for structured formats.
func TestFilter_RunsBeforeQuery(t *testing.T) {
	h := newFilterExecHarness(t, []schema.Affected{settingsAffected("on", true), settingsAffected("off", false)}, nil)

	require.NoError(t, h.run(DescribeAffectedCmdArgs{Format: "json", IncludeSettings: true, Filter: actionsEnabledFilter, Query: "length"}))

	// The query result (a number) is what reaches the printer, and it counts only the filtered items.
	assert.EqualValues(t, 1, h.printed)
}

// TestResolveAffectedFilter covers the --upload rule, which mirrors --tags/--labels.
func TestResolveAffectedFilter(t *testing.T) {
	t.Run("the flag form with --upload is an error", func(t *testing.T) {
		a := &DescribeAffectedCmdArgs{Upload: true, Filter: actionsEnabledFilter}

		err := resolveAffectedFilter(a)

		require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
		assert.Contains(t, errUtils.Format(err, errUtils.FormatterConfig{}), "--filter")
	})

	t.Run("the env form with --upload is dropped", func(t *testing.T) {
		a := &DescribeAffectedCmdArgs{Upload: true, Filter: actionsEnabledFilter, FilterEnvVar: "ATMOS_DESCRIBE_AFFECTED_FILTER"}

		require.NoError(t, resolveAffectedFilter(a))

		assert.Empty(t, a.Filter)
	})

	t.Run("negative path: without --upload the filter is never cleared", func(t *testing.T) {
		a := &DescribeAffectedCmdArgs{Filter: actionsEnabledFilter, FilterEnvVar: "ATMOS_DESCRIBE_AFFECTED_FILTER"}

		require.NoError(t, resolveAffectedFilter(a))

		assert.Equal(t, actionsEnabledFilter, a.Filter)
	})

	t.Run("no filter with --upload is fine", func(t *testing.T) {
		require.NoError(t, resolveAffectedFilter(&DescribeAffectedCmdArgs{Upload: true}))
	})

	t.Run("it runs as part of the argument validation", func(t *testing.T) {
		err := validateDescribeAffectedArgs(&DescribeAffectedCmdArgs{Format: "json", Upload: true, Filter: ".component", ErrorMode: "warn"})

		require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	})
}

// TestFilter_CombinesWithLabelSelectors verifies a result must satisfy both the selector and the filter,
// and that the filter does not promote dependents of a dropped item the way the selectors do.
func TestFilter_CombinesWithLabelSelectors(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}
	current := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{
		"auto-on":    map[string]any{"metadata": map[string]any{"labels": map[string]any{"ci": "auto"}}, "settings": githubSettings(true)},
		"auto-off":   map[string]any{"metadata": map[string]any{"labels": map[string]any{"ci": "auto"}}, "settings": githubSettings(false)},
		"manual-on":  map[string]any{"metadata": map[string]any{"labels": map[string]any{"ci": "manual"}}, "settings": githubSettings(true)},
		"manual-off": map[string]any{"metadata": map[string]any{"labels": map[string]any{"ci": "manual"}}, "settings": githubSettings(false)},
	}}}}
	remote := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{}}}}

	d := describeAffectedExec{atmosConfig: atmosConfig}
	d.IsTTYSupportForStdout = func() bool { return false }
	d.executeDescribeAffectedWithTargetRefCheckout = func(
		_ *schema.AtmosConfiguration,
		_, _, _ string,
		_, _ bool,
		_ string, _, _ bool,
		_ []string, filter AffectedFilter,
		_ auth.AuthManager,
		_ bool,
		_ DescribeStacksErrorOptions,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		affected, findErr := findAffected(&current, &remote, atmosConfig, nil, false, true, "", filter, "")
		return affected, nil, nil, "", findErr
	}
	d.addDependentsToAffected = func(*schema.AtmosConfiguration, *[]schema.Affected, bool, bool, bool, []string, string, auth.AuthManager, bool, DescribeStacksErrorOptions) error {
		return nil
	}

	outputFile := filepath.Join(t.TempDir(), "github_output")
	args := DescribeAffectedCmdArgs{
		Format:           "matrix",
		GithubOutputFile: outputFile,
		CLIConfig:        atmosConfig,
		LabelsRaw:        "ci=auto",
		Filter:           actionsEnabledFilter,
		IncludeSettings:  true,
	}
	require.NoError(t, resolveAffectedSelectors(&args))
	require.NoError(t, d.Execute(&args))

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "auto-on")
	assert.NotContains(t, string(content), "auto-off")
	assert.NotContains(t, string(content), "manual-on")
	assert.NotContains(t, string(content), "manual-off")
	assert.Contains(t, string(content), "count=1")
}

// TestNewAffectedPredicate covers the builder directly: empty means no filtering, a bad expression fails
// even with nothing to evaluate it against.
func TestNewAffectedPredicate(t *testing.T) {
	t.Run("an empty expression returns a nil predicate", func(t *testing.T) {
		pred, err := newAffectedPredicate(&schema.AtmosConfiguration{}, "")

		require.NoError(t, err)
		assert.Nil(t, pred)
	})

	t.Run("a syntax error is reported without any item to evaluate", func(t *testing.T) {
		_, err := newAffectedPredicate(&schema.AtmosConfiguration{}, ".[ not valid")

		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	})

	t.Run("a valid expression returns a working predicate", func(t *testing.T) {
		pred, err := newAffectedPredicate(&schema.AtmosConfiguration{}, `.component == "vpc"`)
		require.NoError(t, err)

		yes, err := pred(map[string]any{"component": "vpc"})
		require.NoError(t, err)
		no, err := pred(map[string]any{"component": "eks"})
		require.NoError(t, err)
		assert.True(t, yes)
		assert.False(t, no)
	})
}

// TestPruneAffectedByPredicate_PropagatesErrors verifies a predicate failure aborts at any depth and returns no partial result.
func TestPruneAffectedByPredicate_PropagatesErrors(t *testing.T) {
	boom := errUtils.ErrInvalidConfig
	failOn := func(component string) affectedPredicate {
		return func(item map[string]any) (bool, error) {
			if item["component"] == component {
				return false, boom
			}
			return true, nil
		}
	}

	input := []schema.Affected{topLevelAffected("top", typedDependent("child", "", typedDependent("grandchild", "")))}

	for _, component := range []string{"top", "child", "grandchild"} {
		t.Run("failure on "+component, func(t *testing.T) {
			got, err := pruneAffectedByPredicate(input, failOn(component))

			require.ErrorIs(t, err, boom)
			assert.Nil(t, got)
		})
	}

	t.Run("negative path: a predicate that never fails returns everything", func(t *testing.T) {
		got, err := pruneAffectedByPredicate(input, failOn("absent"))

		require.NoError(t, err)
		assert.Equal(t, []string{"top"}, affectedComponents(got))
	})
}

// TestAffectedItemToMap covers the conversion, including values that cannot be encoded or decoded as an object.
func TestAffectedItemToMap(t *testing.T) {
	t.Run("an affected item uses its json field names", func(t *testing.T) {
		m, err := affectedItemToMap(settingsAffected("vpc", true))

		require.NoError(t, err)
		assert.Equal(t, "vpc", m["component"])
		assert.Equal(t, "dev", m["stack"])
		assert.Equal(t, true, m["settings"].(map[string]any)["github"].(map[string]any)["actions_enabled"])
	})

	t.Run("an unencodable value is an error", func(t *testing.T) {
		_, err := affectedItemToMap(make(chan int))

		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	})

	t.Run("a value that is not an object is an error", func(t *testing.T) {
		_, err := affectedItemToMap("not an object")

		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	})
}
