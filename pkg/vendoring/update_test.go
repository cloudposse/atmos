package vendoring

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLister returns canned tags keyed by the resolved Git URI.
type fakeLister struct {
	tagsByURI map[string][]string
}

func (f *fakeLister) ListTags(_ context.Context, uri string) ([]string, error) {
	return f.tagsByURI[uri], nil
}

const updateFixture = `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example
spec:
  sources:
    # VPC component.
    - component: "vpc"
      source: "github.com/cloudposse/terraform-aws-vpc"
      version: "0.1.0"  # pinned
      tags: [networking]
    - component: "eks"
      source: "github.com/cloudposse/terraform-aws-eks"
      version: "1.0.0"
      tags: [compute]
    - component: "mock"
      source: "oci://ghcr.io/cloudposse/mock:{{.Version}}"
      version: "{{.Version}}"
`

func writeUpdateFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "vendor.yaml")
	require.NoError(t, os.WriteFile(file, []byte(updateFixture), 0o644))
	return file
}

func newFakeLister() *fakeLister {
	return &fakeLister{tagsByURI: map[string][]string{
		"https://github.com/cloudposse/terraform-aws-vpc.git": {"0.1.0", "0.2.0", "1.0.0"},
		"https://github.com/cloudposse/terraform-aws-eks.git": {"1.0.0"},
	}}
}

func resultFor(report *UpdateReport, component string) *SourceUpdateResult {
	for i := range report.Results {
		if report.Results[i].Component == component {
			return &report.Results[i]
		}
	}
	return nil
}

func TestUpdate_AppliesAndPreservesFormatting(t *testing.T) {
	file := writeUpdateFixture(t)

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Lister: newFakeLister()})
	require.NoError(t, err)

	vpc := resultFor(report, "vpc")
	require.NotNil(t, vpc)
	assert.Equal(t, StatusUpdated, vpc.Status)
	assert.Equal(t, "1.0.0", vpc.LatestVersion)

	eks := resultFor(report, "eks")
	require.NotNil(t, eks)
	assert.Equal(t, StatusUpToDate, eks.Status)

	mock := resultFor(report, "mock")
	require.NotNil(t, mock)
	assert.Equal(t, StatusSkipped, mock.Status)

	// File was edited in place, preserving the comment, and only vpc changed.
	got, err := os.ReadFile(file)
	require.NoError(t, err)
	s := string(got)
	assert.Contains(t, s, `version: "1.0.0"`)
	assert.Contains(t, s, "# pinned", "inline comment preserved")
	assert.Contains(t, s, "# VPC component.", "head comment preserved")
	assert.Contains(t, s, "{{.Version}}", "templated source preserved")
	assert.Equal(t, 1, report.UpdatedCount())
}

func TestUpdate_DryRunDoesNotWrite(t *testing.T) {
	file := writeUpdateFixture(t)
	before, _ := os.ReadFile(file)

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, DryRun: true, Lister: newFakeLister()})
	require.NoError(t, err)
	assert.Equal(t, 1, report.UpdatedCount())

	after, _ := os.ReadFile(file)
	assert.Equal(t, string(before), string(after), "dry-run must not modify the file")
}

func TestUpdate_ComponentFilter(t *testing.T) {
	file := writeUpdateFixture(t)
	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Component: "vpc", Lister: newFakeLister()})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "vpc", report.Results[0].Component)
}

func TestUpdate_TagsFilter(t *testing.T) {
	file := writeUpdateFixture(t)
	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Tags: []string{"compute"}, Lister: newFakeLister()})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "eks", report.Results[0].Component)
}

func TestUpdate_ConstraintBlocksMajor(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "vendor.yaml")
	manifest := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example
spec:
  sources:
    - component: "vpc"
      source: "github.com/cloudposse/terraform-aws-vpc"
      version: "0.1.0"
      constraints:
        version: ">=0.1.0 <1.0.0"
`
	require.NoError(t, os.WriteFile(file, []byte(manifest), 0o644))

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Lister: newFakeLister()})
	require.NoError(t, err)
	vpc := resultFor(report, "vpc")
	require.NotNil(t, vpc)
	// ^0.1.0 allows 0.2.0 but not 1.0.0.
	assert.Equal(t, StatusUpdated, vpc.Status)
	assert.Equal(t, "0.2.0", vpc.LatestVersion)
}
