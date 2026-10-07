package exec

import (
	"os"
	"path/filepath"
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

// Compile-time sentinel so a rename of the filter fields fails the build here.
var _ = AffectedFilter{ExcludeLocked: true, Tags: []string{"a"}, Labels: map[string]string{"a": "b"}}

// TestShouldSkipComponent_Selectors covers the abstract/disabled/locked exclusions and the
// --tags/--labels selectors applied by shouldSkipComponent.
func TestShouldSkipComponent_Selectors(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		filter   AffectedFilter
		wantSkip bool
	}{
		{
			name:     "no filter keeps component without tags or labels",
			metadata: map[string]any{},
			filter:   AffectedFilter{},
			wantSkip: false,
		},
		{
			name:     "abstract component is skipped",
			metadata: map[string]any{"type": "abstract"},
			filter:   AffectedFilter{},
			wantSkip: true,
		},
		{
			name:     "disabled component is skipped",
			metadata: map[string]any{"enabled": false},
			filter:   AffectedFilter{},
			wantSkip: true,
		},
		{
			name:     "abstract component is skipped even when selectors match",
			metadata: map[string]any{"type": "abstract", "labels": map[string]any{"ci": "auto"}},
			filter:   AffectedFilter{Labels: map[string]string{"ci": "auto"}},
			wantSkip: true,
		},
		{
			name:     "disabled component is skipped even when selectors match",
			metadata: map[string]any{"enabled": false, "tags": []any{"prod"}},
			filter:   AffectedFilter{Tags: []string{"prod"}},
			wantSkip: true,
		},
		{
			name:     "locked component is kept without ExcludeLocked",
			metadata: map[string]any{"locked": true},
			filter:   AffectedFilter{},
			wantSkip: false,
		},
		{
			name:     "locked component is skipped with ExcludeLocked",
			metadata: map[string]any{"locked": true},
			filter:   AffectedFilter{ExcludeLocked: true},
			wantSkip: true,
		},
		{
			name:     "tag match keeps component",
			metadata: map[string]any{"tags": []any{"prod", "tier-1"}},
			filter:   AffectedFilter{Tags: []string{"tier-1"}},
			wantSkip: false,
		},
		{
			name:     "tags match any of the selector tags",
			metadata: map[string]any{"tags": []any{"prod"}},
			filter:   AffectedFilter{Tags: []string{"dev", "prod"}},
			wantSkip: false,
		},
		{
			name:     "tag mismatch skips component",
			metadata: map[string]any{"tags": []any{"dev"}},
			filter:   AffectedFilter{Tags: []string{"prod"}},
			wantSkip: true,
		},
		{
			name:     "label match keeps component",
			metadata: map[string]any{"labels": map[string]any{"ci": "auto", "team": "platform"}},
			filter:   AffectedFilter{Labels: map[string]string{"ci": "auto"}},
			wantSkip: false,
		},
		{
			name:     "label value mismatch skips component",
			metadata: map[string]any{"labels": map[string]any{"ci": "manual"}},
			filter:   AffectedFilter{Labels: map[string]string{"ci": "auto"}},
			wantSkip: true,
		},
		{
			name:     "labels must all match",
			metadata: map[string]any{"labels": map[string]any{"ci": "auto"}},
			filter:   AffectedFilter{Labels: map[string]string{"ci": "auto", "team": "platform"}},
			wantSkip: true,
		},
		{
			name: "tags and labels both match",
			metadata: map[string]any{
				"tags":   []any{"prod"},
				"labels": map[string]any{"ci": "auto"},
			},
			filter:   AffectedFilter{Tags: []string{"prod"}, Labels: map[string]string{"ci": "auto"}},
			wantSkip: false,
		},
		{
			name: "tags match but labels mismatch skips component",
			metadata: map[string]any{
				"tags":   []any{"prod"},
				"labels": map[string]any{"ci": "manual"},
			},
			filter:   AffectedFilter{Tags: []string{"prod"}, Labels: map[string]string{"ci": "auto"}},
			wantSkip: true,
		},
		{
			name: "labels match but tags mismatch skips component",
			metadata: map[string]any{
				"tags":   []any{"dev"},
				"labels": map[string]any{"ci": "auto"},
			},
			filter:   AffectedFilter{Tags: []string{"prod"}, Labels: map[string]string{"ci": "auto"}},
			wantSkip: true,
		},
		{
			name:     "metadata without tags is skipped when a tag selector is set",
			metadata: map[string]any{"component": "vpc"},
			filter:   AffectedFilter{Tags: []string{"prod"}},
			wantSkip: true,
		},
		{
			name:     "metadata without labels is skipped when a label selector is set",
			metadata: map[string]any{"component": "vpc"},
			filter:   AffectedFilter{Labels: map[string]string{"ci": "auto"}},
			wantSkip: true,
		},
		{
			name:     "unrendered template tags never match a selector",
			metadata: map[string]any{"tags": "{{ .vars.tags }}"},
			filter:   AffectedFilter{Tags: []string{"prod"}},
			wantSkip: true,
		},
		{
			name:     "selectors alone do not skip a component that satisfies them",
			metadata: map[string]any{"locked": true, "labels": map[string]any{"ci": "auto"}},
			filter:   AffectedFilter{Labels: map[string]string{"ci": "auto"}},
			wantSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantSkip, shouldSkipComponent(tt.metadata, "vpc", tt.filter))
		})
	}
}

