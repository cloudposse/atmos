package vendoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// fakeComponentDirResolver implements ComponentDirResolver against a fixed directory, so
// ResolveComponentSource can be tested without loading a real atmos.yaml.
type fakeComponentDirResolver struct {
	dir string
	err error
}

func (f fakeComponentDirResolver) ComponentDir(_, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.dir, nil
}

// chdir switches the working directory for the duration of the test (VendorFilePresent looks for
// ./vendor.yaml relative to cwd) and restores it on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
}

func TestResolveComponentSource_VendorYamlDeclaresComponent_UsesVendorYaml(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, dir, "vendor.yaml", `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: "1.0.0"
      targets: ["components/terraform/vpc"]
`)
	componentDir := filepath.Join(dir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	writeFile(t, componentDir, "component.yaml", componentManifestFixture)

	resolved, err := ResolveComponentSource(&ResolveSourceParams{
		Component: "vpc",
		Resolver:  fakeComponentDirResolver{dir: componentDir},
	})
	require.NoError(t, err)
	assert.False(t, resolved.FromComponentManifest, "vendor.yaml must win when it declares the component")
	assert.Equal(t, "github.com/cloudposse/terraform-aws-vpc", resolved.Source.Source)
	assert.Equal(t, "1.0.0", resolved.Source.Version)
}

func TestResolveComponentSource_FallsBackWhenVendorYamlDoesNotDeclareComponent(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, dir, "vendor.yaml", `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: other
      source: github.com/cloudposse/terraform-aws-other
      version: "1.0.0"
      targets: ["components/terraform/other"]
`)
	componentDir := filepath.Join(dir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	writeFile(t, componentDir, "component.yaml", componentManifestFixture)

	resolved, err := ResolveComponentSource(&ResolveSourceParams{
		Component: "vpc",
		Resolver:  fakeComponentDirResolver{dir: componentDir},
	})
	require.NoError(t, err)
	assert.True(t, resolved.FromComponentManifest)
	assert.Equal(t, "github.com/cloudposse/terraform-aws-vpc?ref={{.Version}}", resolved.Source.Source)
	assert.Equal(t, filepath.Join(componentDir, "component.yaml"), resolved.File)
}

