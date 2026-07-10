package vendoring

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// fakeLister returns canned tags keyed by the resolved Git URI.
type fakeLister struct {
	tagsByURI map[string][]string
	err       error
}

func (f *fakeLister) ListTags(_ context.Context, uri string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
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
      targets: ["components/terraform/vpc"]
      tags: [networking]
    - component: "eks"
      source: "github.com/cloudposse/terraform-aws-eks"
      version: "1.0.0"
      targets: ["components/terraform/eks"]
      tags: [compute]
    - component: "mock"
      source: "oci://ghcr.io/cloudposse/mock:{{.Version}}"
      version: "{{.Version}}"
      targets: ["components/terraform/mock"]
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
	before, err := os.ReadFile(file)
	require.NoError(t, err)

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, DryRun: true, Lister: newFakeLister()})
	require.NoError(t, err)
	assert.Equal(t, 1, report.UpdatedCount())

	after, err := os.ReadFile(file)
	require.NoError(t, err)
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

func TestUpdate_TypeFilter(t *testing.T) {
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
      targets: ["components/terraform/vpc"]
    - component: "nginx"
      source: "github.com/cloudposse/terraform-aws-nginx"
      version: "0.1.0"
      targets:
        - path: "components/helmfile/nginx"
`
	require.NoError(t, os.WriteFile(file, []byte(manifest), 0o644))
	lister := &fakeLister{tagsByURI: map[string][]string{
		"https://github.com/cloudposse/terraform-aws-vpc.git":   {"0.1.0", "0.2.0"},
		"https://github.com/cloudposse/terraform-aws-nginx.git": {"0.1.0", "0.3.0"},
	}}

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, DryRun: true, Lister: lister})
	require.NoError(t, err)
	require.Len(t, report.Results, 2)
	assert.NotNil(t, resultFor(report, "vpc"))
	assert.NotNil(t, resultFor(report, "nginx"))

	report, err = Update(nil, &UpdateParams{VendorFiles: []string{file}, Type: "helmfile", DryRun: true, Lister: lister})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "nginx", report.Results[0].Component)
}

func TestUpdate_HardFailureReturnsReportAndError(t *testing.T) {
	file := writeUpdateFixture(t)
	listErr := errors.New("list failed")

	report, err := Update(nil, &UpdateParams{
		VendorFiles: []string{file},
		Component:   "vpc",
		Lister:      &fakeLister{err: listErr},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVendorUpdateFailed)
	assert.ErrorIs(t, err, listErr)
	require.NotNil(t, report)
	require.Len(t, report.Results, 1)
	assert.Equal(t, StatusFailed, report.Results[0].Status)
	assert.Contains(t, report.Results[0].Reason, "list failed")
}

// TestUpdate_DefaultVersionSetterUnchanged is a regression test: a nil VersionSetter must behave
// exactly as before the VersionSetter field was introduced.
func TestUpdate_DefaultVersionSetterUnchanged(t *testing.T) {
	file := writeUpdateFixture(t)

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Component: "vpc", Lister: newFakeLister()})
	require.NoError(t, err)
	vpc := resultFor(report, "vpc")
	require.NotNil(t, vpc)
	assert.Equal(t, StatusUpdated, vpc.Status)

	v, err := ComponentVersionPath(file, "vpc")
	require.NoError(t, err)
	assert.Equal(t, "spec.sources[0].version", v)
}

func writeComponentManifestUpdateFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "component.yaml")
	require.NoError(t, os.WriteFile(file, []byte(componentManifestFixture), 0o644))
	return file
}

func TestUpdateResolved_ComponentManifest(t *testing.T) {
	file := writeComponentManifestUpdateFixture(t)
	cfg, err := ReadComponentManifest(file)
	require.NoError(t, err)
	resolved := &ResolvedSource{
		Source:                ComponentManifestSource(cfg, "vpc", "terraform"),
		File:                  file,
		FromComponentManifest: true,
	}
	lister := &fakeLister{tagsByURI: map[string][]string{
		"https://github.com/cloudposse/terraform-aws-vpc.git": {"1.2.3", "1.5.0"},
	}}

	report, err := UpdateResolved(resolved, &UpdateParams{Lister: lister})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, StatusUpdated, report.Results[0].Status)
	assert.Equal(t, "1.5.0", report.Results[0].LatestVersion)

	got, err := atmosyaml.GetFile(file, ComponentManifestVersionPath)
	require.NoError(t, err)
	assert.Equal(t, "1.5.0", got)

	content, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(content), "{{.Version}}", "template in source uri preserved")
}

func TestUpdateResolved_DryRunDoesNotWrite(t *testing.T) {
	file := writeComponentManifestUpdateFixture(t)
	before, err := os.ReadFile(file)
	require.NoError(t, err)
	cfg, err := ReadComponentManifest(file)
	require.NoError(t, err)
	resolved := &ResolvedSource{
		Source:                ComponentManifestSource(cfg, "vpc", "terraform"),
		File:                  file,
		FromComponentManifest: true,
	}
	lister := &fakeLister{tagsByURI: map[string][]string{
		"https://github.com/cloudposse/terraform-aws-vpc.git": {"1.2.3", "1.5.0"},
	}}

	report, err := UpdateResolved(resolved, &UpdateParams{DryRun: true, Lister: lister})
	require.NoError(t, err)
	assert.Equal(t, StatusUpdated, report.Results[0].Status)

	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "dry-run must not modify the file")
}

func TestUpdateResolved_SkipsTemplatedVersion(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "component.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`apiVersion: atmos/v1
kind: ComponentVendorConfig
spec:
  source:
    uri: "oci://ghcr.io/cloudposse/mock:{{.Version}}"
    version: "{{.Version}}"
`), 0o644))
	cfg, err := ReadComponentManifest(file)
	require.NoError(t, err)
	resolved := &ResolvedSource{
		Source:                ComponentManifestSource(cfg, "mock", "terraform"),
		File:                  file,
		FromComponentManifest: true,
	}

	report, err := UpdateResolved(resolved, &UpdateParams{})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, StatusSkipped, report.Results[0].Status)
}

// TestUpdate_ExtraSources_VendorYamlPrecedence verifies that when a component appears in both
// VendorFiles' sources and ExtraSources (component-manifest sweep), the VendorFiles source wins
// and the ExtraSources entry is not double-processed.
func TestUpdate_ExtraSources_VendorYamlPrecedence(t *testing.T) {
	file := writeUpdateFixture(t) // declares "vpc" pinned at 0.1.0, latest 1.0.0.
	componentFile := writeComponentManifestUpdateFixture(t)
	cfg, err := ReadComponentManifest(componentFile)
	require.NoError(t, err)

	extra := &ResolvedSource{
		Source:                &schema.AtmosVendorSource{Component: "vpc", Source: cfg.Spec.Source.Uri, Version: "9.9.9"},
		File:                  componentFile,
		FromComponentManifest: true,
	}

	report, err := Update(nil, &UpdateParams{
		VendorFiles:  []string{file},
		ExtraSources: []*ResolvedSource{extra},
		Component:    "vpc",
		Lister:       newFakeLister(),
	})
	require.NoError(t, err)
	require.Len(t, report.Results, 1, "the component.yaml duplicate for vpc must be skipped")
	assert.Equal(t, "0.1.0", report.Results[0].CurrentVersion, "vendor.yaml's vpc entry (0.1.0) must be the one processed")
}

// TestUpdate_ExtraSources_ComponentManifestOnly verifies a component only declared via
// ExtraSources (no vendor.yaml at all) is checked/updated using the component-manifest
// VersionSetter.
func TestUpdate_ExtraSources_ComponentManifestOnly(t *testing.T) {
	componentFile := writeComponentManifestUpdateFixture(t)
	cfg, err := ReadComponentManifest(componentFile)
	require.NoError(t, err)
	extra := &ResolvedSource{
		Source:                ComponentManifestSource(cfg, "vpc", "terraform"),
		File:                  componentFile,
		FromComponentManifest: true,
	}
	lister := &fakeLister{tagsByURI: map[string][]string{
		"https://github.com/cloudposse/terraform-aws-vpc.git": {"1.2.3", "1.5.0"},
	}}

	report, err := Update(nil, &UpdateParams{ExtraSources: []*ResolvedSource{extra}, Lister: lister})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, StatusUpdated, report.Results[0].Status)

	got, err := atmosyaml.GetFile(componentFile, ComponentManifestVersionPath)
	require.NoError(t, err)
	assert.Equal(t, "1.5.0", got)
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
      targets: ["components/terraform/vpc"]
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

// progressCall records a single OnProgress invocation for assertion.
type progressCall struct {
	component string
	index     int
	total     int
}

// TestUpdate_OnProgress_ReportsSequentialOrder proves OnProgress fires once per source, in the
// exact sequential order Update checks them, with a stable total reflecting all sources that will
// be checked (post-filter) — driving a live "Checking <component>..." progress indicator.
func TestUpdate_OnProgress_ReportsSequentialOrder(t *testing.T) {
	file := writeUpdateFixture(t) // declares vpc, eks, mock (mock is templated => skipped, but still checked/reported).

	var calls []progressCall
	report, err := Update(nil, &UpdateParams{
		VendorFiles: []string{file},
		Lister:      newFakeLister(),
		OnProgress: func(component string, index, total int) {
			calls = append(calls, progressCall{component: component, index: index, total: total})
		},
	})
	require.NoError(t, err)
	require.Len(t, report.Results, 3)

	require.Len(t, calls, 3)
	assert.Equal(t, progressCall{component: "vpc", index: 1, total: 3}, calls[0])
	assert.Equal(t, progressCall{component: "eks", index: 2, total: 3}, calls[1])
	assert.Equal(t, progressCall{component: "mock", index: 3, total: 3}, calls[2])
}

// TestUpdate_OnProgress_HonorsFilters proves that sources filtered out by Component/Tags/Type do
// not trigger OnProgress and are excluded from the reported total, keeping the progress count in
// sync with what checkAndUpdateSource actually processes.
func TestUpdate_OnProgress_HonorsFilters(t *testing.T) {
	file := writeUpdateFixture(t)

	var calls []progressCall
	report, err := Update(nil, &UpdateParams{
		VendorFiles: []string{file},
		Component:   "vpc",
		Lister:      newFakeLister(),
		OnProgress: func(component string, index, total int) {
			calls = append(calls, progressCall{component: component, index: index, total: total})
		},
	})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)

	require.Len(t, calls, 1)
	assert.Equal(t, progressCall{component: "vpc", index: 1, total: 1}, calls[0])
}

// TestUpdate_OnProgress_ExtraSourcesAfterVendorFiles proves progress across both loops in Update
// (VendorFiles then ExtraSources) shares one continuous index/total, and that an ExtraSources
// entry shadowed by a vendor.yaml source of the same name (which "wins") is not double-reported.
func TestUpdate_OnProgress_ExtraSourcesAfterVendorFiles(t *testing.T) {
	file := writeUpdateFixture(t) // declares vpc, eks, mock.
	componentFile := writeComponentManifestUpdateFixture(t)
	cfg, err := ReadComponentManifest(componentFile)
	require.NoError(t, err)

	shadowed := &ResolvedSource{
		Source:                &schema.AtmosVendorSource{Component: "vpc", Source: cfg.Spec.Source.Uri, Version: "9.9.9"},
		File:                  componentFile,
		FromComponentManifest: true,
	}
	extra := &ResolvedSource{
		Source:                ComponentManifestSource(cfg, "vpc", "terraform"),
		File:                  componentFile,
		FromComponentManifest: true,
	}
	extra.Source.Component = "nginx" // Not declared in vendor.yaml, so it's checked.

	var calls []progressCall
	report, err := Update(nil, &UpdateParams{
		VendorFiles:  []string{file},
		ExtraSources: []*ResolvedSource{shadowed, extra},
		Lister: &fakeLister{tagsByURI: map[string][]string{
			"https://github.com/cloudposse/terraform-aws-vpc.git": {"0.1.0", "0.2.0", "1.0.0"},
			"https://github.com/cloudposse/terraform-aws-eks.git": {"1.0.0"},
		}},
		OnProgress: func(component string, index, total int) {
			calls = append(calls, progressCall{component: component, index: index, total: total})
		},
	})
	require.NoError(t, err)
	require.Len(t, report.Results, 4, "vpc, eks, mock from vendor.yaml, plus nginx from ExtraSources; the shadowed vpc duplicate is skipped")

	require.Len(t, calls, 4)
	assert.Equal(t, progressCall{component: "vpc", index: 1, total: 4}, calls[0])
	assert.Equal(t, progressCall{component: "eks", index: 2, total: 4}, calls[1])
	assert.Equal(t, progressCall{component: "mock", index: 3, total: 4}, calls[2])
	assert.Equal(t, progressCall{component: "nginx", index: 4, total: 4}, calls[3])
}

// TestUpdateResolved_OnProgress proves the single-item --component path reports exactly one
// "Checking <component>... (1/1)" progress call before checking it.
func TestUpdateResolved_OnProgress(t *testing.T) {
	file := writeComponentManifestUpdateFixture(t)
	cfg, err := ReadComponentManifest(file)
	require.NoError(t, err)
	resolved := &ResolvedSource{
		Source:                ComponentManifestSource(cfg, "vpc", "terraform"),
		File:                  file,
		FromComponentManifest: true,
	}
	lister := &fakeLister{tagsByURI: map[string][]string{
		"https://github.com/cloudposse/terraform-aws-vpc.git": {"1.2.3", "1.5.0"},
	}}

	var calls []progressCall
	report, err := UpdateResolved(resolved, &UpdateParams{
		Lister: lister,
		OnProgress: func(component string, index, total int) {
			calls = append(calls, progressCall{component: component, index: index, total: total})
		},
	})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)

	require.Equal(t, []progressCall{{component: "vpc", index: 1, total: 1}}, calls)
}

// fakeArchivedChecker is a test double for ArchivedChecker: it never touches the network, either
// returning err (proving best-effort error handling) or looking up a canned archived flag by
// gitURI (defaulting to false/not-archived for any URI not present in the map).
type fakeArchivedChecker struct {
	archivedByURI map[string]bool
	err           error
}

func (f *fakeArchivedChecker) IsArchived(_ context.Context, gitURI string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.archivedByURI[gitURI], nil
}

// TestUpdate_ArchivedChecker_MarksResultRegardlessOfStatus proves Archived is set independently
// of Status: a component can be both "updated" (or "up to date") AND flagged as having an
// archived upstream repo — these are two orthogonal facts, not mutually exclusive states.
func TestUpdate_ArchivedChecker_MarksResultRegardlessOfStatus(t *testing.T) {
	file := writeUpdateFixture(t) // vpc -> StatusUpdated, eks -> StatusUpToDate, mock -> StatusSkipped.
	checker := &fakeArchivedChecker{archivedByURI: map[string]bool{
		"https://github.com/cloudposse/terraform-aws-vpc.git": true,
		"https://github.com/cloudposse/terraform-aws-eks.git": true,
	}}

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Lister: newFakeLister(), ArchivedChecker: checker})
	require.NoError(t, err)

	vpc := resultFor(report, "vpc")
	require.NotNil(t, vpc)
	assert.Equal(t, StatusUpdated, vpc.Status)
	assert.True(t, vpc.Archived, "archived must be set even though the component was updated")

	eks := resultFor(report, "eks")
	require.NotNil(t, eks)
	assert.Equal(t, StatusUpToDate, eks.Status)
	assert.True(t, eks.Archived, "archived must be set even though the component is up to date")
}

// TestUpdate_ArchivedChecker_ErrorDoesNotAffectRealStatus proves an archived-check failure
// (network hiccup, rate limit, whatever) is swallowed and never turns into a StatusFailed for the
// component, nor otherwise perturbs the real version-check outcome — only Archived stays false.
func TestUpdate_ArchivedChecker_ErrorDoesNotAffectRealStatus(t *testing.T) {
	file := writeUpdateFixture(t)
	checker := &fakeArchivedChecker{err: errors.New("archived check boom")}

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Lister: newFakeLister(), ArchivedChecker: checker})
	require.NoError(t, err, "an archived-check error must never surface as a hard Update error")

	vpc := resultFor(report, "vpc")
	require.NotNil(t, vpc)
	assert.Equal(t, StatusUpdated, vpc.Status, "real status must come through unaffected")
	assert.False(t, vpc.Archived, "archived-check failure must be treated as not-archived, not as a component failure")

	eks := resultFor(report, "eks")
	require.NotNil(t, eks)
	assert.Equal(t, StatusUpToDate, eks.Status)
	assert.False(t, eks.Archived)
}

// TestUpdate_ArchivedChecker_NonGitHubSourceReportsNotArchived proves a source whose checker
// reports "not archived" (the contract non-GitHub sources must follow) comes through with
// Archived=false and its real status unaffected.
func TestUpdate_ArchivedChecker_NonGitHubSourceReportsNotArchived(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "vendor.yaml")
	manifest := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example
spec:
  sources:
    - component: "vpc"
      source: "https://gitlab.com/example/terraform-vpc.git"
      version: "0.1.0"
      targets: ["components/terraform/vpc"]
`
	require.NoError(t, os.WriteFile(file, []byte(manifest), 0o644))
	lister := &fakeLister{tagsByURI: map[string][]string{
		"https://gitlab.com/example/terraform-vpc.git": {"0.1.0", "0.2.0"},
	}}
	checker := &fakeArchivedChecker{archivedByURI: map[string]bool{}} // Never reports archived.

	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Lister: lister, ArchivedChecker: checker})
	require.NoError(t, err)
	vpc := resultFor(report, "vpc")
	require.NotNil(t, vpc)
	assert.Equal(t, StatusUpdated, vpc.Status)
	assert.False(t, vpc.Archived)
}

// TestUpdate_NilArchivedChecker_FallsBackToDefault proves a nil UpdateParams.ArchivedChecker
// falls back to the package-level DefaultArchivedChecker, mirroring how a nil Lister falls back
// to version.DefaultLister. The real DefaultArchivedChecker is swapped out for the duration of
// the test so this stays a fast, network-free unit test.
func TestUpdate_NilArchivedChecker_FallsBackToDefault(t *testing.T) {
	original := DefaultArchivedChecker
	t.Cleanup(func() { DefaultArchivedChecker = original })
	DefaultArchivedChecker = &fakeArchivedChecker{archivedByURI: map[string]bool{
		"https://github.com/cloudposse/terraform-aws-vpc.git": true,
	}}

	file := writeUpdateFixture(t)
	report, err := Update(nil, &UpdateParams{VendorFiles: []string{file}, Component: "vpc", Lister: newFakeLister()})
	require.NoError(t, err)
	vpc := resultFor(report, "vpc")
	require.NotNil(t, vpc)
	assert.True(t, vpc.Archived)
}