// TestAffectedFilter_HasSelectors verifies that only tags and labels count as selectors.
func TestAffectedFilter_HasSelectors(t *testing.T) {
	assert.False(t, AffectedFilter{}.hasSelectors())
	assert.False(t, AffectedFilter{ExcludeLocked: true}.hasSelectors())
	assert.True(t, AffectedFilter{Tags: []string{"prod"}}.hasSelectors())
	assert.True(t, AffectedFilter{Labels: map[string]string{"ci": "auto"}}.hasSelectors())
}

// TestFindAffected_Selectors checks the selectors through the full component-processing path,
// including components that have no metadata section at all.
func TestFindAffected_Selectors(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}
	newComponents := func() map[string]any {
		return map[string]any{
			"auto": map[string]any{"metadata": map[string]any{
				"component": "auto",
				"tags":      []any{"safe"},
				"labels":    map[string]any{"ci": "auto"},
			}},
			"manual": map[string]any{"metadata": map[string]any{
				"component": "manual",
				"labels":    map[string]any{"ci": "manual"},
			}},
			"bare": map[string]any{"vars": map[string]any{"a": "b"}},
		}
	}

	tests := []struct {
		name   string
		filter AffectedFilter
		want   []string
	}{
		{name: "no selector reports every component", filter: AffectedFilter{}, want: []string{"auto", "manual", "bare"}},
		{name: "label selector excludes manual and metadata-less components", filter: AffectedFilter{Labels: map[string]string{"ci": "auto"}}, want: []string{"auto"}},
		{name: "tag selector keeps only tagged components", filter: AffectedFilter{Tags: []string{"safe"}}, want: []string{"auto"}},
		{name: "unmatched selector reports nothing", filter: AffectedFilter{Labels: map[string]string{"ci": "never"}}, want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": newComponents()}}}
			remote := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{}}}}

			affected, err := findAffected(&current, &remote, atmosConfig, nil, false, false, "", tt.filter, "")
			require.NoError(t, err)

			got := make([]string, 0, len(affected))
			for _, a := range affected {
				got = append(got, a.Component)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

// TestResolveAffectedSelectors covers --tags/--labels normalization and validation.
func TestResolveAffectedSelectors(t *testing.T) {
	tests := []struct {
		name       string
		args       DescribeAffectedCmdArgs
		wantTags   []string
		wantLabels map[string]string
		wantErr    error
	}{
		{name: "nothing set", args: DescribeAffectedCmdArgs{}},
		{
			name:       "tags and labels are normalized",
			args:       DescribeAffectedCmdArgs{Tags: []string{" prod ", "", "tier-1"}, LabelsRaw: "ci=auto, team:platform"},
			wantTags:   []string{"prod", "tier-1"},
			wantLabels: map[string]string{"ci": "auto", "team": "platform"},
		},
		{
			name:    "malformed labels are rejected",
			args:    DescribeAffectedCmdArgs{LabelsRaw: "not-a-pair"},
			wantErr: errUtils.ErrInvalidFlag,
		},
		{
			name:    "upload with tags is rejected",
			args:    DescribeAffectedCmdArgs{Upload: true, Tags: []string{"prod"}},
			wantErr: errUtils.ErrInvalidFlag,
		},
		{
			name:    "upload with labels is rejected",
			args:    DescribeAffectedCmdArgs{Upload: true, LabelsRaw: "ci=auto"},
			wantErr: errUtils.ErrInvalidFlag,
		},
		{
			name: "upload without selectors is accepted",
			args: DescribeAffectedCmdArgs{Upload: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			err := resolveAffectedSelectors(&args)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTags, args.Tags)
			assert.Equal(t, tt.wantLabels, args.Labels)
		})
	}
}

