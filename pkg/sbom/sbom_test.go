package sbom

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	toolchainlock "github.com/cloudposse/atmos/pkg/toolchain/lockfile"
	vendorlock "github.com/cloudposse/atmos/pkg/vendoring/lockfile"
	atmosversion "github.com/cloudposse/atmos/pkg/version"
	versionmanager "github.com/cloudposse/atmos/pkg/version/manager"
)

func TestBuildAndRenderAllLockDomains(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Toolchain.LockFile = "toolchain.lock.yaml"
	config.Version.LockFile = "versions.lock.yaml"

	vendor := vendorlock.New()
	vendor.Artifacts["vpc"] = vendorlock.Artifact{
		Component: "vpc",
		Kind:      "source",
		Target:    filepath.Join(base, "components", "terraform", "vpc"),
		Source: vendorlock.Source{
			Declared: "oci://ghcr.io/example/vpc:v1",
			Digest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		Files: []vendorlock.File{{Path: "main.tf", Type: "file", Mode: 0o644, SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
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

	graph, err := Build(config, true)
	require.NoError(t, err)
	require.Len(t, graph.Components, 4)
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
	vendor.Artifacts["vpc"] = vendorlock.Artifact{Component: "vpc", Kind: "git", Target: componentDir, Source: vendorlock.Source{Declared: "https://token@example.com/org/vpc.git?signature=secret", Resolved: "https://example.com/org/vpc.git", Digest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}
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
