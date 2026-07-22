package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

// fileURIFromPath converts an absolute filesystem path into a "file://" URI that
// resolveFileURIPath can parse back to the original path on every platform: on POSIX the path
// already starts with "/", so it is used as-is; on Windows (e.g. "C:\foo") it is slash-converted
// and given a leading "/" so the URI reads "file:///C:/foo", matching resolveFileURIPath's
// documented drive-letter handling.
func fileURIFromPath(path string) string {
	sourcePath := filepath.ToSlash(path)
	if len(sourcePath) == 0 || sourcePath[0] != '/' {
		sourcePath = "/" + sourcePath
	}
	return "file://" + sourcePath
}

// TestResolveFileURIPath_NotFileScheme proves resolveFileURIPath only handles "file://" URIs,
// leaving every other scheme (or a bare path) untouched.
func TestResolveFileURIPath_NotFileScheme(t *testing.T) {
	for _, uri := range []string{
		"github.com/cloudposse/terraform-null-label.git",
		"https://example.com/foo",
		"/already/absolute/path",
		"relative/path",
	} {
		_, ok := resolveFileURIPath(uri)
		assert.False(t, ok, "uri %q must not be treated as a file:// scheme", uri)
	}
}

// TestResolveFileURIPath_PreservesAbsoluteRoot proves a "file://" URI resolves to the absolute
// path it names, instead of being trimmed down to a path relative to the current directory.
// Regression: "file:///tmp/source" previously resolved to the relative "tmp/source" because the
// leading "/" was unconditionally stripped.
func TestResolveFileURIPath_PreservesAbsoluteRoot(t *testing.T) {
	absPath := filepath.Join(t.TempDir(), "source")

	resolved, ok := resolveFileURIPath(fileURIFromPath(absPath))
	require.True(t, ok)
	assert.Equal(t, absPath, resolved)
}

// TestHandleLocalFileScheme_FileURI_RecomputesSourceIsLocalFile proves handleLocalFileScheme
// recognizes an existing file named by a "file://" URI (sourceIsLocalFile), not just an existing
// file named by a plain path resolved against componentPath.
func TestHandleLocalFileScheme_FileURI_RecomputesSourceIsLocalFile(t *testing.T) {
	tempDir := t.TempDir()
	sourceFile := filepath.Join(tempDir, "source")
	require.NoError(t, os.WriteFile(sourceFile, []byte("test"), 0o644))

	uri, useLocalFileSystem, sourceIsLocalFile := handleLocalFileScheme(t.TempDir(), fileURIFromPath(sourceFile))

	assert.Equal(t, sourceFile, uri)
	assert.True(t, useLocalFileSystem)
	assert.True(t, sourceIsLocalFile, "sourceIsLocalFile must be recomputed for the resolved file:// path")
}

// TestProcessComponentMixins_FileURI_NormalizesToAbsolutePath proves a mixin declared with a
// "file://" URI resolves to the absolute path it names (shared with handleLocalFileScheme via
// resolveFileURIPath), instead of a broken relative path.
func TestProcessComponentMixins_FileURI_NormalizesToAbsolutePath(t *testing.T) {
	// A path that does not exist locally, so processComponentMixins does not skip it via the
	// already-materialized dedup check.
	missingSource := filepath.Join(t.TempDir(), "does-not-exist")

	spec := &schema.VendorComponentSpec{
		Mixins: []schema.VendorComponentMixins{
			{Uri: fileURIFromPath(missingSource), Filename: "context.tf"},
		},
	}

	packages, err := processComponentMixins(nil, spec, t.TempDir())
	require.NoError(t, err)
	require.Len(t, packages, 1)
	assert.Equal(t, missingSource, packages[0].URI())
}

// TestBuildComponentVendorPackages_OciScheme proves a component source.uri using the "oci://"
// scheme is classified as pkgTypeOci with the scheme stripped from the stored uri, matching the
// mixin-side handling in resolveMixinPackage.
func TestBuildComponentVendorPackages_OciScheme(t *testing.T) {
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{Uri: "oci://ghcr.io/cloudposse/vpc:1.0.0"},
	}

	packages, err := buildComponentVendorPackages(buildComponentPackagesOptions{VendorComponentSpec: spec, Component: "vpc", ComponentPath: t.TempDir()})

	require.NoError(t, err)
	require.Len(t, packages, 1)
	assert.Equal(t, install.PkgTypeOci, packages[0].PkgType())
	assert.Equal(t, "ghcr.io/cloudposse/vpc:1.0.0", packages[0].URI())
}

