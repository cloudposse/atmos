package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// writeLocalComponentVendorConfig writes a component.yaml under <basePath>/<component> whose
// source.uri points at sourceDir (an absolute local path, so it resolves as pkgTypeLocal and is
// copied directly off disk - no network access, matching this repo's "prefer unit tests with
// mocks/fakes over integration tests" convention). Returns the component's own directory.
func writeLocalComponentVendorConfig(t *testing.T, basePath, component, sourceDir string) string {
	t.Helper()
	componentDir := filepath.Join(basePath, component)
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	content := `apiVersion: atmos/v1
kind: ComponentVendorConfig
spec:
  source:
    uri: "` + filepath.ToSlash(sourceDir) + `"
`
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "component.yaml"), []byte(content), 0o644))
	return componentDir
}

// TestExecuteComponentVendorInternal_PullsLocalSource proves the single-component entry point
// (the "atmos vendor pull --component X" path, invoked by internal/exec/vendor.go's
// handleComponentVendor) resolves and pulls a local source's files onto disk.
func TestExecuteComponentVendorInternal_PullsLocalSource(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))
	componentPath := t.TempDir()

	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{Uri: sourceDir},
	}

	err := ExecuteComponentVendorInternal(&schema.AtmosConfiguration{BasePath: t.TempDir()}, spec, "vpc", componentPath, false)

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(componentPath, "main.tf"))
}

// TestExecuteComponentVendorInternal_PropagatesBuildError proves an unresolvable spec (missing
// source URI) fails before executeVendorModel is ever invoked.
func TestExecuteComponentVendorInternal_PropagatesBuildError(t *testing.T) {
	err := ExecuteComponentVendorInternal(&schema.AtmosConfiguration{}, &schema.VendorComponentSpec{}, "vpc", t.TempDir(), false)

	assert.Error(t, err)
}

// TestExecuteComponentVendorPullBatch_PullsAllComponentsInOneCall proves the batched entry point
// used by "atmos vendor update --pull" (cmd/vendor/update.go's runVendorPull) resolves and pulls
// every named component, landing each one's files on disk from a single call.
func TestExecuteComponentVendorPullBatch_PullsAllComponentsInOneCall(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "components", "terraform")
	require.NoError(t, os.MkdirAll(basePath, 0o755))

	names := []string{"vpc", "eks", "account"}
	dirs := make(map[string]string, len(names))
	for _, name := range names {
		sourceDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# "+name+"\n"), 0o644))
		dirs[name] = writeLocalComponentVendorConfig(t, basePath, name, sourceDir)
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}

	err := ExecuteComponentVendorPullBatch(atmosConfig, names, cfg.TerraformComponentType, false)
	require.NoError(t, err, "batched pull of multiple component.yaml-declared components must succeed")

	for _, name := range names {
		assert.FileExists(t, filepath.Join(dirs[name], "main.tf"), "component %q must have been pulled", name)
	}
}

// TestExecuteComponentVendorPullBatch_EmptyComponents_NoOp proves an empty component list is a
// pure no-op: no atmosConfig-dependent resolution is even attempted.
func TestExecuteComponentVendorPullBatch_EmptyComponents_NoOp(t *testing.T) {
	err := ExecuteComponentVendorPullBatch(&schema.AtmosConfiguration{}, nil, cfg.TerraformComponentType, false)
	assert.NoError(t, err)
}

// TestExecuteComponentVendorPullBatch_PropagatesResolutionError proves a component whose
// component.yaml can't be resolved fails the whole batch immediately (fail-fast), rather than
// silently skipping it and under-pulling the rest.
func TestExecuteComponentVendorPullBatch_PropagatesResolutionError(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "components", "terraform")
	require.NoError(t, os.MkdirAll(basePath, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"does-not-exist"}, cfg.TerraformComponentType, false)
	assert.Error(t, err, "an unresolvable component must fail the batch")
}

