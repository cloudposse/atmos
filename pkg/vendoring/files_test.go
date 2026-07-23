package vendoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

const versionPathFixture = `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example
spec:
  sources:
    # VPC component.
    - component: "vpc"
      source: "oci://ghcr.io/cloudposse/atmos/mock:{{.Version}}"
      version: "v0"  # pinned version
      targets:
        - "components/terraform/vpc"
    - component: "eks"
      source: "github.com/cloudposse/terraform-aws-eks?ref={{.Version}}"
      version: "1.2.3"
`

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

const mainWithImports = `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: root
spec:
  imports:
    - vendor/terraform.yaml
  sources:
    - component: "root-comp"
      source: "github.com/example/root"
      version: "1.0.0"
`

const importedManifest = `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: tf
spec:
  sources:
    # targets uses the string form, which must decode without error.
    - component: "vpc"
      source: "github.com/cloudposse/terraform-aws-vpc"
      version: "2.0.0"
      targets: ["components/terraform/vpc"]
      tags: [networking]
`

func TestCollectManifestFiles_FollowsImports(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))
	main := writeFile(t, dir, "vendor.yaml", mainWithImports)
	writeFile(t, filepath.Join(dir, "vendor"), "terraform.yaml", importedManifest)

	files, err := CollectManifestFiles(main)
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, main, files[0])
	assert.Equal(t, filepath.Join(dir, "vendor", "terraform.yaml"), files[1])
}

func TestCollectManifestFiles_PropagatesReadAndImportErrors(t *testing.T) {
	_, err := CollectManifestFiles(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadVendorFile)

	dir := t.TempDir()
	file := writeFile(t, dir, "vendor.yaml", `spec:
  imports:
    - missing.yaml
`)
	_, err = CollectManifestFiles(file)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadVendorFile)
}

func TestCollectManifestFiles_PropagatesParseErrors(t *testing.T) {
	file := writeFile(t, t.TempDir(), "vendor.yaml", "spec: [")

	_, err := CollectManifestFiles(file)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrParseVendorFile)
}

func TestReadVendorSources_DecodesStringTargets(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "vendor.yaml", importedManifest)

	sources, err := readVendorSources(file)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "vpc", sources[0].Component)
	assert.Equal(t, "2.0.0", sources[0].Version)
	require.Len(t, sources[0].Targets, 1)
	assert.Equal(t, "components/terraform/vpc", sources[0].Targets[0].Path)
	assert.Equal(t, []string{"networking"}, sources[0].Tags)
}

func TestReadVendorSources_PropagatesDecodeError(t *testing.T) {
	_, err := readVendorSources(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadVendorFile)
}

func TestFindSource(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))
	main := writeFile(t, dir, "vendor.yaml", mainWithImports)
	writeFile(t, filepath.Join(dir, "vendor"), "terraform.yaml", importedManifest)

	files, err := CollectManifestFiles(main)
	require.NoError(t, err)

	src, file, err := FindSource(files, "vpc")
	require.NoError(t, err)
	assert.Equal(t, "github.com/cloudposse/terraform-aws-vpc", src.Source)
	assert.Equal(t, "terraform.yaml", filepath.Base(file), "vpc is declared in the imported file")

	_, _, err = FindSource(files, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVendorSourceNotFound)
}

func TestComponentVersionPath(t *testing.T) {
	file := writeFile(t, t.TempDir(), "vendor.yaml", versionPathFixture)

	path, err := ComponentVersionPath(file, "vpc")
	require.NoError(t, err)
	assert.Equal(t, "spec.sources[0].version", path)

	path, err = ComponentVersionPath(file, "eks")
	require.NoError(t, err)
	assert.Equal(t, "spec.sources[1].version", path)
}

func TestComponentVersionPath_NotFound(t *testing.T) {
	file := writeFile(t, t.TempDir(), "vendor.yaml", versionPathFixture)

	_, err := ComponentVersionPath(file, "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

func TestComponentVersionPath_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	_, err := ComponentVersionPath(missing, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrReadFile)
}

func TestSetComponentVersion_PreservesFormatting(t *testing.T) {
	file := writeFile(t, t.TempDir(), "vendor.yaml", versionPathFixture)

	require.NoError(t, SetComponentVersion(file, "vpc", "v1.5.0"))

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	s := string(got)

	assert.Contains(t, s, `version: "v1.5.0"`, "vpc version updated")
	assert.Contains(t, s, "# VPC component.", "comment preserved")
	assert.Contains(t, s, "# pinned version", "inline comment preserved")
	assert.Contains(t, s, "{{.Version}}", "template in source preserved")
	// eks untouched.
	v, err := atmosyaml.GetFile(file, "spec.sources[1].version")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", v)
}

func TestSetComponentVersion_NotFound(t *testing.T) {
	file := writeFile(t, t.TempDir(), "vendor.yaml", versionPathFixture)

	err := SetComponentVersion(file, "nope", "v9")
	require.Error(t, err)
	// A genuinely missing component is reported as path-not-found.
	assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

// TestSetComponentVersion_SurfacesRealError verifies that a non "component
// missing" failure (here, an unreadable/nonexistent manifest file) is returned
// as-is rather than being rewritten as "component not found".
func TestSetComponentVersion_SurfacesRealError(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	err := SetComponentVersion(missingFile, "vpc", "v9")
	require.Error(t, err)
	// The real cause (read failure) must surface, not a bogus "component not found".
	assert.ErrorIs(t, err, atmosyaml.ErrReadFile)
	assert.NotErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

// TestSetComponentVersion_InvalidYAML verifies that invalid YAML surfaces a
// parse error, not a "component not found" message.
func TestSetComponentVersion_InvalidYAML(t *testing.T) {
	file := writeFile(t, t.TempDir(), "vendor.yaml", "spec: {sources: [ unclosed\n")

	err := SetComponentVersion(file, "vpc", "v9")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrParseVendorFile)
	assert.NotErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound, "invalid YAML must not be reported as component-not-found")
}

func TestCollectManifestFiles_CyclicImportsTerminate(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.yaml", `spec:
  imports:
    - b.yaml
`)
	writeFile(t, dir, "b.yaml", `spec:
  imports:
    - a.yaml
`)
	files, err := CollectManifestFiles(a)
	require.NoError(t, err)
	require.Len(t, files, 2, "cycle must be de-duplicated, not infinite")
	assert.Equal(t, a, files[0])
	assert.Equal(t, filepath.Join(dir, "b.yaml"), files[1])
}
