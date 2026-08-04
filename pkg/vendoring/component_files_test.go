package vendoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
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

// TestResolveComponentPath covers the resolution contract: an absolute path for an existing
// component directory, ErrComponentDirNotFound for a missing or non-directory path (the
// missing-path case flows through u.IsDirectory's os.Stat error, not the !dirExists branch),
// and ErrUnsupportedComponentType for unknown component types.
func TestResolveComponentPath(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "components", "terraform", "vpc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "components", "terraform", "file.txt"), []byte("x"), 0o644))
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Components.Terraform.BasePath = filepath.Join("components", "terraform")

	t.Run("existing directory resolves to an absolute path", func(t *testing.T) {
		resolved, err := ResolveComponentPath(config, "vpc", "terraform")
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(resolved))
		assert.Equal(t, filepath.Join(base, "components", "terraform", "vpc"), resolved)
	})

	t.Run("missing directory returns ErrComponentDirNotFound", func(t *testing.T) {
		_, err := ResolveComponentPath(config, "missing", "terraform")
		require.ErrorIs(t, err, errUtils.ErrComponentDirNotFound)
	})

	t.Run("regular file returns ErrComponentDirNotFound", func(t *testing.T) {
		_, err := ResolveComponentPath(config, "file.txt", "terraform")
		require.ErrorIs(t, err, errUtils.ErrComponentDirNotFound)
	})

	t.Run("unsupported component type is rejected", func(t *testing.T) {
		_, err := ResolveComponentPath(config, "vpc", "ansible2")
		require.ErrorIs(t, err, errUtils.ErrUnsupportedComponentType)
	})
}
