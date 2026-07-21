package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
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

	// componentPath must live under basePath, matching how ReadAndProcessComponentVendorConfigFile
	// always constructs it (filepath.Join(atmosConfig.BasePath, componentBasePath, component)) - the
	// vendor lock's target-containment check rejects a target outside the configured project root.
	basePath := t.TempDir()
	componentPath := filepath.Join(basePath, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))

	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{Uri: sourceDir},
	}

	err := ExecuteComponentVendorInternal(&schema.AtmosConfiguration{BasePath: basePath}, spec, "vpc", componentPath, false, false)

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(componentPath, "main.tf"))
}

// TestExecuteComponentVendorInternal_PropagatesBuildError proves an unresolvable spec (missing
// source URI) fails before executeVendorModel is ever invoked.
func TestExecuteComponentVendorInternal_PropagatesBuildError(t *testing.T) {
	err := ExecuteComponentVendorInternal(&schema.AtmosConfiguration{}, &schema.VendorComponentSpec{}, "vpc", t.TempDir(), false, false)

	assert.Error(t, err)
}

// TestExecuteComponentVendorInternal_RefreshLock_ForcesReDownload proves refreshLock=true bypasses
// filterMaterializedComponentVendorPackages, forcing an already-materialized component through
// executeVendorModel again - mirroring internal/exec/vendor_utils.go's
// "!params.dryRun && !params.refreshLock" gate already used by the vendor.yaml path. This is the
// single-"--component"-path companion to
// TestExecuteVendorPullCommand_Everything_NoVendorFile_RefreshLock_ForcesReDownload
// (vendor_pull_sweep_test.go), which proves the same behavior through the repo-wide sweep.
func TestExecuteComponentVendorInternal_RefreshLock_ForcesReDownload(t *testing.T) {
	sourceDir := t.TempDir()
	sourceFile := filepath.Join(sourceDir, "main.tf")
	require.NoError(t, os.WriteFile(sourceFile, []byte("# v1\n"), 0o644))

	basePath := t.TempDir()
	componentPath := filepath.Join(basePath, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	targetFile := filepath.Join(componentPath, "main.tf")

	spec := &schema.VendorComponentSpec{Source: schema.VendorComponentSource{Uri: sourceDir}}
	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}

	// First pull: materializes and records a lock entry.
	require.NoError(t, ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, false, false))
	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	require.Equal(t, "# v1\n", string(content))

	// Mutate the upstream source.
	require.NoError(t, os.WriteFile(sourceFile, []byte("# v2\n"), 0o644))

	// Second pull without refreshLock: already-materialized target, must be skipped.
	require.NoError(t, ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, false, false))
	content, err = os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "# v1\n", string(content), "without refreshLock, an already-materialized component must not be re-pulled")

	// Third pull with refreshLock: must bypass the materialization filter and re-pull.
	require.NoError(t, ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, false, true))
	content, err = os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "# v2\n", string(content), "refreshLock must force a re-download even when already materialized")
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

	err := ExecuteComponentVendorPullBatch(atmosConfig, names, cfg.TerraformComponentType, false, false)
	require.NoError(t, err, "batched pull of multiple component.yaml-declared components must succeed")

	for _, name := range names {
		assert.FileExists(t, filepath.Join(dirs[name], "main.tf"), "component %q must have been pulled", name)
	}
}

// TestExecuteComponentVendorPullBatch_EmptyComponents_NoOp proves an empty component list is a
// pure no-op: no atmosConfig-dependent resolution is even attempted.
func TestExecuteComponentVendorPullBatch_EmptyComponents_NoOp(t *testing.T) {
	err := ExecuteComponentVendorPullBatch(&schema.AtmosConfiguration{}, nil, cfg.TerraformComponentType, false, false)
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

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"does-not-exist"}, cfg.TerraformComponentType, false, false)
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

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, false, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `component "vpc"`, "the failing component's name must be identifiable in a multi-component batch")
}