// TestBuildComponentVendorPackages_SemverRangeResolvesAndTemplatesConcreteVersion proves a
// component.yaml `version:` that is a semver range resolves (via the injected fakeTagLister, no
// real network access) to a concrete version, and that concrete version -- not the literal range
// string -- is what's templated into source.uri.
func TestBuildComponentVendorPackages_SemverRangeResolvesAndTemplatesConcreteVersion(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			Uri:     "github.com/cloudposse/terraform-aws-vpc.git?ref={{.Version}}",
			Version: "^1.0.0",
		},
	}

	packages, err := buildComponentVendorPackages(buildComponentPackagesOptions{
		AtmosConfig:         atmosConfig,
		VendorComponentSpec: spec,
		Component:           "vpc",
		ComponentPath:       t.TempDir(),
		Lister:              &fakeTagLister{tags: []string{"v1.0.0", "v1.2.3", "v1.5.0", "v2.0.0"}},
	})

	require.NoError(t, err)
	require.Len(t, packages, 1)
	assert.Equal(t, "v1.5.0", packages[0].Version)
	assert.Equal(t, "^1.0.0", packages[0].RawVersion)
	assert.Contains(t, packages[0].URI(), "ref=v1.5.0")
}

// TestBuildComponentVendorPackages_SemverRangeOnNonGitSourceReturnsClearError proves a
// component.yaml `version:` that is a semver range (e.g. "^1.0.0") on a source with no
// tag-listing mechanism (OCI here) fails buildComponentVendorPackages with a clear, wrapped error
// instead of silently templating the literal range string into the uri. Network-free: a non-Git
// source is rejected by install.ResolveDeclaredVersion before it ever reaches the (real, unmocked)
// default Lister.
func TestBuildComponentVendorPackages_SemverRangeOnNonGitSourceReturnsClearError(t *testing.T) {
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{Uri: "oci://ghcr.io/cloudposse/vpc:{{.Version}}", Version: "^1.0.0"},
	}

	_, err := buildComponentVendorPackages(buildComponentPackagesOptions{AtmosConfig: &schema.AtmosConfiguration{BasePath: t.TempDir()}, VendorComponentSpec: spec, Component: "vpc", ComponentPath: t.TempDir()})

	require.Error(t, err)
	require.ErrorIs(t, err, install.ErrVersionRangeRequiresGitSource)
}

// TestBuildComponentVendorPackages_SemverRangeConflictsWithConstraintsVersion proves a
// component.yaml source combining a semver-range version: with a constraints.version ceiling is
// rejected before any resolution/network access is attempted -- the two are mutually exclusive.
func TestBuildComponentVendorPackages_SemverRangeConflictsWithConstraintsVersion(t *testing.T) {
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			Uri:         "github.com/cloudposse/terraform-aws-vpc.git?ref={{.Version}}",
			Version:     "^1.0.0",
			Constraints: &schema.VendorConstraints{Version: "<2.0.0"},
		},
	}

	_, err := buildComponentVendorPackages(buildComponentPackagesOptions{
		AtmosConfig:         &schema.AtmosConfiguration{BasePath: t.TempDir()},
		VendorComponentSpec: spec,
		Component:           "vpc",
		ComponentPath:       t.TempDir(),
		Lister:              &fakeTagLister{tags: []string{"v1.0.0", "v1.5.0"}},
	})

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrVersionRangeConflictsWithConstraints)
}

// TestResolveMixinPackage_OciScheme proves a mixin uri using the "oci://" scheme is classified as
// pkgTypeOci with the scheme stripped, mirroring buildComponentVendorPackages' component-level
// handling of the same scheme.
func TestResolveMixinPackage_OciScheme(t *testing.T) {
	spec := &schema.VendorComponentSpec{}
	mixin := &schema.VendorComponentMixins{Uri: "oci://ghcr.io/cloudposse/mixins:context.tf", Filename: "context.tf"}

	pkg, alreadyMaterialized, err := resolveMixinPackage(nil, spec, mixin, t.TempDir())

	require.NoError(t, err)
	assert.False(t, alreadyMaterialized)
	assert.Equal(t, install.PkgTypeOci, pkg.PkgType())
	assert.Equal(t, "ghcr.io/cloudposse/mixins:context.tf", pkg.URI())
}

