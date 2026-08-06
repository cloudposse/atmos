package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	vendorcomponent "github.com/cloudposse/atmos/pkg/vendoring/component"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
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

	err := ExecuteComponentVendorInternal(&schema.AtmosConfiguration{BasePath: basePath}, spec, "vpc", componentPath, install.InstallOptions{})

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(componentPath, "main.tf"))
}

// TestExecuteComponentVendorInternal_PropagatesBuildError proves an unresolvable spec (missing
// source URI) fails before executeVendorModel is ever invoked.
func TestExecuteComponentVendorInternal_PropagatesBuildError(t *testing.T) {
	err := ExecuteComponentVendorInternal(&schema.AtmosConfiguration{}, &schema.VendorComponentSpec{}, "vpc", t.TempDir(), install.InstallOptions{})

	assert.Error(t, err)
}

// TestExecuteComponentVendorInternal_RefreshLock_ForcesReDownload proves refreshLock=true bypasses
// install.FilterPending, forcing an already-materialized component through executeVendorModel
// again - mirroring internal/exec/vendor_utils.go's ExecuteAtmosVendorInternal, which already used
// the same DryRun/RefreshLock-skips-FilterPending behavior for the vendor.yaml path. This is the
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
	require.NoError(t, ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, install.InstallOptions{}))
	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	require.Equal(t, "# v1\n", string(content))

	// Mutate the upstream source.
	require.NoError(t, os.WriteFile(sourceFile, []byte("# v2\n"), 0o644))

	// Second pull without refreshLock: already-materialized target, must be skipped.
	require.NoError(t, ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, install.InstallOptions{}))
	content, err = os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "# v1\n", string(content), "without refreshLock, an already-materialized component must not be re-pulled")

	// Third pull with refreshLock: must bypass the materialization filter and re-pull.
	require.NoError(t, ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, install.InstallOptions{RefreshLock: true}))
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

	err := ExecuteComponentVendorPullBatch(atmosConfig, names, cfg.TerraformComponentType, install.InstallOptions{})
	require.NoError(t, err, "batched pull of multiple component.yaml-declared components must succeed")

	for _, name := range names {
		assert.FileExists(t, filepath.Join(dirs[name], "main.tf"), "component %q must have been pulled", name)
	}
}

// TestExecuteComponentVendorPullBatch_EmptyComponents_NoOp proves an empty component list is a
// pure no-op: no atmosConfig-dependent resolution is even attempted.
func TestExecuteComponentVendorPullBatch_EmptyComponents_NoOp(t *testing.T) {
	err := ExecuteComponentVendorPullBatch(&schema.AtmosConfiguration{}, nil, cfg.TerraformComponentType, install.InstallOptions{})
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

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"does-not-exist"}, cfg.TerraformComponentType, install.InstallOptions{})
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

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, install.InstallOptions{})

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

	require.NoError(t, ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, install.InstallOptions{}))
	assert.FileExists(t, filepath.Join(dir, "main.tf"))

	// Add a new file to the source after the first pull; if the second call re-pulls instead of
	// skipping the already-materialized component, this file would show up too.
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "extra.tf"), []byte("# added later\n"), 0o644))

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, install.InstallOptions{})
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

	err := ExecuteComponentVendorPullBatch(atmosConfig, []string{"vpc"}, cfg.TerraformComponentType, install.InstallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify vendor lock")
}

// TestBuildComponentVendorPackages_WiresProcessTmpl proves internal/exec wires its own ProcessTmpl
// (Sprig/gomplate/Atmos template functions) into pkg/vendoring/component.BuildVendorPackages as
// its TemplateFunc, and that the resulting package list has the same shape
// ExecuteComponentVendorInternal used to build inline: the component package first (IsMixin
// false), followed by one entry per mixin (IsMixin true). The pure package-building logic itself
// (URI templating, mixin resolution, error propagation) is covered directly in
// pkg/vendoring/component's own tests.
func TestBuildComponentVendorPackages_WiresProcessTmpl(t *testing.T) {
	componentPath := t.TempDir()

	// A local mixin file that already exists at the resolved path is skipped by
	// processComponentMixins (it only fetches mixins not already present), so point the mixin uri
	// at a non-existent local file to ensure it produces a package entry.
	spec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			Uri:     "github.com/cloudposse/terraform-null-label.git//?ref={{.Version}}",
			Version: "0.1.0",
		},
		Mixins: []schema.VendorComponentMixins{
			{Uri: "github.com/cloudposse/terraform-null-label.git//exports/context.tf?ref=0.1.0", Filename: "context.tf"},
		},
	}

	packages, err := vendorcomponent.BuildVendorPackages(vendorcomponent.BuildPackagesOptions{
		VendorComponentSpec: spec,
		Component:           "vpc",
		ComponentPath:       componentPath,
		TemplateFunc:        ProcessTmpl,
	})
	require.NoError(t, err)
	require.Len(t, packages, 2)

	assert.Equal(t, "vpc", packages[0].Name)
	assert.False(t, packages[0].IsMixin())
	assert.Contains(t, packages[0].URI(), "ref=0.1.0", "ProcessTmpl must have templated {{.Version}} into the source uri")

	assert.True(t, packages[1].IsMixin())
	assert.Equal(t, "context.tf", packages[1].MixinFilename())
}