// TestExecuteComponentVendorPullBatch_AllMaterialized_NoOp proves a batch where every component is
// already vendored and unchanged filters down to zero packages and returns without error, instead
// of calling executeVendorModel with an empty list.
func TestExecuteComponentVendorPullBatch_AllMaterialized_NoOp(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "components", "terraform")
	require.NoError(t, os.MkdirAll(basePath, 0o755))

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))
	dir := writeLocalComponentVendorConfig(t, basePath, "vpc", sourceDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}

	require.NoError(t, ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, false, false))
	assert.FileExists(t, filepath.Join(dir, "main.tf"))

	// Add a new file to the source after the first pull; if the second call re-pulls instead of
	// skipping the already-materialized component, this file would show up too.
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "extra.tf"), []byte("# added later\n"), 0o644))

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, false, false)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "extra.tf"), "an already-materialized component must be skipped in a batch pull too")
}

// TestExecuteComponentVendorPullBatch_PropagatesMaterializationCheckError proves the batch entry
// point surfaces the vendor lock verification error (rather than swallowing it) when the existing
// vendor.lock.yaml can't be parsed.
func TestExecuteComponentVendorPullBatch_PropagatesMaterializationCheckError(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "components", "terraform")
	require.NoError(t, os.MkdirAll(basePath, 0o755))

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))
	writeLocalComponentVendorConfig(t, basePath, "vpc", sourceDir)

	// A malformed vendor.lock.yaml makes lockfile.Load fail when checking materialization.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "vendor.lock.yaml"), []byte("not: [valid yaml"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, false, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify vendor lock")
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

	packages, err := buildComponentVendorPackages(nil, spec, "vpc", componentPath)
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
	packages, err := buildComponentVendorPackages(nil, spec, "vpc", t.TempDir())
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

	packages, err := buildComponentVendorPackages(nil, spec, "vpc", t.TempDir())

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

	packages, err := buildComponentVendorPackages(nil, spec, "vpc", t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingMixinURI)
	assert.Nil(t, packages)
}

// TestFilterMaterializedComponentVendorPackages_PropagatesError proves a package whose
// componentPath cannot be related back to the project root (the vendor lock's target-containment
// check) surfaces lockfile.IsMaterialized's error, rather than silently treating it as pending.
func TestFilterMaterializedComponentVendorPackages_PropagatesError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	packages := []pkgComponentVendor{
		{name: "vpc", componentPath: t.TempDir(), pkgType: pkgTypeLocal, uri: t.TempDir(), IsComponent: true},
	}

	pending, err := filterMaterializedComponentVendorPackages(packages, atmosConfig)

	require.Error(t, err)
	assert.Nil(t, pending)
}

// TestExecuteComponentVendorInternal_PropagatesMaterializationCheckError proves
// ExecuteComponentVendorInternal surfaces the vendor lock verification error (rather than
// swallowing it or proceeding to install) when the component's path can't be related back to the
// project's BasePath.
func TestExecuteComponentVendorInternal_PropagatesMaterializationCheckError(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))

	// componentPath deliberately lives outside atmosConfig.BasePath's tree, so the vendor lock's
	// target-containment check cannot make it relative to the project root.
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	componentPath := t.TempDir()

	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{Uri: sourceDir},
	}

	err := ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, false, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "vendor lock target")
}

// TestExecuteComponentVendorInternal_AlreadyMaterialized_SkipsReinstall proves a second call for
// an unchanged, already-vendored component is a no-op that neither errors nor re-copies from the
// source. A file is added to the source directory between calls; if the second call re-ran the
// install (rather than skipping it), that new file would appear in componentPath too.
func TestExecuteComponentVendorInternal_AlreadyMaterialized_SkipsReinstall(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))

	basePath := t.TempDir()
	componentPath := filepath.Join(basePath, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{Uri: sourceDir},
	}

	require.NoError(t, ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, false, false))
	assert.FileExists(t, filepath.Join(componentPath, "main.tf"))

	// Add a new file to the source after the first install. If the second call re-installs
	// instead of skipping, this file would be copied into componentPath too.
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "extra.tf"), []byte("# added later\n"), 0o644))

	err := ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, false, false)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(componentPath, "extra.tf"), "an already-materialized component must be skipped, not re-copied from source")
}

