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
)

// buildHandleStackVendorFixture creates a temp atmos project with two stacks: "dev", declaring a
// vendorable "vpc" component (its own component.yaml, local source, metadata.labels.tier: "1") and
// a non-vendorable "eks" component (no component.yaml, no labels); and "prod", declaring its own
// "vpc-prod" component (metadata.labels.tier: "2"). Changes the process working directory to the
// fixture root and returns the initialized AtmosConfiguration, mirroring describe_stacks_test.go's
// buildDescribeStacksDegradationFixture.
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

	devStack := "components:\n  terraform:\n    vpc:\n      metadata:\n        labels:\n          tier: \"1\"\n      vars: {}\n    eks:\n      vars: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte(devStack), 0o644))

	prodStack := "components:\n  terraform:\n    vpc-prod:\n      metadata:\n        labels:\n          tier: \"2\"\n      vars: {}\n"
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

// TestHandleStackVendor_ComponentAlsoInVendorYaml_WarnsAboutDivergentSource is the direct
// regression test for the reported bug: when a --stack-resolved component has BOTH its own
// component.yaml (which --stack/--labels always installs from) AND a vendor.yaml entry (which
// --component/bare --tags would install from instead), the two can silently disagree on content
// with zero indication to the user -- "atmos vendor pull -c vpc" and "atmos vendor pull --stack dev"
// installed different content into the identical target directory, both exiting 0. --stack must now
// warn (not error -- this is documented, intentional precedence, not something to reject) whenever
// a resolved component also has a vendor.yaml entry, so the divergence risk is surfaced.
func TestHandleStackVendor_ComponentAlsoInVendorYaml_WarnsAboutDivergentSource(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)
	require.NoError(t, os.WriteFile("vendor.yaml", []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock-vpc:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/vpc"]
`), 0o644))

	stderr, cleanup := setupVendorModelTestUI(t)
	defer cleanup()

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "dev"})

	require.NoError(t, err, "the divergence must only warn, never block a --stack pull that would otherwise succeed")
	assert.FileExists(t, filepath.Join(atmosConfig.BasePath, "components", "terraform", "vpc", "main.tf"),
		"vpc must still install via its own component.yaml, unaffected by the warning")
	assert.Contains(t, stderr.String(), "vpc", "the warning must name the affected component")
	assert.Contains(t, stderr.String(), "vendor.yaml", "the warning must explain the divergence risk against vendor.yaml")
}

// TestHandleStackVendor_UnknownStack proves an unresolvable stack name fails loudly with
// errUtils.ErrInvalidArgumentError rather than silently succeeding as a no-op -- the same sentinel
// and wording "atmos vendor update --stack" already uses for its own zero-match case (see
// cmd/vendor/update.go), since pull and update share an identical stack/labels-only selector
// vocabulary.
func TestHandleStackVendor_UnknownStack(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "does-not-exist"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

// buildHandleStackVendorMalformedStackFixture creates a temp atmos project with a single stack file
// ("stacks/dev.yaml") that is not valid YAML, and chdirs into it -- used to force
// ExecuteDescribeStacksScoped to fail outright (a genuine describe-stacks error), as opposed to
// TestHandleStackVendor_UnknownStack/TestHandleStackVendor_LabelsMatchingNothingErrors' "resolved
// fine but matched nothing" case.
func buildHandleStackVendorMalformedStackFixture(t *testing.T) schema.AtmosConfiguration {
	t.Helper()

	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte("components: [\n"), 0o644))

	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// processStacks=true so atmosConfig.StackConfigFilesAbsolutePaths is populated with
	// stacks/dev.yaml -- otherwise ExecuteDescribeStacksScoped would have no stack files to
	// process at all, and would resolve to zero stacks (a "no match", not a parse error).
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// TestHandleStackVendor_NamedStackDescribeStacksErrorPropagates proves a genuine
// ExecuteDescribeStacksScoped failure (malformed stack YAML, not just an unmatched stack name)
// surfaces as handleStackVendor's own wrapped "failed to describe stack %q" error when --stack is
// given.
func TestHandleStackVendor_NamedStackDescribeStacksErrorPropagates(t *testing.T) {
	atmosConfig := buildHandleStackVendorMalformedStackFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "dev"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrExecuteDescribeStacks)
	assert.Contains(t, err.Error(), `failed to describe stack "dev"`)
}

// TestHandleStackVendor_LabelsOnlyDescribeStacksErrorPropagates proves the same
// ExecuteDescribeStacksScoped failure surfaces through handleStackVendor's other wording (the
// --labels-only, no --stack branch) when no --stack narrows which stack is being described.
func TestHandleStackVendor_LabelsOnlyDescribeStacksErrorPropagates(t *testing.T) {
	atmosConfig := buildHandleStackVendorMalformedStackFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Labels: map[string]string{"tier": "1"}})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrExecuteDescribeStacks)
	assert.Contains(t, err.Error(), "failed to describe stacks for the given --labels selector")
}

// TestHandleStackVendor_LabelsMatchingNothingErrors proves the labels-only zero-match branch (no
// --stack, --labels matching no component in any stack) errors with the same sentinel/wording as
// the named-stack case above, rather than silently succeeding as a no-op.
func TestHandleStackVendor_LabelsMatchingNothingErrors(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Labels: map[string]string{"tier": "does-not-exist"}})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

// buildHandleStackVendorTypeFilterFixture creates a temp atmos project with a single "dev" stack
// declaring two vendorable components of different types: a terraform "vpc" and a helmfile "app",
// each with its own component.yaml/local source. Used to prove handleStackVendor's --type filtering
// (flg.TypeChanged) narrows the --stack path to a single component type, mirroring
// buildHandleStackVendorFixture's setup style but with atmos.yaml also configuring a helmfile base
// path.
func buildHandleStackVendorTypeFilterFixture(t *testing.T) schema.AtmosConfiguration {
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

	appDir := filepath.Join(tmpDir, "components", "helmfile", "app")
	require.NoError(t, os.MkdirAll(appDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "helmfile.yaml"), []byte(""), 0o644))
	appSource := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(appSource, "helmfile.yaml"), []byte("# app\n"), 0o644))
	writeLocalComponentVendorConfig(t, filepath.Join(tmpDir, "components", "helmfile"), "app", appSource)

	devStack := "components:\n  terraform:\n    vpc:\n      vars: {}\n  helmfile:\n    app:\n      vars: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte(devStack), 0o644))

	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n  helmfile:\n    base_path: components/helmfile\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// TestHandleStackVendor_TypeChanged_Stack_OnlyMatchingTypePulled proves flg.TypeChanged threads
// through handleStackVendor's --stack path the same way TestExecuteVendorPullCommand_Everything_
// NoVendorFile_TypeFilter proves it for the --everything/sweep path: an explicit
// VendorFlags{ComponentType: helmfile, TypeChanged: true} must narrow componentTypes to only
// "helmfile", so the stack's terraform "vpc" is never even considered/pulled, while the helmfile
// "app" is.
func TestHandleStackVendor_TypeChanged_Stack_OnlyMatchingTypePulled(t *testing.T) {
	atmosConfig := buildHandleStackVendorTypeFilterFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "dev", ComponentType: cfg.HelmfileComponentType, TypeChanged: true})

	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(atmosConfig.BasePath, "components", "helmfile", "app", "helmfile.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, "# app\n", string(content), "helmfile app must be pulled when --type helmfile is explicit")

	vpcContent, readErr := os.ReadFile(filepath.Join(atmosConfig.BasePath, "components", "terraform", "vpc", "main.tf"))
	require.NoError(t, readErr)
	assert.Empty(t, string(vpcContent), "terraform vpc must not be pulled when --stack is combined with an explicit --type helmfile")
}

// initMinimalAtmosProjectFixture writes a minimal atmos.yaml (terraform components under
// "components/terraform", stacks under "stacks") at tmpDir, chdirs the test into it, and returns
// the initialized AtmosConfiguration (processStacks=true). Shared tail for fixture builders that
// only differ in what they put under stacks/ and components/ before calling this -- e.g.
// buildHandleStackVendorNoManifestFixture below and describe_stacks_test.go's
// buildDescribeStacksDegradationFixture.
func initMinimalAtmosProjectFixture(t *testing.T, tmpDir string) schema.AtmosConfiguration {
	t.Helper()

	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// buildHandleStackVendorNoManifestFixture creates a temp atmos project with a single "dev" stack
// declaring one component ("eks") that has a directory but no component.yaml of its own. Used to
// prove handleStackVendor's no-manifest no-op: the stack resolves fine, but nothing in it has a
// manifest to vendor.
func buildHandleStackVendorNoManifestFixture(t *testing.T) schema.AtmosConfiguration {
	t.Helper()

	tmpDir := t.TempDir()

	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	eksDir := filepath.Join(tmpDir, "components", "terraform", "eks")
	require.NoError(t, os.MkdirAll(eksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(eksDir, "main.tf"), []byte(""), 0o644))

	devStack := "components:\n  terraform:\n    eks:\n      vars: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte(devStack), 0o644))

	return initMinimalAtmosProjectFixture(t, tmpDir)
}

// TestHandleStackVendor_NoManifests_NoOp proves handleStackVendor's "the stack resolved components,
// but none has its own component.yaml" branch: the stack lookup itself succeeds (stacksMap is
// non-empty), but resolveAndFilterStackComponents comes back empty, and handleStackVendor must
// return nil without touching any files, rather than erroring.
func TestHandleStackVendor_NoManifests_NoOp(t *testing.T) {
	atmosConfig := buildHandleStackVendorNoManifestFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "dev"})

	require.NoError(t, err)

	entries, readErr := os.ReadDir(filepath.Join(atmosConfig.BasePath, "components", "terraform", "eks"))
	require.NoError(t, readErr)
	assert.Len(t, entries, 1, "eks's directory must contain only its own pre-existing main.tf, nothing pulled")
}

// TestHandleStackVendor_MalformedVendorYaml_PropagatesError proves filterStackComponentsByTags
// surfaces a genuine vendor.yaml parse error (rather than treating it as "no vendor.yaml" or
// swallowing it into the "no match" sentinel) when --stack/--tags are combined and the repo's
// vendor.yaml can't be parsed. Mirrors TestExecuteComponentVendorPullBatch_
// PropagatesMaterializationCheckError's malformed-YAML technique.
func TestHandleStackVendor_MalformedVendorYaml_PropagatesError(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	require.NoError(t, os.WriteFile("vendor.yaml", []byte("not: [valid yaml"), 0o644))

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "dev", Tags: []string{"networking"}})

	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrInvalidArgumentError, "a malformed vendor.yaml must surface a genuine parse error, not the 'no match' sentinel")
	// NotErrorIs alone passes for ANY unrelated error (e.g. a fixture setup failure), not just the
	// genuine YAML parse error this test is meant to prove -- pin down that the error actually names
	// the manifest that failed to parse.
	assert.Contains(t, err.Error(), "vendor.yaml", "the error must name the manifest that failed to parse")
}

// TestResolveAndFilterStackComponents_EmptyComponentsWithTags_ReturnsEmptyNoError proves the early
// return's left-hand OR condition (len(componentsByType) == 0) independently: when --stack/--labels
// resolves to zero components in the first place, a non-empty --tags must not be treated as a "tags
// filtered everything out" error -- there was nothing for it to filter. This is distinct from
// TestExecuteVendorPullCommand_StackAndTagsExcludesUntaggedComponents, which covers the case where
// resolution finds a non-empty set that tags then narrows to empty (an explicit error there).
func TestResolveAndFilterStackComponents_EmptyComponentsWithTags_ReturnsEmptyNoError(t *testing.T) {
	got, err := resolveAndFilterStackComponents(&schema.AtmosConfiguration{}, map[string]any{}, []string{cfg.TerraformComponentType}, []string{"networking"})

	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestResolveAndFilterStackComponents_PropagatesComponentHasVendorManifestError proves a genuine
// componentHasVendorManifest error (as opposed to the expected/tolerated
// errUtils.ErrComponentDirNotFound, which resolveStackVendorComponents silently skips) propagates
// through both resolveStackVendorComponents' own "if err != nil" check and
// resolveAndFilterStackComponents' wrapping check around it, rather than being swallowed. An
// unsupported component type (a stack section keyed by a name that isn't terraform/helmfile/packer)
// is used to force vendoring.ResolveComponentPath's ErrUnsupportedComponentType, the only "generic,
// non-NotFound" error this path can realistically produce end to end.
func TestResolveAndFilterStackComponents_PropagatesComponentHasVendorManifestError(t *testing.T) {
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"bogus-type": map[string]any{
					"vpc": stackComponentEntry("", false),
				},
			},
		},
	}

	got, err := resolveAndFilterStackComponents(&schema.AtmosConfiguration{}, stacksMap, []string{"bogus-type"}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedComponentType)
	assert.Nil(t, got)
}

// TestWalkStackVendorComponents_SkipsMalformedEntries proves walkStackVendorComponents and
// stackComponentsSection tolerate malformed stacksMap shapes without panicking or erroring: a stack
// entry that isn't a map, a stack with no "components" key, and a component-type section that isn't
// a map are all silently skipped, while a normal, well-formed entry alongside them is still resolved.
func TestWalkStackVendorComponents_SkipsMalformedEntries(t *testing.T) {
	stacksMap := map[string]any{
		"weird": "not-a-map",
		"no-components-key": map[string]any{
			"vars": map[string]any{},
		},
		"empty-components": map[string]any{
			"components": map[string]any{},
		},
		"bad-type-section": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: "not-a-map",
			},
		},
		"dev": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": stackComponentEntry("", false),
				},
			},
		},
	}

	got := walkStackVendorComponents(stacksMap, []string{cfg.TerraformComponentType})

	require.Len(t, got, 1, "only the well-formed 'dev' stack's vpc must be resolved")
	assert.Equal(t, stackVendorComponent{ComponentType: cfg.TerraformComponentType, Name: "vpc"}, got[0])
}

// buildHandleStackVendorMixedResultFixture creates a temp atmos project with a single "dev" stack
// declaring two vendorable components of different types: a terraform "vpc" with a real, pullable
// local source, and a helmfile "app" whose component.yaml is well-formed but whose declared source
// points at a local path that doesn't exist -- so pulling it fails only inside
// ExecuteComponentVendorPullBatch, mirroring writeSweepComponentManifestFixture's use in
// TestExecuteVendorPullCommand_Everything_NoVendorFile_OneTypeGroupFails_OtherStillPulled
// (vendor_pull_sweep_test.go), but exercised via handleStackVendor/pullStackComponentsByType instead
// of handleVendorPullSweep.
func buildHandleStackVendorMixedResultFixture(t *testing.T) schema.AtmosConfiguration {
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

	appDir := filepath.Join(tmpDir, "components", "helmfile", "app")
	require.NoError(t, os.MkdirAll(appDir, 0o755))
	missingSource := filepath.Join(t.TempDir(), "does-not-exist")
	writeLocalComponentVendorConfig(t, filepath.Join(tmpDir, "components", "helmfile"), "app", missingSource)

	devStack := "components:\n  terraform:\n    vpc:\n      vars: {}\n  helmfile:\n    app:\n      vars: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte(devStack), 0o644))

	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n  helmfile:\n    base_path: components/helmfile\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// TestHandleStackVendor_OneTypeGroupFails_OtherStillPulled proves pullStackComponentsByType's
// per-type errors.Join behavior with two real component types (not one): the helmfile "app"'s
// unresolvable source must not prevent the terraform "vpc" from being pulled, and the helmfile
// failure must still surface as an error rather than being silently swallowed.
func TestHandleStackVendor_OneTypeGroupFails_OtherStillPulled(t *testing.T) {
	atmosConfig := buildHandleStackVendorMixedResultFixture(t)

	err := handleStackVendor(&atmosConfig, &VendorFlags{Stack: "dev"})

	require.Error(t, err, "the helmfile component's unresolvable source must surface as an error")

	content, readErr := os.ReadFile(filepath.Join(atmosConfig.BasePath, "components", "terraform", "vpc", "main.tf"))
	require.NoError(t, readErr)
	assert.Equal(t, "# vpc\n", string(content), "the valid terraform component must still be pulled despite the helmfile component failing")
}