// TestResolveAffectedSelectors_UploadMessage pins the user-facing explanation for the rejected combination.
func TestResolveAffectedSelectors_UploadMessage(t *testing.T) {
	args := DescribeAffectedCmdArgs{Upload: true, LabelsRaw: "ci=auto"}
	err := resolveAffectedSelectors(&args)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "--tags/--labels is not supported with --upload")
	assert.Contains(t, err.Error(), "the upload is always unfiltered")
}

// TestSetDescribeAffectedFlagValueInCliArgs_Selectors verifies --tags/--labels are read from the flag set
// and flow through resolveAffectedSelectors into the filter, and that --upload rejects them.
func TestSetDescribeAffectedFlagValueInCliArgs_Selectors(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "")

	newArgs := func(t *testing.T, set map[string]string) DescribeAffectedCmdArgs {
		t.Helper()
		flags := newDescribeAffectedFlagSet()
		for k, v := range set {
			require.NoError(t, flags.Set(k, v))
		}
		args := DescribeAffectedCmdArgs{CLIConfig: &schema.AtmosConfiguration{}}
		SetDescribeAffectedFlagValueInCliArgs(flags, &args)
		return args
	}

	t.Run("flags populate the filter", func(t *testing.T) {
		args := newArgs(t, map[string]string{"tags": "prod,tier-1", "labels": "ci=auto", "exclude-locked": "true"})
		require.NoError(t, resolveAffectedSelectors(&args))
		assert.Equal(t, AffectedFilter{
			ExcludeLocked: true,
			Tags:          []string{"prod", "tier-1"},
			Labels:        map[string]string{"ci": "auto"},
		}, args.affectedFilter())
	})

	t.Run("no flags yields an empty selector filter", func(t *testing.T) {
		args := newArgs(t, nil)
		require.NoError(t, resolveAffectedSelectors(&args))
		assert.False(t, args.affectedFilter().hasSelectors())
	})

	t.Run("bad labels are rejected", func(t *testing.T) {
		args := newArgs(t, map[string]string{"labels": "ci"})
		require.ErrorIs(t, resolveAffectedSelectors(&args), errUtils.ErrInvalidFlag)
	})

	t.Run("upload with selectors is rejected", func(t *testing.T) {
		args := newArgs(t, map[string]string{"upload": "true", "tags": "prod"})
		require.ErrorIs(t, resolveAffectedSelectors(&args), errUtils.ErrInvalidFlag)
	})
}