// TestInstallMixin_LocalFileSource proves installMixin materializes a mixin whose pkgType is
// pkgTypeRemote (resolveMixinPackage never reclassifies a local file the way component sources
// do via handleLocalFileScheme) by fetching a plain local file path through go-getter's built-in
// local-file support - no network access involved - copying it to componentPath, and recording a
// vendor lock entry for it.
func TestInstallMixin_LocalFileSource(t *testing.T) {
	sourceFile := filepath.Join(t.TempDir(), "context.tf")
	require.NoError(t, os.WriteFile(sourceFile, []byte("# mixin\n"), 0o644))

	basePath := t.TempDir()
	componentPath := filepath.Join(basePath, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	pkg := &pkgComponentVendor{
		uri:           sourceFile,
		name:          "mixin " + sourceFile,
		componentPath: componentPath,
		pkgType:       pkgTypeRemote,
		IsMixins:      true,
		mixinFilename: "context.tf",
	}

	err := installMixin(pkg, atmosConfig)

	require.NoError(t, err)
	data, readErr := os.ReadFile(filepath.Join(componentPath, "context.tf"))
	require.NoError(t, readErr)
	assert.Equal(t, "# mixin\n", string(data))

	lock, err := lockfile.Load(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, lock.Artifacts, 1, "installMixin must record a vendor lock entry for the mixin")
}

// TestInstallMixin_LocalPkgType_MissingUri proves the pkgTypeLocal branch's guard against an empty
// uri returns a descriptive sentinel error instead of proceeding into an unimplemented code path.
func TestInstallMixin_LocalPkgType_MissingUri(t *testing.T) {
	pkg := &pkgComponentVendor{pkgType: pkgTypeLocal, IsMixins: true, name: "mixin"}

	err := installMixin(pkg, &schema.AtmosConfiguration{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMixinEmpty)
}

// TestInstallMixin_LocalPkgType_NotImplemented proves a non-empty local mixin uri surfaces the
// "not implemented" sentinel rather than silently no-op-ing.
func TestInstallMixin_LocalPkgType_NotImplemented(t *testing.T) {
	pkg := &pkgComponentVendor{pkgType: pkgTypeLocal, IsMixins: true, name: "mixin", uri: "some/local/path"}

	err := installMixin(pkg, &schema.AtmosConfiguration{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMixinNotImplemented)
}

// TestInstallMixin_RecordVendorLockErrorSurfaced proves a vendor-lock recording failure after a
// successful copy is surfaced as an error naming the mixin (rather than reporting success despite
// no receipt ever having been written).
func TestInstallMixin_RecordVendorLockErrorSurfaced(t *testing.T) {
	sourceFile := filepath.Join(t.TempDir(), "context.tf")
	require.NoError(t, os.WriteFile(sourceFile, []byte("# mixin\n"), 0o644))

	// componentPath deliberately lives outside atmosConfig.BasePath's tree, so
	// recordComponentVendorLock's lockfile.Replace can't relate it back to the project root, even
	// though the preceding copy to componentPath itself succeeds.
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	componentPath := t.TempDir()

	pkg := &pkgComponentVendor{
		uri:           sourceFile,
		name:          "mixin " + sourceFile,
		componentPath: componentPath,
		pkgType:       pkgTypeRemote,
		IsMixins:      true,
		mixinFilename: "context.tf",
	}

	err := installMixin(pkg, atmosConfig)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "record mixin vendor lock")
	// The copy itself must have succeeded before the lock recording failed.
	assert.FileExists(t, filepath.Join(componentPath, "context.tf"))
}

// TestInstallComponent_RecordVendorLockErrorSurfaced proves a vendor-lock recording failure after
// a successful component copy is surfaced as an error naming the component (rather than reporting
// success despite no receipt ever having been written).
func TestInstallComponent_RecordVendorLockErrorSurfaced(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))

	// componentPath deliberately lives outside atmosConfig.BasePath's tree, so
	// recordComponentVendorLock's lockfile.Replace can't relate it back to the project root, even
	// though the preceding copy to componentPath itself succeeds.
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	componentPath := t.TempDir()

	pkg := &pkgComponentVendor{
		uri:                 sourceDir,
		name:                "vpc",
		componentPath:       componentPath,
		pkgType:             pkgTypeLocal,
		sourceIsLocalFile:   false,
		IsComponent:         true,
		vendorComponentSpec: &schema.VendorComponentSpec{Source: schema.VendorComponentSource{Uri: sourceDir}},
	}

	err := installComponent(pkg, atmosConfig)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "record component vendor lock")
	// The copy itself must have succeeded before the lock recording failed.
	assert.FileExists(t, filepath.Join(componentPath, "main.tf"))
}