// TestFilterMaterializedComponentVendorPackages_PropagatesError proves a package whose
// componentPath cannot be related back to the project root (the vendor lock's target-containment
// check) surfaces install.FilterPending's error, rather than silently treating it as pending.
func TestFilterMaterializedComponentVendorPackages_PropagatesError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	packages := []install.VendorPackage{
		install.NewComponentVendorPackage(&install.ComponentPackageParams{
			Name: "vpc", ComponentPath: t.TempDir(), PkgType: install.PkgTypeLocal, URI: t.TempDir(),
		}),
	}

	pending, err := install.FilterPending(atmosConfig, packages, install.InstallOptions{})

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

	err := ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, install.InstallOptions{})

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

	require.NoError(t, ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, install.InstallOptions{}))
	assert.FileExists(t, filepath.Join(componentPath, "main.tf"))

	// Add a new file to the source after the first install. If the second call re-installs
	// instead of skipping, this file would be copied into componentPath too.
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "extra.tf"), []byte("# added later\n"), 0o644))

	err := ExecuteComponentVendorInternal(atmosConfig, spec, "vpc", componentPath, install.InstallOptions{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(componentPath, "extra.tf"), "an already-materialized component must be skipped, not re-copied from source")
}

// TestResolveVendorComponentName proves the stack-declared component name is used as-is unless the
// component's own data declares a metadata.component override (e.g. multiple stack instances of
// the same underlying component vendor from the same directory).
func TestResolveVendorComponentName(t *testing.T) {
	tests := []struct {
		name     string
		compName string
		data     any
		want     string
	}{
		{"no override uses the stack-declared name", "vpc-flow-logs", map[string]any{}, "vpc-flow-logs"},
		{"non-map data uses the stack-declared name", "vpc-flow-logs", "not-a-map", "vpc-flow-logs"},
		{"no metadata section uses the stack-declared name", "vpc-flow-logs", map[string]any{"vars": map[string]any{}}, "vpc-flow-logs"},
		{
			"metadata.component override is used", "vpc-flow-logs",
			map[string]any{"metadata": map[string]any{"component": "vpc"}},
			"vpc",
		},
		{
			"empty metadata.component override falls back to the stack-declared name", "vpc-flow-logs",
			map[string]any{"metadata": map[string]any{"component": ""}},
			"vpc-flow-logs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveVendorComponentName(tt.compName, tt.data))
		})
	}
}

// TestComponentHasVendorManifest proves the existence check --stack relies on to silently skip
// stack components with no component.yaml of their own, distinguishing that "no" case from a real
// error (an unsupported component type).
func TestComponentHasVendorManifest(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}
	terraformBasePath := filepath.Join(basePath, "components", "terraform")

	t.Run("component with a component.yaml", func(t *testing.T) {
		writeLocalComponentVendorConfig(t, terraformBasePath, "vpc", t.TempDir())

		has, err := componentHasVendorManifest(atmosConfig, "vpc", cfg.TerraformComponentType)

		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("component directory exists but has no component.yaml", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(terraformBasePath, "eks"), 0o755))

		has, err := componentHasVendorManifest(atmosConfig, "eks", cfg.TerraformComponentType)

		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("component directory does not exist", func(t *testing.T) {
		has, err := componentHasVendorManifest(atmosConfig, "does-not-exist", cfg.TerraformComponentType)

		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("unsupported component type is a real error, not a silent skip", func(t *testing.T) {
		_, err := componentHasVendorManifest(atmosConfig, "vpc", "bogus-type")

		require.Error(t, err)
	})
}

// stackComponentEntry builds one component's map[string]any entry as ExecuteDescribeStacks would
// return it, optionally overriding metadata.component and/or marking it abstract.
func stackComponentEntry(metadataComponent string, abstract bool) map[string]any {
	metadata := map[string]any{}
	if metadataComponent != "" {
		metadata["component"] = metadataComponent
	}
	if abstract {
		metadata["type"] = "abstract"
	}
	return map[string]any{"metadata": metadata}
}

// TestResolveStackVendorComponents proves the full resolution pipeline --stack relies on: abstract
// components are skipped, components without a component.yaml are silently skipped, a
// metadata.component override is honored and deduped against the component it resolves to, and
// components are grouped by type for ExecuteComponentVendorPullBatch.
func TestResolveStackVendorComponents(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
			Helmfile:  schema.Helmfile{BasePath: "components/helmfile"},
		},
	}
	terraformBasePath := filepath.Join(basePath, "components", "terraform")
	helmfileBasePath := filepath.Join(basePath, "components", "helmfile")

	// "vpc" and "app" are real, vendorable components.
	writeLocalComponentVendorConfig(t, terraformBasePath, "vpc", t.TempDir())
	writeLocalComponentVendorConfig(t, helmfileBasePath, "app", t.TempDir())
	// "eks" has a directory but no component.yaml - not every stack component vendors this way.
	require.NoError(t, os.MkdirAll(filepath.Join(terraformBasePath, "eks"), 0o755))

	stacksMap := map[string]any{
		"dev-us-west-2": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc":                stackComponentEntry("", false),
					"vpc-flow-logs":      stackComponentEntry("vpc", false), // dedups against "vpc".
					"eks":                stackComponentEntry("", false),    // no component.yaml -> skipped.
					"abstract-component": stackComponentEntry("", true),     // abstract -> skipped.
				},
				cfg.HelmfileComponentType: map[string]any{
					"app": stackComponentEntry("", false),
				},
			},
		},
	}

	got, err := resolveStackVendorComponents(atmosConfig, stacksMap, []string{cfg.TerraformComponentType, cfg.HelmfileComponentType})

	require.NoError(t, err)
	assert.Equal(t, []string{"vpc"}, got[cfg.TerraformComponentType])
	assert.Equal(t, []string{"app"}, got[cfg.HelmfileComponentType])
	assert.Len(t, got, 2, "eks (no manifest) and abstract-component (abstract) must not appear in any group")
}

