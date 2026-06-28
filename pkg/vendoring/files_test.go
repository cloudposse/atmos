package vendoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

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
	assert.True(t, filepath.IsAbs(files[1]) || filepath.Base(files[1]) == "terraform.yaml")
}

func TestReadVendorSources_DecodesStringTargets(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "vendor.yaml", importedManifest)

	sources, err := readVendorSources(file)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "vpc", sources[0].Component)
	assert.Equal(t, "2.0.0", sources[0].Version)
	assert.Equal(t, []string{"networking"}, sources[0].Tags)
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
	assert.Len(t, files, 2, "cycle must be de-duplicated, not infinite")
}