// TestExecuteComponentVendorPullBatch_PropagatesBuildError proves a component.yaml that resolves
// fine (valid YAML) but declares no source uri fails the batch via buildComponentVendorPackages,
// distinct from TestExecuteComponentVendorPullBatch_PropagatesResolutionError's
// config-resolution failure.
func TestExecuteComponentVendorPullBatch_PropagatesBuildError(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "components", "terraform")
	componentDir := filepath.Join(basePath, "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "component.yaml"), []byte(`apiVersion: atmos/v1
kind: ComponentVendorConfig
spec: {}
`), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `component "vpc"`, "the failing component's name must be identifiable in a multi-component batch")
}

// TestBuildComponentVendorPackages_ComponentAndMixins proves buildComponentVendorPackages (the
// helper extracted from ExecuteComponentVendorInternal so ExecuteComponentVendorPullBatch can
// build package lists for multiple components without executing each one) returns the same shape
// ExecuteComponentVendorInternal used to build inline: the component package first, IsComponent
// true, followed by one entry per mixin with IsMixins true.
func TestBuildComponentVendorPackages_ComponentAndMixins(t *testing.T) {
	componentPath := t.TempDir()

	// A local mixin file that already exists at the resolved path is skipped by
	// processComponentMixins (it only fetches mixins not already present), so point the mixin uri
	// at a non-existent local file to ensure it produces a package entry.
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			Uri: "github.com/cloudposse/terraform-null-label.git//?ref=0.1.0",
		},
		Mixins: []schema.VendorComponentMixins{
			{Uri: "github.com/cloudposse/terraform-null-label.git//exports/context.tf?ref=0.1.0", Filename: "context.tf"},
		},
	}

	packages, err := buildComponentVendorPackages(spec, "vpc", componentPath)
	require.NoError(t, err)
	require.Len(t, packages, 2)

	assert.Equal(t, "vpc", packages[0].name)
	assert.True(t, packages[0].IsComponent)
	assert.False(t, packages[0].IsMixins)

	assert.True(t, packages[1].IsMixins)
	assert.False(t, packages[1].IsComponent)
	assert.Equal(t, "context.tf", packages[1].mixinFilename)
}

// TestBuildComponentVendorPackages_MissingUri proves an empty source URI is rejected before any
// package is built, matching ExecuteComponentVendorInternal's pre-refactor behavior.
func TestBuildComponentVendorPackages_MissingUri(t *testing.T) {
	spec := &schema.VendorComponentSpec{}
	packages, err := buildComponentVendorPackages(spec, "vpc", t.TempDir())
	assert.Error(t, err)
	assert.Nil(t, packages)
}

// TestBuildComponentVendorPackages_InvalidUriTemplate proves a source.uri that fails to parse as
// a Go template (only attempted when source.version is set) is surfaced rather than silently
// falling back to the literal, unparsed uri.
func TestBuildComponentVendorPackages_InvalidUriTemplate(t *testing.T) {
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			Uri:     "github.com/cloudposse/terraform-null-label.git//?ref={{.Version",
			Version: "0.1.0",
		},
	}

	packages, err := buildComponentVendorPackages(spec, "vpc", t.TempDir())

	assert.Error(t, err)
	assert.Nil(t, packages)
}

// TestBuildComponentVendorPackages_PropagatesMixinError proves a mixin missing its required uri
// fails the whole component, matching processComponentMixins' own fail-fast validation.
func TestBuildComponentVendorPackages_PropagatesMixinError(t *testing.T) {
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			Uri: "github.com/cloudposse/terraform-null-label.git//?ref=0.1.0",
		},
		Mixins: []schema.VendorComponentMixins{
			{Filename: "context.tf"}, // Missing Uri.
		},
	}

	packages, err := buildComponentVendorPackages(spec, "vpc", t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingMixinURI)
	assert.Nil(t, packages)
}