// buildHandleStackVendorFixture creates a temp atmos project with two stacks: "dev", declaring a
// vendorable "vpc" component (its own component.yaml, local source) and a non-vendorable "eks"
// component (no component.yaml); and "prod", declaring its own "vpc-prod" component. Changes the
// process working directory to the fixture root and returns the initialized AtmosConfiguration,
// mirroring describe_stacks_test.go's buildDescribeStacksDegradationFixture.
func buildHandleStackVendorFixture(t *testing.T) schema.AtmosConfiguration {
	t.Helper()

	tmpDir := t.TempDir()

	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	vpcDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "main.tf"), []byte(""), 0o644))
	vpcSource := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(vpcSource, "main.tf"), []byte("# vpc\n"), 0o644))
	writeLocalComponentVendorConfig(t, filepath.Join(tmpDir, "components", "terraform"), "vpc", vpcSource)

	eksDir := filepath.Join(tmpDir, "components", "terraform", "eks")
	require.NoError(t, os.MkdirAll(eksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(eksDir, "main.tf"), []byte(""), 0o644))

	vpcProdDir := filepath.Join(tmpDir, "components", "terraform", "vpc-prod")
	require.NoError(t, os.MkdirAll(vpcProdDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vpcProdDir, "main.tf"), []byte(""), 0o644))
	vpcProdSource := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(vpcProdSource, "main.tf"), []byte("# vpc-prod\n"), 0o644))
	writeLocalComponentVendorConfig(t, filepath.Join(tmpDir, "components", "terraform"), "vpc-prod", vpcProdSource)

	devStack := "components:\n  terraform:\n    vpc:\n      vars: {}\n    eks:\n      vars: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte(devStack), 0o644))

	prodStack := "components:\n  terraform:\n    vpc-prod:\n      vars: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "prod.yaml"), []byte(prodStack), 0o644))

	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// TestHandleStackVendor_PullsOnlyTheGivenStacksComponents proves the end-to-end "atmos vendor pull
// --stack dev" flow: it resolves "dev"'s components via ExecuteDescribeStacks, pulls "vpc" (which
// declares its own component.yaml), silently skips "eks" (no component.yaml), and never touches
// "prod"'s "vpc-prod" even though it has a component.yaml of its own -- --stack scopes strictly to
// the named stack.
func TestHandleStackVendor_PullsOnlyTheGivenStacksComponents(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "dev"})

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(atmosConfig.BasePath, "components", "terraform", "vpc", "main.tf"))
	content, readErr := os.ReadFile(filepath.Join(atmosConfig.BasePath, "components", "terraform", "vpc", "main.tf"))
	require.NoError(t, readErr)
	assert.Equal(t, "# vpc\n", string(content), "vpc's component.yaml source must have been pulled")

	prodContent, readErr := os.ReadFile(filepath.Join(atmosConfig.BasePath, "components", "terraform", "vpc-prod", "main.tf"))
	require.NoError(t, readErr)
	assert.Empty(t, string(prodContent), "prod's vpc-prod must not be touched by a --stack dev pull")
}

// TestHandleStackVendor_UnknownStack proves an unresolvable stack name fails loudly with
// errUtils.ErrStackNotFound rather than silently succeeding as a no-op.
func TestHandleStackVendor_UnknownStack(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "does-not-exist"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}
