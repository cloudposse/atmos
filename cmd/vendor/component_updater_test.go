package vendor

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/vendoring"
)

func TestComponentUpdaterScopeIsStable(t *testing.T) {
	assert.Equal(t, "all", updateScope("", nil))
	assert.Equal(t, "group-platform", updateScope("platform", nil))
	assert.Equal(t, updateScope("", []string{"vpc", "eks"}), updateScope("", []string{"eks", "vpc"}))
}

func TestComponentUpdaterGroupFiltering(t *testing.T) {
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "terraform/vpc", Status: vendoring.StatusUpdated},
		{Component: "terraform/eks/blue", Status: vendoring.StatusUpdated},
		{Component: "terraform/eks/legacy", Status: vendoring.StatusUpdated},
		{Component: "terraform/rds", Status: vendoring.StatusUpdated},
	}}
	assert.Equal(t, []string{"terraform/eks/blue", "terraform/vpc"}, filterGroupComponents(report, []string{"terraform/vpc", "terraform/eks/*"}, []string{"terraform/eks/legacy"}))
}

func TestComponentUpdaterTemplates(t *testing.T) {
	v := viper.New()
	v.Set("vendor.ci.pull_request.title", "update {{ .scope.name }}")
	v.Set("vendor.ci.pull_request.body", "{{ .updates | markdownTable }}")
	title, body, err := renderPRTemplates(v, "all", &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", CurrentVersion: "1", LatestVersion: "2"}}})
	require.NoError(t, err)
	assert.Equal(t, "update all", title)
	assert.Contains(t, body, "| vpc | 1 | 2 |")
}
