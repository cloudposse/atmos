package updater

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/vendoring"
)

func TestUpdateScopeIsStable(t *testing.T) {
	assert.Equal(t, "all", UpdateScope("", nil))
	assert.Equal(t, "group-platform", UpdateScope("platform", nil))
	assert.Equal(t, UpdateScope("", []string{"vpc", "eks"}), UpdateScope("", []string{"eks", "vpc"}))
}

func TestFilterGroupComponentsAndReport(t *testing.T) {
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "terraform/vpc", Status: vendoring.StatusUpdated},
		{Component: "terraform/eks/blue", Status: vendoring.StatusUpdated},
		{Component: "terraform/eks/legacy", Status: vendoring.StatusUpdated},
		{Component: "terraform/rds", Status: vendoring.StatusUpdated},
	}}
	assert.Equal(t, []string{"terraform/eks/blue", "terraform/vpc"}, FilterGroupComponents(report, []string{"terraform/vpc", "terraform/eks/*"}, []string{"terraform/eks/legacy"}))

	filtered := FilterReport(report, []string{"terraform/vpc"})
	require.Len(t, filtered.Results, 1)
	assert.Equal(t, "terraform/vpc", filtered.Results[0].Component)
}

func TestMatchesPatterns(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		patterns []string
		empty    bool
		want     bool
	}{
		{name: "no patterns returns empty default true", value: "vpc", patterns: nil, empty: true, want: true},
		{name: "no patterns returns empty default false", value: "vpc", patterns: nil, empty: false, want: false},
		{name: "matching glob", value: "terraform/eks/blue", patterns: []string{"terraform/eks/*"}, empty: false, want: true},
		{name: "non-matching glob", value: "terraform/rds", patterns: []string{"terraform/eks/*"}, empty: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MatchesPatterns(tt.value, tt.patterns, tt.empty))
		})
	}
}
