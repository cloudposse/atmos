//nolint:gocritic // Graph fixtures intentionally use value components to exercise serialization copies.
package sbom

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	toolchainlock "github.com/cloudposse/atmos/pkg/toolchain/lockfile"
	vendorlock "github.com/cloudposse/atmos/pkg/vendoring/lockfile"
	atmosversion "github.com/cloudposse/atmos/pkg/version"
	versionmanager "github.com/cloudposse/atmos/pkg/version/manager"
)

func TestBuildAndRenderVendorLockDomain(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}

	vendor := vendorlock.New()
	vendor.Artifacts["vpc"] = vendorlock.Artifact{
		Name:   "vpc",
		Kind:   "source",
		Target: filepath.Join(base, "components", "terraform", "vpc"),
		Source: vendorlock.Source{
			Declared: "oci://ghcr.io/example/vpc:v1",
			Digest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		Files: []vendorlock.File{{Path: "main.tf", Type: "file", Mode: 0o644, SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
	}
	require.NoError(t, vendorlock.Save(config, vendor))

	// An empty Scope normalizes to ScopeTerraform (see TestNormalizeOptionsDefaultsScopeWhenEmpty),
	// so this also exercises BuildWithOptions' default entry point.
	graph, err := BuildWithOptions(config, Options{IncludeFiles: true})
	require.NoError(t, err)
	require.Len(t, graph.Components, 2)
	require.Len(t, graph.Relationships, 1)

	for _, format := range []string{FormatCycloneDXJSON, FormatSPDXJSON} {
		content, renderErr := Render(graph, format)
		require.NoError(t, renderErr)
		require.NotContains(t, string(content), base)
		var document map[string]any
		require.NoError(t, json.Unmarshal(content, &document))
		if format == FormatCycloneDXJSON {
			require.Equal(t, "CycloneDX", document["bomFormat"])
			require.Len(t, document["dependencies"], 1)
		} else {
			require.Equal(t, "SPDX-2.3", document["spdxVersion"])
			require.Len(t, document["relationships"], 1)
		}
	}
}

func TestBuildDependenciesScopeIncludesToolchainAndVersionTrackEvidence(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Toolchain.LockFile = "toolchain.lock.yaml"
	config.Version.LockFile = "versions.lock.yaml"

	vendor := vendorlock.New()
	vendor.Artifacts["vpc"] = vendorlock.Artifact{
		Name:   "vpc",
		Kind:   "source",
		Target: filepath.Join(base, "components", "terraform", "vpc"),
		Source: vendorlock.Source{
			Declared: "oci://ghcr.io/example/vpc:v1",
			Digest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
	require.NoError(t, vendorlock.Save(config, vendor))

	tools := toolchainlock.New()
	tools.Tools["terraform"] = &toolchainlock.Tool{
		Version: "1.10.0",
		Platforms: map[string]*toolchainlock.PlatformEntry{
			"darwin_arm64": {URL: "https://releases.hashicorp.com/terraform.zip", Checksum: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
		},
	}
	require.NoError(t, toolchainlock.Save(filepath.Join(base, config.Toolchain.LockFile), tools))

	versions := &versionmanager.LockFile{Version: 1, Tracks: map[string]map[string]versionmanager.LockEntry{
		"stable": {"aws": {Version: "5.0.0", Ecosystem: "terraform", Datasource: "github-releases", Provider: "hashicorp/aws", Package: "aws", Digest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}},
	}}
	require.NoError(t, versionmanager.SaveLock(config, versions))

	graph, err := BuildWithOptions(config, Options{Scope: ScopeDependencies})
	require.NoError(t, err)
	ids := componentIDs(graph)
	require.Contains(t, ids, "vendor:vpc")
	require.Contains(t, ids, "toolchain:terraform:darwin_arm64")
	require.Contains(t, ids, "version:stable:aws")
}

func TestBuildTerraformScopeWithImmutableModuleEvidence(t *testing.T) {
	base := t.TempDir()
	componentDir := filepath.Join(base, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(filepath.Join(componentDir, "modules", "network"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, ".terraform.lock.hcl"), []byte(`provider "registry.terraform.io/hashicorp/aws" {
  version = "5.95.0"
  hashes = ["zh:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "modules", "network", "main.tf"), []byte("terraform {}"), 0o644))

	previous := runTerraformModules
	runTerraformModules = func(_ context.Context, _ string, _ string) ([]byte, error) {
		return []byte(`{"format_version":"1.0","modules":[{"key":"child","source":"./modules/network"}]}`), nil
	}
	t.Cleanup(func() { runTerraformModules = previous })

	config := &schema.AtmosConfiguration{BasePath: base}
	config.Components.Terraform.BasePath = filepath.Join("components", "terraform")
	vendor := vendorlock.New()
	vendor.Artifacts["vpc"] = vendorlock.Artifact{Name: "vpc", Kind: "git", Target: componentDir, Source: vendorlock.Source{Declared: "https://token@example.com/org/vpc.git?signature=secret", Resolved: "https://example.com/org/vpc.git", Digest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}
	require.NoError(t, vendorlock.Save(config, vendor))
	graph, err := BuildWithOptions(config, Options{Scope: ScopeTerraform, Mode: ModeNTIA, Subject: Subject{Name: "infra-live", Version: "2026.07.17", Supplier: "Example"}})
	require.NoError(t, err)
	require.Contains(t, componentIDs(graph), "terraform-provider:registry.terraform.io/hashicorp/aws@5.95.0")
	require.Contains(t, componentIDs(graph), "terraform-module:terraform:vpc:child")
	require.Contains(t, graph.Relationships, Relationship{From: "terraform:vpc", To: "vendor:vpc", Type: "depends_on"})
	content, renderErr := Render(graph, FormatCycloneDXJSON)
	require.NoError(t, renderErr)
	require.NotContains(t, string(content), base)
	require.Contains(t, string(content), "https://example.com/org/vpc.git")
	for _, coverage := range graph.Coverage {
		require.Equal(t, "complete", coverage.Status)
	}
}

func TestTerraformProviderWithoutArchiveSHAIsIncompleteButRetainsLockHash(t *testing.T) {
	base := t.TempDir()
	componentDir := filepath.Join(base, "components", "terraform", "ses")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, ".terraform.lock.hcl"), []byte(`provider "registry.opentofu.org/hashicorp/aws" {
  version = "4.67.0"
  hashes = ["h1:base64encodedchecksum"]
}`), 0o644))
	previous := runTerraformModules
	runTerraformModules = func(_ context.Context, _ string, _ string) ([]byte, error) {
		return []byte(`{"format_version":"1.0"}`), nil
	}
	t.Cleanup(func() { runTerraformModules = previous })
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Components.Terraform.BasePath = filepath.Join("components", "terraform")
	graph, err := BuildWithOptions(config, Options{Scope: ScopeTerraform, Mode: ModeProvenance})
	require.NoError(t, err)
	content, renderErr := Render(graph, FormatCycloneDXJSON)
	require.NoError(t, renderErr)
	require.Contains(t, string(content), "h1:base64encodedchecksum")
	for _, coverage := range graph.Coverage {
		if coverage.Adapter == "terraform-providers" {
			require.Equal(t, "incomplete", coverage.Status)
		}
	}
}

func TestNTIARequiresSubjectAndCompleteCoverage(t *testing.T) {
	_, err := BuildWithOptions(&schema.AtmosConfiguration{BasePath: t.TempDir()}, Options{Scope: ScopeTerraform, Mode: ModeNTIA})
	require.ErrorContains(t, err, "subject name, version, and supplier")
}

func componentIDs(graph *Graph) []string {
	ids := make([]string, 0, len(graph.Components))
	for _, component := range graph.Components {
		ids = append(ids, component.ID)
	}
	return ids
}

func TestRenderRejectsUnknownFormat(t *testing.T) {
	t.Parallel()
	_, err := Render(&Graph{}, "xml")
	require.Error(t, err)
}

func TestRenderDoesNotExposeLocalFilesystemPaths(t *testing.T) {
	graph := &Graph{Components: []Component{{
		ID:      "/private/workspace/components/terraform/root",
		Name:    "/private/workspace/components/terraform/root",
		Version: "/private/workspace/version",
		Type:    "application",
		Source:  "/private/workspace/components/terraform/root",
		Properties: map[string]string{
			"atmos:component-path": "/private/workspace/components/terraform/root",
			"atmos:lock-file":      "C:\\Users\\operator\\workspace\\.terraform.lock.hcl",
		},
	}}, Relationships: []Relationship{{From: "/private/workspace/components/terraform/root", To: "C:\\Users\\operator\\workspace\\module", Type: "depends_on"}}}
	for _, format := range []string{FormatCycloneDXJSON, FormatSPDXJSON} {
		content, err := Render(graph, format)
		require.NoError(t, err)
		require.NotContains(t, string(content), "/private/workspace")
		require.NotContains(t, string(content), `C:\\Users\\operator`)
	}
}

func TestRepositoryPathMakesRelativeBaseAbsolute(t *testing.T) {
	base := t.TempDir()
	t.Chdir(base)
	config := &schema.AtmosConfiguration{BasePath: "."}
	require.Equal(t, "components/terraform/vpc/.terraform.lock.hcl", repositoryPath(config, filepath.Join(base, "components", "terraform", "vpc", ".terraform.lock.hcl")))
}

// --- normalizeOptions / BuildWithOptions error propagation --------------------

func TestNormalizeOptionsDefaultsModeWhenEmpty(t *testing.T) {
	t.Parallel()
	options := Options{}
	require.NoError(t, normalizeOptions(&options))
	require.Equal(t, ModeProvenance, options.Mode)
}

func TestNormalizeOptionsRejectsUnsupportedMode(t *testing.T) {
	t.Parallel()
	err := normalizeOptions(&Options{Mode: "bogus"})
	require.ErrorIs(t, err, errUnsupportedMode)
}

func TestNormalizeOptionsRejectsUnsupportedScope(t *testing.T) {
	t.Parallel()
	err := normalizeOptions(&Options{Scope: "bogus"})
	require.ErrorIs(t, err, errUnsupportedScope)
}

func TestNormalizeOptionsDefaultsScopeWhenEmpty(t *testing.T) {
	t.Parallel()
	options := Options{}
	require.NoError(t, normalizeOptions(&options))
	require.Equal(t, ScopeTerraform, options.Scope)
}

func TestNormalizeOptionsAcceptsDependenciesScope(t *testing.T) {
	t.Parallel()
	options := Options{Scope: ScopeDependencies}
	require.NoError(t, normalizeOptions(&options))
	require.Equal(t, ScopeDependencies, options.Scope)
}

func TestBuildWithOptionsPropagatesNormalizeOptionsError(t *testing.T) {
	t.Parallel()
	_, err := BuildWithOptions(&schema.AtmosConfiguration{BasePath: t.TempDir()}, Options{Mode: "bogus"})
	require.ErrorIs(t, err, errUnsupportedMode)
}

func TestBuildWithOptionsPropagatesVendorLockParseError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	require.NoError(t, os.WriteFile(vendorlock.Path(config), []byte("not: [valid yaml"), 0o644))

	_, err := BuildWithOptions(config, Options{Scope: ScopeTerraform})
	require.Error(t, err)
	require.ErrorContains(t, err, "parse vendor lock")
}

func TestBuildWithOptionsPropagatesScopeArtifactsError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Toolchain.LockFile = "toolchain.lock.yaml"
	require.NoError(t, os.WriteFile(filepath.Join(base, config.Toolchain.LockFile), []byte("not: [valid yaml"), 0o644))

	_, err := BuildWithOptions(config, Options{Scope: ScopeDependencies})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to parse lock file")
}

func TestAppendVersionsErrorPropagatesThroughScopeArtifacts(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Version.LockFile = "versions.lock.yaml"
	require.NoError(t, os.WriteFile(filepath.Join(base, config.Version.LockFile), []byte("not: [valid yaml"), 0o644))

	_, err := BuildWithOptions(config, Options{Scope: ScopeDependencies})
	require.Error(t, err)
}

// --- appendVendor: artifact naming fallback ------------------------------------

func TestAppendVendorFallsBackToArtifactIDWhenComponentNameEmpty(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	lock := vendorlock.New()
	lock.Artifacts["unnamed"] = vendorlock.Artifact{
		Kind:   "source",
		Target: filepath.Join(base, "components", "terraform", "unnamed"),
		Source: vendorlock.Source{Declared: "oci://ghcr.io/example/unnamed:v1"},
	}
	require.NoError(t, vendorlock.Save(config, lock))

	graph := &Graph{}
	require.NoError(t, appendVendor(graph, config, false))
	require.Len(t, graph.Components, 1)
	require.Equal(t, "unnamed", graph.Components[0].Name)
}

// --- appendJITSources -----------------------------------------------------------

func writeJITReceipt(t *testing.T, componentDir string, metadata workdir.WorkdirMetadata) {
	t.Helper()
	atmosDir := filepath.Join(componentDir, workdir.AtmosDir)
	require.NoError(t, os.MkdirAll(atmosDir, 0o755))
	content, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, workdir.MetadataFile), content, 0o644))
}

func TestAppendJITSourcesReturnsCompleteWithNoSearchPath(t *testing.T) {
	t.Parallel()
	complete, detail, err := appendJITSources(&Graph{}, &schema.AtmosConfiguration{})
	require.NoError(t, err)
	require.True(t, complete)
	require.Contains(t, detail, "no JIT workdir search path")
}

func TestAppendJITSourcesParsesCompleteReceipt(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	componentDir := filepath.Join(base, "components", "terraform", "vpc")
	writeJITReceipt(t, componentDir, workdir.WorkdirMetadata{
		Component:      "vpc",
		Stack:          "core-usw2",
		SourceIdentity: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		SourceResolved: "https://example.com/org/vpc.git",
		CreatedAt:      time.Now(),
	})

	graph := &Graph{}
	complete, detail, err := appendJITSources(graph, &schema.AtmosConfiguration{BasePath: base})
	require.NoError(t, err)
	require.True(t, complete)
	require.Contains(t, detail, "vendor lock and JIT receipts parsed")
	require.Len(t, graph.Components, 1)
	require.Equal(t, "vpc", graph.Components[0].Name)
	require.Equal(t, "https://example.com/org/vpc.git", graph.Components[0].Source)
}

func TestAppendJITSourcesIncompleteWhenIdentityMissing(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	completeDir := filepath.Join(base, "components", "terraform", "complete")
	writeJITReceipt(t, completeDir, workdir.WorkdirMetadata{Component: "complete", SourceIdentity: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"})
	incompleteDir := filepath.Join(base, "components", "terraform", "incomplete")
	writeJITReceipt(t, incompleteDir, workdir.WorkdirMetadata{Component: "incomplete", SourceURI: "https://example.com/org/incomplete.git"})

	graph := &Graph{}
	complete, detail, err := appendJITSources(graph, &schema.AtmosConfiguration{BasePath: base})
	require.NoError(t, err)
	require.False(t, complete)
	require.Contains(t, detail, "lack immutable source identity")
	require.Len(t, graph.Components, 2)
	for _, component := range graph.Components {
		if component.Name == "incomplete" {
			require.Equal(t, "NOASSERTION", component.Version)
			require.Equal(t, "https://example.com/org/incomplete.git", component.Source)
		}
	}
}

func TestAppendJITSourcesReturnsErrorForCorruptReceipt(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	atmosDir := filepath.Join(base, "components", "terraform", "vpc", workdir.AtmosDir)
	require.NoError(t, os.MkdirAll(atmosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, workdir.MetadataFile), []byte("{not valid json"), 0o644))

	_, _, err := appendJITSources(&Graph{}, &schema.AtmosConfiguration{BasePath: base})
	require.Error(t, err)
	require.ErrorContains(t, err, "parse JIT workdir receipt")
}

func TestAppendVendorPropagatesJITSourcesError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	atmosDir := filepath.Join(base, "components", "terraform", "vpc", workdir.AtmosDir)
	require.NoError(t, os.MkdirAll(atmosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, workdir.MetadataFile), []byte("{not valid json"), 0o644))

	err := appendVendor(&Graph{}, &schema.AtmosConfiguration{BasePath: base}, false)
	require.Error(t, err)
}

// --- loadJITReceipt / firstNonEmpty ---------------------------------------------

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "all empty", values: []string{"", ""}, want: ""},
		{name: "no values", values: nil, want: ""},
		{name: "first wins", values: []string{"a", "b"}, want: "a"},
		{name: "skips leading empties", values: []string{"", "", "c"}, want: "c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, firstNonEmpty(tt.values...))
		})
	}
}

// --- relativeToBase / repositoryPath --------------------------------------------

func TestRelativeToBaseFallsBackToPathWhenRelUnavailable(t *testing.T) {
	t.Parallel()
	// filepath.Rel cannot make an absolute path relative to a relative base;
	// relativeToBase must return the original path unchanged in that case.
	require.Equal(t, "/absolute/path", relativeToBase("relative-base", "/absolute/path"))
}

func TestRepositoryPathReturnsEmptyForEmptyInput(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", repositoryPath(&schema.AtmosConfiguration{}, ""))
}

func TestRepositoryPathFallsBackToBaseNameWithoutConfigBasePath(t *testing.T) {
	t.Parallel()
	absPath := filepath.Join(t.TempDir(), "vendor.lock.yaml")
	require.Equal(t, filepath.Base(absPath), repositoryPath(nil, absPath))
	require.Equal(t, filepath.Base(absPath), repositoryPath(&schema.AtmosConfiguration{}, absPath))
}

// --- appendToolchain / appendVersions: nil config and absent lock files --------

func TestAppendToolchainAndAppendVersionsNoOpOnNilConfig(t *testing.T) {
	t.Parallel()
	graph := &Graph{}
	require.NoError(t, appendToolchain(graph, nil))
	require.NoError(t, appendVersions(graph, nil))
	require.Empty(t, graph.Components)
}

func TestAppendToolchainAndAppendVersionsNoOpWhenLockFilesAbsent(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}

	graph := &Graph{}
	require.NoError(t, appendToolchain(graph, config))
	require.NoError(t, appendVersions(graph, config))
	require.Empty(t, graph.Components)
}

func TestAppendToolchainPropagatesCorruptLockError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Toolchain.LockFile = "toolchain.lock.yaml"
	require.NoError(t, os.WriteFile(filepath.Join(base, config.Toolchain.LockFile), []byte("not: [valid yaml"), 0o644))

	err := appendToolchain(&Graph{}, config)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to parse lock file")
}

func TestAppendVersionsPropagatesCorruptLockError(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Version.LockFile = "versions.lock.yaml"
	require.NoError(t, os.WriteFile(filepath.Join(base, config.Version.LockFile), []byte("not: [valid yaml"), 0o644))

	err := appendVersions(&Graph{}, config)
	require.Error(t, err)
}

// --- validateNTIA ----------------------------------------------------------------

func TestValidateNTIARequiresTerraformScope(t *testing.T) {
	t.Parallel()
	graph := &Graph{Subject: Subject{Name: "n", Version: "v", Supplier: "s"}}
	err := validateNTIA(graph, "not-terraform")
	require.ErrorIs(t, err, errNTIAScopeRequired)
}

func TestValidateNTIARequiresCompleteCoverage(t *testing.T) {
	t.Parallel()
	graph := &Graph{
		Subject:  Subject{Name: "n", Version: "v", Supplier: "s"},
		Coverage: []Coverage{{Adapter: "atmos-sources", Status: "incomplete", Detail: "some detail"}},
	}
	err := validateNTIA(graph, ScopeTerraform)
	require.ErrorIs(t, err, errNTIACoverageIncomplete)
	require.ErrorContains(t, err, "atmos-sources adapter is incomplete")
}

func TestRenderIdentifiesAtmosGeneratorVersion(t *testing.T) {
	original := atmosversion.Version
	atmosversion.Version = "1.2.3"
	t.Cleanup(func() { atmosversion.Version = original })
	graph := &Graph{}
	cycloneDX, err := Render(graph, FormatCycloneDXJSON)
	require.NoError(t, err)
	require.Contains(t, string(cycloneDX), `"version": "1.2.3"`)
	require.Contains(t, string(cycloneDX), "Cloud Posse, LLC")
	require.Contains(t, string(cycloneDX), "pkg:golang/github.com/cloudposse/atmos@1.2.3")
	require.Contains(t, string(cycloneDX), "https://github.com/cloudposse/atmos")
	spdx, err := Render(graph, FormatSPDXJSON)
	require.NoError(t, err)
	require.Contains(t, string(spdx), "Tool: atmos-1.2.3")
	require.Contains(t, string(spdx), "generator repository: https://github.com/cloudposse/atmos")
}

func TestRenderTreatsNilGraphAsEmpty(t *testing.T) {
	t.Parallel()
	content, err := Render(nil, FormatCycloneDXJSON)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(content, &document))
	require.Equal(t, "CycloneDX", document["bomFormat"])
	require.Empty(t, document["components"])
}

func TestRenderIncludesPURLSupplierAndSubjectMetadata(t *testing.T) {
	t.Parallel()
	graph := &Graph{
		Subject: Subject{Name: "infra-live", Version: "2026.07.18", Supplier: "Example Corp"},
		Components: []Component{{
			ID:       "terraform-provider:registry.terraform.io/hashicorp/aws@5.95.0",
			Name:     "registry.terraform.io/hashicorp/aws",
			Version:  "5.95.0",
			Type:     "library",
			PURL:     "pkg:golang/registry.terraform.io/hashicorp/aws@5.95.0",
			Supplier: "HashiCorp",
			Source:   "https://registry.terraform.io/hashicorp/aws",
		}},
	}

	cyclonedxContent, err := Render(graph, FormatCycloneDXJSON)
	require.NoError(t, err)
	require.Contains(t, string(cyclonedxContent), `"purl": "pkg:golang/registry.terraform.io/hashicorp/aws@5.95.0"`)
	require.Contains(t, string(cyclonedxContent), `"name": "HashiCorp"`)

	spdxContent, err := Render(graph, FormatSPDXJSON)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(spdxContent, &document))
	require.Equal(t, "infra-live", document["name"])
	packages, ok := document["packages"].([]any)
	require.True(t, ok)
	require.Len(t, packages, 1)
	pkg, ok := packages[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "HashiCorp", pkg["supplier"])
	externalRefs, ok := pkg["externalRefs"].([]any)
	require.True(t, ok)
	require.Len(t, externalRefs, 1)
	relationships, ok := document["relationships"].([]any)
	require.True(t, ok)
	foundDescribes := false
	for _, entry := range relationships {
		relationship, ok := entry.(map[string]any)
		require.True(t, ok)
		if relationship["relationshipType"] == "DESCRIBES" {
			foundDescribes = true
			require.Equal(t, "SPDXRef-DOCUMENT", relationship["spdxElementId"])
		}
	}
	require.True(t, foundDescribes, "expected an SPDX DESCRIBES relationship for the document subject")
}