// TestResolveMixinPackage_AlreadyMaterialized proves a mixin whose target file already exists at
// the resolved destination path is reported as already materialized (nothing to fetch), instead
// of producing a package that would re-download and overwrite it.
func TestResolveMixinPackage_AlreadyMaterialized(t *testing.T) {
	componentPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "context.tf"), []byte("# already here\n"), 0o644))

	spec := &schema.VendorComponentSpec{}
	mixin := &schema.VendorComponentMixins{Uri: "context.tf", Filename: "context.tf"}

	pkg, alreadyMaterialized, err := resolveMixinPackage(nil, spec, mixin, componentPath)

	require.NoError(t, err)
	assert.True(t, alreadyMaterialized)
	assert.Equal(t, install.VendorPackage{}, pkg, "an already-materialized mixin returns the zero-value package, since there is nothing to fetch")
}

// TestResolveMixinPackage_TemplateError proves an invalid Go template in a versioned mixin's uri
// is surfaced as an error identifying the failing mixin (filename and uri), rather than a bare
// template-package error with no mixin context.
func TestResolveMixinPackage_TemplateError(t *testing.T) {
	spec := &schema.VendorComponentSpec{}
	mixin := &schema.VendorComponentMixins{
		Uri:      "https://example.com/{{.Version",
		Version:  "1.0.0",
		Filename: "context.tf",
	}

	_, alreadyMaterialized, err := resolveMixinPackage(nil, spec, mixin, t.TempDir())

	require.Error(t, err)
	assert.False(t, alreadyMaterialized)
	assert.Contains(t, err.Error(), "context.tf", "the error must identify which mixin failed to template")
}

// TestParseMixinURI_RendersVersionTemplate proves a versioned mixin's uri template is rendered
// against the mixin itself (so `{{ .Version }}` resolves), instead of being returned unparsed.
func TestParseMixinURI_RendersVersionTemplate(t *testing.T) {
	mixin := &schema.VendorComponentMixins{
		Uri:      "https://example.com/archive/{{ .Version }}/context.tf",
		Version:  "1.2.3",
		Filename: "context.tf",
	}

	uri, err := parseMixinURI(nil, mixin)

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/archive/1.2.3/context.tf", uri)
}

// TestProcessComponentMixins_MissingFilename proves a mixin with a uri but no filename is
// rejected before resolveMixinPackage is ever invoked, matching the sibling missing-uri check.
func TestProcessComponentMixins_MissingFilename(t *testing.T) {
	spec := &schema.VendorComponentSpec{
		Mixins: []schema.VendorComponentMixins{
			{Uri: "github.com/cloudposse/terraform-null-label.git//exports/context.tf?ref=0.1.0"}, // Missing Filename.
		},
	}

	packages, err := processComponentMixins(nil, spec, t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingMixinFilename)
	assert.Nil(t, packages)
}

// TestProcessComponentMixins_AlreadyMaterializedMixinIsSkipped proves a mixin whose target already
// exists on disk produces no package (it is silently skipped, not re-fetched), while a second,
// not-yet-materialized mixin in the same component.yaml still produces one.
func TestProcessComponentMixins_AlreadyMaterializedMixinIsSkipped(t *testing.T) {
	componentPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "context.tf"), []byte("# already here\n"), 0o644))

	spec := &schema.VendorComponentSpec{
		Mixins: []schema.VendorComponentMixins{
			{Uri: "context.tf", Filename: "context.tf"},
			{Uri: "github.com/cloudposse/terraform-null-label.git//exports/fixtures.tf?ref=0.1.0", Filename: "fixtures.tf"},
		},
	}

	packages, err := processComponentMixins(nil, spec, componentPath)

	require.NoError(t, err)
	require.Len(t, packages, 1, "only the not-yet-materialized mixin should produce a package")
	assert.Equal(t, "fixtures.tf", packages[0].MixinFilename())
}

// TestProcessComponentMixins_PropagatesResolveError proves a mixin whose uri fails template
// evaluation fails the whole component's mixin processing, not just that one mixin.
func TestProcessComponentMixins_PropagatesResolveError(t *testing.T) {
	spec := &schema.VendorComponentSpec{
		Mixins: []schema.VendorComponentMixins{
			{Uri: "https://example.com/{{.Version", Version: "1.0.0", Filename: "context.tf"},
		},
	}

	packages, err := processComponentMixins(nil, spec, t.TempDir())

	require.Error(t, err)
	assert.Nil(t, packages)
}