// TestExecute_MatrixFormat_LabelsExcludeManual is the end-to-end CI scenario: with --labels=ci=auto the
// matrix omits components labeled `ci: manual`, while the same run without the selector includes them.
func TestExecute_MatrixFormat_LabelsExcludeManual(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}
	current := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{
		"app-auto":   map[string]any{"metadata": map[string]any{"component": "app", "labels": map[string]any{"ci": "auto"}}},
		"iam-manual": map[string]any{"metadata": map[string]any{"component": "iam", "labels": map[string]any{"ci": "manual"}}},
	}}}}
	remote := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{}}}}

	d := describeAffectedExec{atmosConfig: atmosConfig}
	d.IsTTYSupportForStdout = func() bool { return false }
	// The stub resolves affected components from the in-memory stacks using the filter Execute built from the args.
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
		affected, err := findAffected(&current, &remote, atmosConfig, nil, false, false, "", filter, "")
		return affected, nil, nil, "", err
	}
	d.addDependentsToAffected = func(*schema.AtmosConfiguration, *[]schema.Affected, bool, bool, bool, []string, string, auth.AuthManager, bool, DescribeStacksErrorOptions) error {
		return nil
	}

	run := func(t *testing.T, args DescribeAffectedCmdArgs) string {
		t.Helper()
		outputFile := filepath.Join(t.TempDir(), "github_output")
		args.Format = "matrix"
		args.GithubOutputFile = outputFile
		args.CLIConfig = atmosConfig
		require.NoError(t, resolveAffectedSelectors(&args))
		require.NoError(t, d.Execute(&args))
		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		return string(content)
	}

	t.Run("labels=ci=auto omits the manual component", func(t *testing.T) {
		content := run(t, DescribeAffectedCmdArgs{LabelsRaw: "ci=auto"})
		assert.Contains(t, content, "app-auto")
		assert.NotContains(t, content, "iam-manual")
		assert.Contains(t, content, "count=1")
	})

	t.Run("without a selector both components are present", func(t *testing.T) {
		content := run(t, DescribeAffectedCmdArgs{})
		assert.Contains(t, content, "app-auto")
		assert.Contains(t, content, "iam-manual")
		assert.Contains(t, content, "count=2")
	})
}

// deletedSelectorFixture builds BASE (remote) and HEAD (current) stacks that cover every deletion path:
//   - stack "dev" exists in both; "iam-manual", "vpc-auto", "priv-tagged", "bare" and "abstract-auto" were removed from HEAD,
//     "kept-manual" still exists in HEAD.
//   - stack "old" was deleted entirely and held an auto and a manual component.
//   - stack "empty" exists in HEAD without a components section, so all its BASE components count as deleted.
func deletedSelectorFixture() (remote, current map[string]any) {
	comp := func(labels map[string]any, tags []any) map[string]any {
		metadata := map[string]any{"component": "x"}
		if labels != nil {
			metadata["labels"] = labels
		}
		if tags != nil {
			metadata["tags"] = tags
		}
		return map[string]any{"metadata": metadata}
	}
	auto := map[string]any{"ci": "auto"}
	manual := map[string]any{"ci": "manual"}

	remote = map[string]any{
		"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{
			"iam-manual":    comp(manual, []any{"privileged"}),
			"vpc-auto":      comp(auto, nil),
			"priv-tagged":   comp(nil, []any{"privileged", "tier-1"}),
			"bare":          map[string]any{"vars": map[string]any{"a": "b"}},
			"abstract-auto": map[string]any{"metadata": map[string]any{"type": "abstract", "labels": auto}},
			"kept-manual":   comp(manual, nil),
		}}},
		"old": map[string]any{"components": map[string]any{"terraform": map[string]any{
			"old-auto":   comp(auto, nil),
			"old-manual": comp(manual, []any{"privileged"}),
			"old-bare":   map[string]any{"vars": map[string]any{"a": "b"}},
		}}},
		"empty": map[string]any{"components": map[string]any{"terraform": map[string]any{
			"empty-auto":   comp(auto, nil),
			"empty-manual": comp(manual, nil),
		}}},
	}
	current = map[string]any{
		"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{
			"kept-manual": comp(manual, nil),
		}}},
		"empty": map[string]any{"vars": map[string]any{}},
	}
	return remote, current
}