func TestResolveComponentSource_FallsBackWhenNoVendorYamlAtAll(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	componentDir := filepath.Join(dir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	writeFile(t, componentDir, "component.yaml", componentManifestFixture)

	resolved, err := ResolveComponentSource(&ResolveSourceParams{
		Component: "vpc",
		Resolver:  fakeComponentDirResolver{dir: componentDir},
	})
	require.NoError(t, err)
	assert.True(t, resolved.FromComponentManifest)
	assert.Equal(t, "1.2.3", resolved.Source.Version)
}

func TestResolveComponentSource_NeitherExists_ReturnsClearError(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	componentDir := filepath.Join(dir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	_, err := ResolveComponentSource(&ResolveSourceParams{
		Component: "vpc",
		Resolver:  fakeComponentDirResolver{dir: componentDir},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVendorSourceNotFound)
	assert.Contains(t, err.Error(), componentDir)
}

// TestResolveComponentSource_VendorYamlExistsButComponentMissingEverywhere is
// TestResolveComponentSource_NeitherExists_ReturnsClearError's counterpart when a vendor.yaml IS
// present (just doesn't declare the requested component): notFoundError's message must say so
// (mentioning the vendor.yaml, not just "no vendor.yaml found"), distinguishing "nothing at all"
// from "checked vendor.yaml too, still nothing".
func TestResolveComponentSource_VendorYamlExistsButComponentMissingEverywhere(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, dir, "vendor.yaml", `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: other
      source: github.com/cloudposse/terraform-aws-other
      version: "1.0.0"
      targets: ["components/terraform/other"]
`)
	componentDir := filepath.Join(dir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	_, err := ResolveComponentSource(&ResolveSourceParams{
		Component: "vpc",
		Resolver:  fakeComponentDirResolver{dir: componentDir},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrVendorSourceNotFound)
	assert.Contains(t, err.Error(), "vendor.yaml", "the error must mention that vendor.yaml itself was checked")
	assert.Contains(t, err.Error(), componentDir)
}

func TestResolveComponentSource_PropagatesBrokenVendorYamlError(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "vendor.yaml", "spec: [")

	_, err := ResolveComponentSource(&ResolveSourceParams{
		VendorFile: file,
		Component:  "vpc",
		Resolver:   fakeComponentDirResolver{dir: t.TempDir()},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrParseVendorFile, "a broken explicit --file must not be silently masked by a fallback attempt")
}

func TestResolveComponentSource_RejectsUnsupportedComponentType(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	_, err := ResolveComponentSource(&ResolveSourceParams{
		Component:     "vpc",
		ComponentType: "bogus",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedComponentType)
}

// TestResolveComponentSource_DefaultResolver_FindsRealComponentDir proves ResolveComponentSource
// works end to end through DefaultComponentDirResolver (no Resolver override, the production
// path), not just the fakeComponentDirResolver every other test in this file injects.
func TestResolveComponentSource_DefaultResolver_FindsRealComponentDir(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	base := filepath.Join(dir, "components", "terraform")
	componentDir := filepath.Join(base, "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	writeFile(t, componentDir, "component.yaml", componentManifestFixture)
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", base)

	resolved, err := ResolveComponentSource(&ResolveSourceParams{Component: "vpc"})
	require.NoError(t, err)
	assert.True(t, resolved.FromComponentManifest)
	assert.Equal(t, filepath.Join(componentDir, "component.yaml"), resolved.File)
}

// TestResolveComponentSource_PropagatesBrokenComponentManifestError proves a malformed
// component.yaml fallback (found on disk, but unparseable) is surfaced rather than mistaken for
// "component.yaml doesn't exist".
func TestResolveComponentSource_PropagatesBrokenComponentManifestError(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeFile(t, dir, "vendor.yaml", `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: other
      source: github.com/cloudposse/terraform-aws-other
      version: "1.0.0"
      targets: ["components/terraform/other"]
`)
	componentDir := filepath.Join(dir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	writeFile(t, componentDir, "component.yaml", "spec: [")

	_, err := ResolveComponentSource(&ResolveSourceParams{
		Component: "vpc",
		Resolver:  fakeComponentDirResolver{dir: componentDir},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrParseVendorFile)
}

func TestVendorFilePresent(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	_, ok := VendorFilePresent("")
	assert.False(t, ok, "no vendor.yaml on disk")

	writeFile(t, dir, "vendor.yaml", "spec: {}\n")
	file, ok := VendorFilePresent("")
	assert.True(t, ok)
	assert.Equal(t, DefaultVendorFile, file)

	override := filepath.Join(t.TempDir(), "custom.yaml")
	file, ok = VendorFilePresent(override)
	assert.True(t, ok, "an explicit override is trusted without a stat check")
	assert.Equal(t, override, file)
}

// TestVendorFilePresent_HonorsVendorBasePath reproduces the reported bug: `vendor update`/`diff`/
// `get`/`set` must respect atmos.yaml's vendor.base_path (e.g. set via --chdir into a repo that
// configures it), not just a bare ./vendor.yaml relative to the process cwd.
func TestVendorFilePresent_HonorsVendorBasePath(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	vendorDir := filepath.Join(dir, "vendor-configs")
	require.NoError(t, os.MkdirAll(vendorDir, 0o755))
	vendorFile := writeFile(t, vendorDir, "vendor.yaml", "spec: {}\n")

	// No vendor.yaml sits at the default cwd-relative location.
	_, ok := VendorFilePresent("")
	assert.False(t, ok, "vendor.yaml only exists at the configured vendor.base_path, not cwd")

	t.Setenv("ATMOS_VENDOR_BASE_PATH", vendorFile)

	file, ok := VendorFilePresent("")
	assert.True(t, ok, "vendor.base_path from atmos config should be honored")
	assert.Equal(t, vendorFile, file)
}

// TestVendorFilePresent_VendorBasePathConfiguredButMissing proves an explicitly configured
// vendor.base_path that doesn't exist on disk is treated as "not present" rather than falling
// back to a bare cwd/atmosConfig.BasePath search -- an explicit setting that points at nothing is
// a misconfiguration, not "unset".
func TestVendorFilePresent_VendorBasePathConfiguredButMissing(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	t.Setenv("ATMOS_VENDOR_BASE_PATH", filepath.Join(dir, "does-not-exist.yaml"))

	_, ok := VendorFilePresent("")
	assert.False(t, ok, "a configured but missing vendor.base_path must not fall back to a cwd search")
}

// TestVendorFilePresent_FallsBackToAtmosBasePathVendorYaml proves that, with no vendor.base_path
// configured and no ./vendor.yaml at cwd, VendorFilePresent still finds a vendor.yaml living at
// atmosConfig.BasePath (the second u.SearchConfigFile fallback), not just a bare cwd-relative one.
func TestVendorFilePresent_FallsBackToAtmosBasePathVendorYaml(t *testing.T) {
	cwd := t.TempDir()
	chdir(t, cwd)

	baseDir := t.TempDir()
	vendorFile := writeFile(t, baseDir, "vendor.yaml", "spec: {}\n")
	t.Setenv("ATMOS_BASE_PATH", baseDir)

	file, ok := VendorFilePresent("")
	require.True(t, ok, "vendor.yaml at atmosConfig.BasePath must be found even without vendor.base_path set")
	assert.Equal(t, vendorFile, file)
}

// TestVendorFilePresent_VendorBasePathRelativeToAtmosBasePath covers a relative vendor.base_path,
// which must be joined against atmosConfig.BasePath (matching resolveVendorConfigFilePath in
// internal/exec/vendor_utils.go, used by `atmos vendor pull`).
func TestVendorFilePresent_VendorBasePathRelativeToAtmosBasePath(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	vendorDir := filepath.Join(dir, "vendor-configs")
	require.NoError(t, os.MkdirAll(vendorDir, 0o755))
	writeFile(t, vendorDir, "vendor.yaml", "spec: {}\n")

	t.Setenv("ATMOS_BASE_PATH", dir)
	t.Setenv("ATMOS_VENDOR_BASE_PATH", filepath.Join("vendor-configs", "vendor.yaml"))

	file, ok := VendorFilePresent("")
	assert.True(t, ok, "relative vendor.base_path should be joined against atmosConfig.BasePath")
	assert.Equal(t, filepath.Join(dir, "vendor-configs", "vendor.yaml"), file)
}

func TestDiscoverComponentManifests_SkipsDirsWithoutManifestAndErrorsOnMalformed(t *testing.T) {
	base := t.TempDir()
	vpcDir := filepath.Join(base, "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	writeFile(t, vpcDir, "component.yaml", componentManifestFixture)

	require.NoError(t, os.MkdirAll(filepath.Join(base, "no-manifest"), 0o755))

	sources, err := DiscoverComponentManifests(base, "terraform")
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "vpc", sources[0].Source.Component)
	assert.True(t, sources[0].FromComponentManifest)

	malformedDir := filepath.Join(base, "broken")
	require.NoError(t, os.MkdirAll(malformedDir, 0o755))
	writeFile(t, malformedDir, "component.yaml", "spec: [")

	_, err = DiscoverComponentManifests(base, "terraform")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrParseVendorFile)
}

func TestDiscoverComponentManifests_MissingBasePathReturnsEmpty(t *testing.T) {
	sources, err := DiscoverComponentManifests(filepath.Join(t.TempDir(), "does-not-exist"), "terraform")
	require.NoError(t, err)
	assert.Empty(t, sources)
}

// TestDiscoverComponentManifests_BasePathIsAFile proves a non-ENOENT os.ReadDir failure (basePath
// exists but isn't a directory) is surfaced as an error, unlike the missing-basePath case above
// (a legitimate "nothing vendored this way yet" state) which returns an empty result instead.
func TestDiscoverComponentManifests_BasePathIsAFile(t *testing.T) {
	basePath := writeFile(t, t.TempDir(), "not-a-dir", "oops")

	_, err := DiscoverComponentManifests(basePath, "terraform")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadVendorFile)
}

// TestDiscoverComponentManifests_SkipsNonDirectoryEntries proves a stray file sitting alongside
// real component directories under basePath is skipped rather than mistaken for a component.
func TestDiscoverComponentManifests_SkipsNonDirectoryEntries(t *testing.T) {
	base := t.TempDir()
	vpcDir := filepath.Join(base, "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	writeFile(t, vpcDir, "component.yaml", componentManifestFixture)
	writeFile(t, base, "README.md", "not a component directory")

	sources, err := DiscoverComponentManifests(base, "terraform")

	require.NoError(t, err)
	require.Len(t, sources, 1, "the stray README.md file must be skipped, not treated as a component directory")
	assert.Equal(t, "vpc", sources[0].Source.Component)
}

func TestDiscoverAllComponentManifests_ScansConfiguredBasePath(t *testing.T) {
	base := t.TempDir()
	vpcDir := filepath.Join(base, "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	writeFile(t, vpcDir, "component.yaml", componentManifestFixture)

	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", base)
	t.Setenv("ATMOS_COMPONENTS_HELMFILE_BASE_PATH", filepath.Join(t.TempDir(), "empty-helmfile"))
	t.Setenv("ATMOS_COMPONENTS_PACKER_BASE_PATH", filepath.Join(t.TempDir(), "empty-packer"))

	all, err := DiscoverAllComponentManifests("terraform", true)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "vpc", all[0].Source.Component)

	all, err = DiscoverAllComponentManifests("terraform", false)
	require.NoError(t, err)
	require.Len(t, all, 1, "non-terraform base paths point at empty directories")
}

// TestDiscoverAllComponentManifests_UnsupportedTypePropagatesError proves an unsupported
// --type (only reachable with onlyType=true, i.e. a "vendor update --type <type>
// --component-manifests" run) surfaces GetComponentBasePath's error rather than silently
// scanning nothing.
func TestDiscoverAllComponentManifests_UnsupportedTypePropagatesError(t *testing.T) {
	_, err := DiscoverAllComponentManifests("bogus", true)

	require.Error(t, err)
}

// TestDiscoverAllComponentManifests_PropagatesDiscoverError proves a malformed component.yaml
// found while sweeping any configured type fails the whole sweep, matching
// DiscoverComponentManifests's own fail-fast-on-malformed-manifest behavior.
func TestDiscoverAllComponentManifests_PropagatesDiscoverError(t *testing.T) {
	base := t.TempDir()
	brokenDir := filepath.Join(base, "broken")
	require.NoError(t, os.MkdirAll(brokenDir, 0o755))
	writeFile(t, brokenDir, "component.yaml", "spec: [")

	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", base)

	_, err := DiscoverAllComponentManifests("terraform", true)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrParseVendorFile)
}
