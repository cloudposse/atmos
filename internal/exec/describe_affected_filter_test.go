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