// TestDetectDeletedComponents_Selectors verifies that --tags/--labels are evaluated against the deleted
// component's BASE metadata for single-component, whole-stack, and empty-components-section deletions.
func TestDetectDeletedComponents_Selectors(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}

	tests := []struct {
		name   string
		filter AffectedFilter
		want   []string // "stack/component" of every reported deletion.
	}{
		{
			name: "no selector reports every non-abstract deletion including metadata-less ones",
			want: []string{
				"dev/iam-manual", "dev/vpc-auto", "dev/priv-tagged", "dev/bare",
				"old/old-auto", "old/old-manual", "old/old-bare",
				"empty/empty-auto", "empty/empty-manual",
			},
		},
		{
			name:   "labels=ci=auto drops manual and metadata-less deletions on every path",
			filter: AffectedFilter{Labels: map[string]string{"ci": "auto"}},
			want:   []string{"dev/vpc-auto", "old/old-auto", "empty/empty-auto"},
		},
		{
			name:   "labels=ci=manual keeps only manual deletions",
			filter: AffectedFilter{Labels: map[string]string{"ci": "manual"}},
			want:   []string{"dev/iam-manual", "old/old-manual", "empty/empty-manual"},
		},
		{
			name:   "tags=privileged keeps only deletions tagged privileged",
			filter: AffectedFilter{Tags: []string{"privileged"}},
			want:   []string{"dev/iam-manual", "dev/priv-tagged", "old/old-manual"},
		},
		{
			name:   "tags are match-any and labels are match-all, combined as AND",
			filter: AffectedFilter{Tags: []string{"privileged", "nope"}, Labels: map[string]string{"ci": "manual"}},
			want:   []string{"dev/iam-manual", "old/old-manual"},
		},
		{
			name:   "unmatched selector reports no deletions",
			filter: AffectedFilter{Labels: map[string]string{"ci": "never"}},
			want:   []string{},
		},
		{
			name:   "exclude-locked alone does not change deletions",
			filter: AffectedFilter{ExcludeLocked: true},
			want: []string{
				"dev/iam-manual", "dev/vpc-auto", "dev/priv-tagged", "dev/bare",
				"old/old-auto", "old/old-manual", "old/old-bare",
				"empty/empty-auto", "empty/empty-manual",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, current := deletedSelectorFixture()

			deleted, err := detectDeletedComponents(&remote, &current, atmosConfig, "", tt.filter)
			require.NoError(t, err)

			got := make([]string, 0, len(deleted))
			for _, d := range deleted {
				assert.True(t, d.Deleted, "%s/%s must be flagged deleted", d.Stack, d.Component)
				got = append(got, d.Stack+"/"+d.Component)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

// TestDetectDeletedComponents_SelectorsKeepDeletionType verifies filtering does not alter the deletion
// classification of the entries that survive.
func TestDetectDeletedComponents_SelectorsKeepDeletionType(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}
	remote, current := deletedSelectorFixture()

	deleted, err := detectDeletedComponents(&remote, &current, atmosConfig, "", AffectedFilter{Labels: map[string]string{"ci": "auto"}})
	require.NoError(t, err)

	byKey := map[string]string{}
	for _, d := range deleted {
		byKey[d.Stack+"/"+d.Component] = d.DeletionType
	}
	assert.Equal(t, map[string]string{
		"dev/vpc-auto":     deletionTypeComponent,
		"old/old-auto":     deletionTypeStack,
		"empty/empty-auto": deletionTypeComponent,
	}, byKey)
}

// TestExecute_MatrixFormat_LabelsExcludeDeletedManual is the end-to-end regression for deleted components
// bypassing --labels: the matrix omits a deleted `ci: manual` component and keeps a deleted `ci: auto` one.
func TestExecute_MatrixFormat_LabelsExcludeDeletedManual(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}
	remote, current := deletedSelectorFixture()
	// Keep the added/modified side empty so only deletions are under test.
	current["dev"] = map[string]any{"components": map[string]any{"terraform": map[string]any{}}}

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
		affected, err := findAffected(&current, &remote, atmosConfig, nil, false, false, "", filter, "")
		return affected, nil, nil, "", err
	}

	run := func(t *testing.T, args DescribeAffectedCmdArgs) string {
		t.Helper()
		outputFile := filepath.Join(t.TempDir(), "github_output")
		args.Format = "matrix"
		args.GithubOutputFile = outputFile
		args.CLIConfig = atmosConfig
		require.NoError(t, resolveAffectedSelectors(&args))
		require.NoError(t, d.Execute(&args))
		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		return string(content)
	}

	t.Run("labels=ci=auto omits deleted manual components", func(t *testing.T) {
		content := run(t, DescribeAffectedCmdArgs{LabelsRaw: "ci=auto"})
		assert.Contains(t, content, "vpc-auto")
		assert.Contains(t, content, "old-auto")
		assert.NotContains(t, content, "iam-manual")
		assert.NotContains(t, content, "old-manual")
		assert.NotContains(t, content, "bare")
		assert.Contains(t, content, "count=3")
	})

	t.Run("without a selector the deleted manual component is present", func(t *testing.T) {
		content := run(t, DescribeAffectedCmdArgs{})
		assert.Contains(t, content, "iam-manual")
		assert.Contains(t, content, "vpc-auto")
	})
}

// dependentFixture builds a dependent record with the given metadata labels.
func dependentFixture(component, ci string, children ...schema.Dependent) schema.Dependent {
	d := schema.Dependent{
		Component:  component,
		Stack:      "dev",
		StackSlug:  "dev-" + component,
		Dependents: children,
	}
	if ci != "" {
		d.Metadata = map[string]any{"labels": map[string]any{"ci": ci}}
	}
	return d
}

func dependentSlugs(dependents []schema.Dependent) []string {
	slugs := make([]string, 0, len(dependents))
	for i := range dependents {
		slugs = append(slugs, dependents[i].StackSlug)
	}
	return slugs
}

// TestFilterAffectedDependents verifies selectors prune nested dependents recursively, promote matching
// descendants of a pruned dependent, and leave everything alone when no selector is set.
func TestFilterAffectedDependents(t *testing.T) {
	newAffected := func() []schema.Affected {
		return []schema.Affected{{
			Component: "vpc", Stack: "dev", StackSlug: "dev-vpc",
			Dependents: []schema.Dependent{
				// manual -> (auto, manual -> auto): the auto descendants must survive, promoted one level.
				dependentFixture(
					"iam", "manual",
					dependentFixture("app", "auto"),
					dependentFixture("db", "manual", dependentFixture("cache", "auto")),
				),
				// auto -> (manual leaf, auto leaf, no-metadata leaf).
				dependentFixture(
					"web", "auto",
					dependentFixture("batch", "manual"),
					dependentFixture("api", "auto"),
					dependentFixture("legacy", ""),
				),
				// A second path to "app" must not produce a duplicate once promoted.
				dependentFixture("ops", "manual", dependentFixture("app", "auto")),
			},
		}}
	}

	t.Run("no selector leaves dependents untouched", func(t *testing.T) {
		for _, filter := range []AffectedFilter{{}, {ExcludeLocked: true}} {
			affected := newAffected()
			filterAffectedDependents(&affected, filter)
			assert.Equal(t, newAffected()[0].Dependents, affected[0].Dependents)
		}
	})

	t.Run("labels=ci=auto drops manual and metadata-less dependents recursively", func(t *testing.T) {
		affected := newAffected()
		filterAffectedDependents(&affected, AffectedFilter{Labels: map[string]string{"ci": "auto"}})

		got := affected[0].Dependents
		// Promoted: app (from iam and ops, de-duplicated) and cache (from db under iam); kept: web.
		assert.Equal(t, []string{"dev-app", "dev-cache", "dev-web"}, dependentSlugs(got))
		for _, d := range got {
			if d.Component == "web" {
				assert.Equal(t, []string{"dev-api"}, dependentSlugs(d.Dependents))
			} else {
				assert.Empty(t, d.Dependents)
			}
		}
		// The affected component itself is never removed or altered.
		assert.Equal(t, "vpc", affected[0].Component)
	})

	t.Run("a selector that matches nothing leaves an empty, non-nil dependents list", func(t *testing.T) {
		affected := newAffected()
		filterAffectedDependents(&affected, AffectedFilter{Labels: map[string]string{"ci": "never"}})
		assert.NotNil(t, affected[0].Dependents)
		assert.Empty(t, affected[0].Dependents)
	})

	t.Run("tags selector is applied to dependents", func(t *testing.T) {
		affected := []schema.Affected{{
			Component: "vpc", Stack: "dev", StackSlug: "dev-vpc",
			Dependents: []schema.Dependent{
				{Component: "a", Stack: "dev", StackSlug: "dev-a", Metadata: map[string]any{"tags": []any{"privileged"}}},
				{Component: "b", Stack: "dev", StackSlug: "dev-b", Metadata: map[string]any{"tags": []any{"other"}}},
			},
		}}
		filterAffectedDependents(&affected, AffectedFilter{Tags: []string{"privileged"}})
		assert.Equal(t, []string{"dev-a"}, dependentSlugs(affected[0].Dependents))
	})

	t.Run("nil dependents stay nil", func(t *testing.T) {
		affected := []schema.Affected{{Component: "vpc", StackSlug: "dev-vpc"}}
		filterAffectedDependents(&affected, AffectedFilter{Labels: map[string]string{"ci": "auto"}})
		assert.Nil(t, affected[0].Dependents)
	})
}

// TestFilterAffectedDependents_IncludedInDependents verifies included_in_dependents is recomputed from the
// pruned trees: an affected component that was only "included" through a pruned dependent is no longer so.
func TestFilterAffectedDependents_IncludedInDependents(t *testing.T) {
	newAffected := func() []schema.Affected {
		return []schema.Affected{
			{
				Component: "vpc", Stack: "dev", StackSlug: "dev-vpc",
				Dependents: []schema.Dependent{
					dependentFixture("iam", "manual"),
					dependentFixture("web", "auto"),
				},
			},
			{Component: "iam", Stack: "dev", StackSlug: "dev-iam"},
			{Component: "web", Stack: "dev", StackSlug: "dev-web"},
		}
	}

	// Baseline: both iam and web are reachable as dependents of vpc.
	baseline := newAffected()
	processIncludedInDependencies(&baseline)
	require.False(t, baseline[0].IncludedInDependents)
	require.True(t, baseline[1].IncludedInDependents)
	require.True(t, baseline[2].IncludedInDependents)

	affected := newAffected()
	processIncludedInDependencies(&affected)
	filterAffectedDependents(&affected, AffectedFilter{Labels: map[string]string{"ci": "auto"}})

	assert.Equal(t, []string{"dev-web"}, dependentSlugs(affected[0].Dependents))
	assert.False(t, affected[0].IncludedInDependents)
	assert.False(t, affected[1].IncludedInDependents, "iam was pruned from vpc's dependents, so it is no longer included")
	assert.True(t, affected[2].IncludedInDependents, "web is still a dependent of vpc")
}

// TestAttachDependentMetadata verifies dependent metadata is looked up from the resolved stacks by stack and
// component, and that a missing component or a missing metadata section leaves it nil.
func TestAttachDependentMetadata(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{"components": map[string]any{
			"terraform": map[string]any{
				"iam":    map[string]any{"metadata": map[string]any{"labels": map[string]any{"ci": "manual"}}},
				"nometa": map[string]any{"vars": map[string]any{}},
			},
			"helmfile": map[string]any{
				"ingress": map[string]any{"metadata": map[string]any{"tags": []any{"edge"}}},
			},
		}},
	}
	dependents := []schema.Dependent{
		{Component: "iam", Stack: "dev"},
		{Component: "ingress", Stack: "dev"},
		{Component: "nometa", Stack: "dev"},
		{Component: "ghost", Stack: "dev"},
		{Component: "iam", Stack: "missing-stack"},
	}

	attachDependentMetadata(dependents, stacks)

	assert.Equal(t, map[string]any{"labels": map[string]any{"ci": "manual"}}, dependents[0].Metadata)
	assert.Equal(t, map[string]any{"tags": []any{"edge"}}, dependents[1].Metadata)
	assert.Nil(t, dependents[2].Metadata)
	assert.Nil(t, dependents[3].Metadata)
	assert.Nil(t, dependents[4].Metadata)
}

