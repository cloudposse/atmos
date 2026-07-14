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

const componentManifestFixture = `apiVersion: atmos/v1
kind: ComponentVendorConfig
metadata:
  name: vpc
  description: VPC component.
spec:
  source:
    type: git
    uri: "github.com/cloudposse/terraform-aws-vpc?ref={{.Version}}"
    version: "1.2.3"  # pinned version
    # component-level constraints.
    constraints:
      version: ">=1.0.0 <2.0.0"
`

func TestFindComponentManifestFile_PrefersYamlThenYml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "component.yaml", componentManifestFixture)

	got, err := FindComponentManifestFile(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "component.yaml"), got)
}

func TestFindComponentManifestFile_FallsBackToYml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "component.yml", componentManifestFixture)

	got, err := FindComponentManifestFile(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "component.yml"), got)
}

func TestFindComponentManifestFile_NotFound(t *testing.T) {
	_, err := FindComponentManifestFile(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentManifestNotFound)
}

func TestReadComponentManifest_Decodes(t *testing.T) {
	file := writeFile(t, t.TempDir(), "component.yaml", componentManifestFixture)

	cfg, err := ReadComponentManifest(file)
	require.NoError(t, err)
	assert.Equal(t, "ComponentVendorConfig", cfg.Kind)
	assert.Equal(t, "github.com/cloudposse/terraform-aws-vpc?ref={{.Version}}", cfg.Spec.Source.Uri)
	assert.Equal(t, "1.2.3", cfg.Spec.Source.Version)
	require.NotNil(t, cfg.Spec.Source.Constraints)
	assert.Equal(t, ">=1.0.0 <2.0.0", cfg.Spec.Source.Constraints.Version)
}

func TestReadComponentManifest_RejectsWrongKind(t *testing.T) {
	file := writeFile(t, t.TempDir(), "component.yaml", `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  source:
    uri: "github.com/cloudposse/terraform-aws-vpc"
`)

	_, err := ReadComponentManifest(file)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidComponentManifestKind)
}

func TestReadComponentManifest_PropagatesReadError(t *testing.T) {
	_, err := ReadComponentManifest(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadVendorFile)
}

func TestReadComponentManifest_PropagatesParseError(t *testing.T) {
	file := writeFile(t, t.TempDir(), "component.yaml", "spec: [")

	_, err := ReadComponentManifest(file)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrParseVendorFile)
}

func TestComponentManifestSource_ConvertsFieldsAndSynthesizesTargets(t *testing.T) {
	file := writeFile(t, t.TempDir(), "component.yaml", componentManifestFixture)
	cfg, err := ReadComponentManifest(file)
	require.NoError(t, err)

	src := ComponentManifestSource(cfg, "vpc", "terraform")
	assert.Equal(t, "vpc", src.Component)
	assert.Equal(t, "github.com/cloudposse/terraform-aws-vpc?ref={{.Version}}", src.Source)
	assert.Equal(t, "1.2.3", src.Version)
	require.Len(t, src.Targets, 1)
	assert.Equal(t, filepath.Join("components", "terraform", "vpc"), src.Targets[0].Path)
	require.NotNil(t, src.Constraints)
	assert.Equal(t, ">=1.0.0 <2.0.0", src.Constraints.Version)
}

func TestSetComponentManifestVersion_PreservesFormatting(t *testing.T) {
	file := writeFile(t, t.TempDir(), "component.yaml", componentManifestFixture)

	require.NoError(t, SetComponentManifestVersion(file, "v2.0.0"))

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	s := string(got)
	assert.Contains(t, s, `version: "v2.0.0"`, "version updated")
	assert.Contains(t, s, "# pinned version", "inline comment preserved")
	assert.Contains(t, s, "# component-level constraints.", "head comment preserved")
	assert.Contains(t, s, "{{.Version}}", "template in source uri preserved")

	v, err := atmosyaml.GetFile(file, "spec.source.version")
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", v)
}