// TestExecute_IncludeDependents_SelectorsPruneDependents is the end-to-end check through Execute: with
// --include-dependents and --labels, a manual dependent is dropped from the output; without the selector
// (the `terraform --affected` situation, where Tags/Labels are cleared) the dependents are unchanged.
func TestExecute_IncludeDependents_SelectorsPruneDependents(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{Terraform: schema.Terraform{BasePath: "components/terraform"}},
	}

	d := describeAffectedExec{atmosConfig: atmosConfig}
	d.IsTTYSupportForStdout = func() bool { return false }
	d.executeDescribeAffectedWithTargetRefCheckout = func(
		_ *schema.AtmosConfiguration, _, _, _ string, _, _ bool, _ string, _, _ bool,
		_ []string, _ AffectedFilter, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{{Component: "vpc", Stack: "dev", StackSlug: "dev-vpc"}}, nil, nil, "", nil
	}
	setDependents := func(affected *[]schema.Affected) {
		(*affected)[0].Dependents = []schema.Dependent{
			dependentFixture("iam", "manual"),
			dependentFixture("app", "auto"),
		}
	}
	// Without selectors Execute uses addDependentsToAffected; with selectors it uses the variant that records metadata.
	d.addDependentsToAffected = func(_ *schema.AtmosConfiguration, affected *[]schema.Affected, _, _, _ bool, _ []string, _ string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) error {
		setDependents(affected)
		return nil
	}
	d.addDependentsToAffectedWithFilter = func(_ *schema.AtmosConfiguration, affected *[]schema.Affected, _ *dependentsOptions) error {
		setDependents(affected)
		return nil
	}

	var captured []schema.Affected
	d.printOrWriteToFile = func(_ *schema.AtmosConfiguration, _ string, _ string, v any) error {
		captured, _ = v.([]schema.Affected)
		return nil
	}

	run := func(t *testing.T, args DescribeAffectedCmdArgs) []schema.Affected {
		t.Helper()
		captured = nil
		args.Format = "json"
		args.IncludeDependents = true
		args.CLIConfig = atmosConfig
		require.NoError(t, resolveAffectedSelectors(&args))
		require.NoError(t, d.Execute(&args))
		return captured
	}

	t.Run("labels=ci=auto drops the manual dependent", func(t *testing.T) {
		got := run(t, DescribeAffectedCmdArgs{LabelsRaw: "ci=auto"})
		require.Len(t, got, 1)
		assert.Equal(t, []string{"dev-app"}, dependentSlugs(got[0].Dependents))
	})

	t.Run("no selector keeps every dependent", func(t *testing.T) {
		got := run(t, DescribeAffectedCmdArgs{})
		require.Len(t, got, 1)
		assert.Equal(t, []string{"dev-iam", "dev-app"}, dependentSlugs(got[0].Dependents))
	})

	t.Run("cleared selectors (terraform --affected) keep every dependent", func(t *testing.T) {
		args := DescribeAffectedCmdArgs{LabelsRaw: "ci=auto"}
		require.NoError(t, resolveAffectedSelectors(&args))
		args.Tags, args.Labels = nil, nil
		args.Format = "json"
		args.IncludeDependents = true
		args.CLIConfig = atmosConfig
		captured = nil
		require.NoError(t, d.Execute(&args))
		require.Len(t, captured, 1)
		assert.Equal(t, []string{"dev-iam", "dev-app"}, dependentSlugs(captured[0].Dependents))
	})
}
