package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// writeSweepComponentManifestFixture writes a component.yaml for componentName under
// <repoRoot>/components/<componentType>/<componentName>/component.yaml, pointing its source.uri at
// a local sourceDir (absolute path) so ExecuteComponentVendorPullBatch resolves it as a local copy
// with no network access - mirrors cmd/vendor/vendor_test.go's writeLocalComponentManifestFixture
// (unexported in a different package, so duplicated here) and this file's own sibling
// writeLocalComponentVendorConfig in vendor_component_utils_test.go (which targets a single fixed
// "terraform" type rather than a parameterized one, needed here for the mixed-type sweep tests).
func writeSweepComponentManifestFixture(t *testing.T, repoRoot, componentType, componentName, sourceDir string) string {
	t.Helper()
	componentDir := filepath.Join(repoRoot, "components", componentType, componentName)
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	file := filepath.Join(componentDir, "component.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`apiVersion: atmos/v1
kind: ComponentVendorConfig
spec:
  source:
    uri: "`+filepath.ToSlash(sourceDir)+`"
    version: "v0.1.0"
`), 0o644))
	return componentDir
}

// newVendorPullSweepTestCmd builds a throwaway *cobra.Command carrying exactly the flags
// cmd/vendor/vendor.go registers on `atmos vendor pull`, plus the global flags
// ProcessCommandLineArgs/cfg.InitCliConfig read directly off cmd.Flags() - mirrors
// TestVendorPullFullWorkflow's pattern in vendor_pull_integration_test.go.
func newVendorPullSweepTestCmd() *cobra.Command {
	cmd := newTestCommandWithGlobalFlags("pull")
	flags := cmd.Flags()
	flags.StringP("component", "c", "", "")
	flags.StringP("stack", "s", "", "")
	flags.StringP("type", "t", "terraform", "")
	flags.Bool("dry-run", false, "")
	flags.String("tags", "", "")
	flags.String("labels", "", "")
	flags.Bool("everything", false, "")
	flags.Bool("refresh-lock", false, "")
	return cmd
}

// TestExecuteVendorPullCommand_Everything_NoVendorFile_SweepsAllComponentManifests is the direct
// regression test for the reported bug: "atmos vendor pull --everything" (and bare "atmos vendor
// pull", which defaults --everything to true via setDefaultEverythingFlag) previously hard-failed
// with ErrVendorConfigNotExist in any repo with no root vendor.yaml, even when every component was
// declared via its own component.yaml. Both the implicit-default and explicit-flag paths must now
// succeed and materialize every discovered component.
func TestExecuteVendorPullCommand_Everything_NoVendorFile_SweepsAllComponentManifests(t *testing.T) {
	setup := func(t *testing.T) []string {
		repoRoot := t.TempDir()
		t.Chdir(repoRoot)

		src1 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(src1, "main.tf"), []byte("# one\n"), 0o644))
		dir1 := writeSweepComponentManifestFixture(t, repoRoot, "terraform", "comp-one", src1)

		src2 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(src2, "main.tf"), []byte("# two\n"), 0o644))
		dir2 := writeSweepComponentManifestFixture(t, repoRoot, "terraform", "comp-two", src2)

		return []string{filepath.Join(dir1, "main.tf"), filepath.Join(dir2, "main.tf")}
	}

	t.Run("bare vendor pull defaults --everything to true", func(t *testing.T) {
		files := setup(t)
		cmd := newVendorPullSweepTestCmd()

		err := ExecuteVendorPullCommand(cmd, nil)

		require.NoError(t, err)
		for _, f := range files {
			assert.FileExists(t, f)
		}
	})

	t.Run("--everything set explicitly", func(t *testing.T) {
		files := setup(t)
		cmd := newVendorPullSweepTestCmd()
		require.NoError(t, cmd.Flags().Set("everything", "true"))

		err := ExecuteVendorPullCommand(cmd, nil)

		require.NoError(t, err)
		for _, f := range files {
			assert.FileExists(t, f)
		}
	})
}

// TestExecuteVendorPullCommand_Everything_NoVendorFile_TypeFilter proves flg.TypeChanged threads
// correctly: an explicit "--type helmfile" must sweep only the helmfile base path, not silently
// fall back to sweeping just the default "terraform" type (which flg.ComponentType alone, without
// TypeChanged, could not distinguish from "no --type given at all").
func TestExecuteVendorPullCommand_Everything_NoVendorFile_TypeFilter(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)

	tfSrc := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tfSrc, "main.tf"), []byte("# tf\n"), 0o644))
	tfDir := writeSweepComponentManifestFixture(t, repoRoot, "terraform", "tf-comp", tfSrc)

	hfSrc := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(hfSrc, "helmfile.yaml"), []byte("# hf\n"), 0o644))
	hfDir := writeSweepComponentManifestFixture(t, repoRoot, "helmfile", "hf-comp", hfSrc)

	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, cmd.Flags().Set("type", "helmfile"))

	err := ExecuteVendorPullCommand(cmd, nil)

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(hfDir, "helmfile.yaml"), "explicit --type helmfile must pull the helmfile component")
	assert.NoFileExists(t, filepath.Join(tfDir, "main.tf"), "explicit --type helmfile must not sweep the terraform component")
}

// TestExecuteVendorPullCommand_Everything_NoVendorFile_NoManifestsFound proves the sweep still
// errors (ErrNoVendorSourcesFound), rather than silently succeeding as a no-op, when there is
// neither a vendor.yaml nor any component.yaml anywhere - a wrong --chdir/cwd invocation of
// --everything should fail loudly, not silently.
func TestExecuteVendorPullCommand_Everything_NoVendorFile_NoManifestsFound(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)

	cmd := newVendorPullSweepTestCmd()

	err := ExecuteVendorPullCommand(cmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoVendorSourcesFound)
}

// TestExecuteVendorPullCommand_Everything_NoVendorFile_DryRun proves flg.DryRun threads through to
// ExecuteComponentVendorPullBatch: a dry-run sweep must succeed without materializing any files.
func TestExecuteVendorPullCommand_Everything_NoVendorFile_DryRun(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.tf"), []byte("# dry\n"), 0o644))
	dir := writeSweepComponentManifestFixture(t, repoRoot, "terraform", "dry-comp", src)

	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))

	err := ExecuteVendorPullCommand(cmd, nil)

	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "main.tf"), "dry-run must not materialize files")
}

// TestExecuteVendorPullCommand_Everything_NoVendorFile_OneTypeGroupFails_OtherStillPulled proves
// handleVendorPullSweep's cross-type-group errors.Join behavior: a component whose source can't be
// pulled in one component type must not prevent a valid component in a different type from being
// pulled, and the failing one's error must still surface (not be silently swallowed).
//
// The failure must be injected at pull time, not at discovery time: DiscoverAllComponentManifests
// (pkg/vendoring/resolve.go) parses every component.yaml across every type before
// handleVendorPullSweep ever groups or batches anything, and a manifest that fails to parse aborts
// that discovery call entirely (for every type, not just its own) - so a malformed component.yaml
// can never reach the per-type-group errors.Join loop this test targets. A syntactically valid
// manifest whose declared source can't actually be resolved (a nonexistent local path) discovers
// fine and fails only once ExecuteComponentVendorPullBatch tries to pull it.
func TestExecuteVendorPullCommand_Everything_NoVendorFile_OneTypeGroupFails_OtherStillPulled(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)

	// Valid terraform component with a real, pullable local source.
	tfSrc := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tfSrc, "main.tf"), []byte("# tf\n"), 0o644))
	tfDir := writeSweepComponentManifestFixture(t, repoRoot, "terraform", "tf-comp", tfSrc)

	// Helmfile component.yaml is well-formed (discovers fine) but its source points at a local path
	// that doesn't exist, so pulling it fails inside ExecuteComponentVendorPullBatch for the
	// helmfile type-group only.
	missingSrc := filepath.Join(t.TempDir(), "does-not-exist")
	hfDir := writeSweepComponentManifestFixture(t, repoRoot, "helmfile", "hf-comp", missingSrc)

	cmd := newVendorPullSweepTestCmd()

	err := ExecuteVendorPullCommand(cmd, nil)

	require.Error(t, err, "the helmfile component's unresolvable source must surface as an error")
	assert.FileExists(t, filepath.Join(tfDir, "main.tf"), "the valid terraform group must still be pulled despite the helmfile group failing")
	entries, readErr := os.ReadDir(hfDir)
	require.NoError(t, readErr)
	assert.Len(t, entries, 1, "the failing helmfile component's directory must contain only its own component.yaml, nothing materialized")
}

// TestExecuteVendorPullCommand_Everything_NoVendorFile_RefreshLock_ForcesReDownload proves
// flg.RefreshLock threads through the sweep path: without --refresh-lock, an already-materialized
// component is skipped even if its upstream source has since changed; with --refresh-lock, the
// materialization filter is bypassed and the component is re-pulled from its current source.
func TestExecuteVendorPullCommand_Everything_NoVendorFile_RefreshLock_ForcesReDownload(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)

	src := t.TempDir()
	sourceFile := filepath.Join(src, "main.tf")
	require.NoError(t, os.WriteFile(sourceFile, []byte("# v1\n"), 0o644))
	dir := writeSweepComponentManifestFixture(t, repoRoot, "terraform", "refresh-comp", src)
	targetFile := filepath.Join(dir, "main.tf")

	// First pull: materializes and records a lock entry.
	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, ExecuteVendorPullCommand(cmd, nil))
	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	require.Equal(t, "# v1\n", string(content))

	// Mutate the upstream source.
	require.NoError(t, os.WriteFile(sourceFile, []byte("# v2\n"), 0o644))

	// Second pull without --refresh-lock: already-materialized target, must be skipped.
	cmd2 := newVendorPullSweepTestCmd()
	require.NoError(t, ExecuteVendorPullCommand(cmd2, nil))
	content, err = os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "# v1\n", string(content), "without --refresh-lock, an already-materialized component must not be re-pulled")

	// Third pull with --refresh-lock: must bypass the materialization filter and re-pull.
	cmd3 := newVendorPullSweepTestCmd()
	require.NoError(t, cmd3.Flags().Set("refresh-lock", "true"))
	require.NoError(t, ExecuteVendorPullCommand(cmd3, nil))
	content, err = os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "# v2\n", string(content), "--refresh-lock must force a re-download even when already materialized")
}

// TestExecuteVendorPullCommand_StackAndTagsExcludesUntaggedComponents is the end-to-end regression
// test for --stack/--tags composition. Vendor pull with both --stack and --tags set must narrow the
// stack-resolved component set by vendor.yaml-declared tags. It must not silently ignore --tags,
// and it must not reject the combination outright. The fixture's dev stack resolves to a vpc
// component that only has a component.yaml, with no vendor.yaml entry and therefore no declared
// tags at all, so a non-empty tags filter excludes it -- proving the filter actually applies rather
// than being a no-op, with the zero-match result surfaced as an explicit error.
func TestExecuteVendorPullCommand_StackAndTagsExcludesUntaggedComponents(t *testing.T) {
	buildHandleStackVendorFixture(t)

	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, cmd.Flags().Set("stack", "dev"))
	require.NoError(t, cmd.Flags().Set("tags", "networking"))

	err := ExecuteVendorPullCommand(cmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

// TestExecuteVendorPullCommand_LabelsOnly_PullsMatchingComponent proves --labels alone (with no
// --stack) reaches the same stack-resolution path --stack does: ExecuteVendorPullCommand's
// needsStackProcessing presence check (stackFlagVal != "" || labelsFlagVal != "") must fire on
// --labels alone so atmosConfig is initialized with processStacks=true, and handleVendorConfig's
// "flg.Stack != "" || len(flg.Labels) > 0" branch must route to handleStackVendor on --labels alone
// too. Every other test in this file that reaches handleStackVendor sets --stack; this is the only
// one that exercises --labels by itself.
func TestExecuteVendorPullCommand_LabelsOnly_PullsMatchingComponent(t *testing.T) {
	atmosConfig := buildHandleStackVendorFixture(t)

	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, cmd.Flags().Set("labels", "tier=1"))

	err := ExecuteVendorPullCommand(cmd, nil)

	require.NoError(t, err)
	content, readErr := os.ReadFile(filepath.Join(atmosConfig.BasePath, "components", "terraform", "vpc", "main.tf"))
	require.NoError(t, readErr)
	assert.Equal(t, "# vpc\n", string(content), "dev's vpc (tier=1) must have been pulled via --labels alone")

	prodContent, readErr := os.ReadFile(filepath.Join(atmosConfig.BasePath, "components", "terraform", "vpc-prod", "main.tf"))
	require.NoError(t, readErr)
	assert.Empty(t, string(prodContent), "prod's vpc-prod (tier=2) must not match --labels tier=1")
}

// TestExecuteVendorPullCommand_ComponentAndTags_NoVendorFile_ReturnsInvalidArgument proves
// handleVendorConfig's "--component resolved via its own component.yaml, which has no tags to match"
// branch: with no vendor.yaml at all, --component combined with --tags can never match (a
// component.yaml has no tags concept), so the call must fail with errUtils.ErrInvalidArgumentError
// instead of silently pulling the component or silently ignoring --tags.
func TestExecuteVendorPullCommand_ComponentAndTags_NoVendorFile_ReturnsInvalidArgument(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.tf"), []byte("# vpc\n"), 0o644))
	dir := writeSweepComponentManifestFixture(t, repoRoot, "terraform", "vpc", src)

	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, cmd.Flags().Set("component", "vpc"))
	require.NoError(t, cmd.Flags().Set("tags", "networking"))

	err := ExecuteVendorPullCommand(cmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
	assert.NoFileExists(t, filepath.Join(dir, "main.tf"), "an unmatched --component/--tags combination must not pull anything")
}

// TestExecuteVendorPullCommand_ComponentOnly_NoVendorFile_PullsComponent proves --component alone
// (no --tags), with no vendor.yaml at all, falls through handleVendorConfig's remaining branches to
// handleComponentVendor and actually pulls the component via its own component.yaml -- the positive
// companion to the --tags-rejection case above, confirming that fallthrough is actually reached.
func TestExecuteVendorPullCommand_ComponentOnly_NoVendorFile_PullsComponent(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.tf"), []byte("# vpc\n"), 0o644))
	dir := writeSweepComponentManifestFixture(t, repoRoot, "terraform", "vpc", src)

	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, cmd.Flags().Set("component", "vpc"))

	err := ExecuteVendorPullCommand(cmd, nil)

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "main.tf"))
}

// TestExecuteVendorPullCommand_ComponentAndTagsMismatch_WithVendorFile_ReturnsInvalidArgument is the
// direct regression test for the reported bug: "atmos vendor pull -c vpc --tags compute", where vpc
// IS declared in vendor.yaml but its own declared tags don't include "compute" -- while some OTHER
// declared component (eks) does -- previously passed validateTagsAndComponents's global,
// unscoped tag-existence check (which only checked that "compute" exists SOMEWHERE in vendor.yaml,
// not that vpc itself has it), then silently filtered down to zero packages in
// processAtmosVendorSource and returned nil. It must now fail with errUtils.ErrInvalidArgumentError,
// matching "vendor update --component --tags" mismatch behavior, and must not pull anything.
func TestExecuteVendorPullCommand_ComponentAndTagsMismatch_WithVendorFile_ReturnsInvalidArgument(t *testing.T) {
	repoRoot := t.TempDir()
	t.Chdir(repoRoot)

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "vendor.yaml"), []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock-vpc:{{.Version}}
      version: v0.1.0
      tags: [networking]
      targets: ["components/terraform/vpc"]
    - component: eks
      source: oci://ghcr.io/cloudposse/mock-eks:{{.Version}}
      version: v0.1.0
      tags: [compute]
      targets: ["components/terraform/eks"]
`), 0o644))

	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, cmd.Flags().Set("component", "vpc"))
	require.NoError(t, cmd.Flags().Set("tags", "compute"))

	err := ExecuteVendorPullCommand(cmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
	assert.NoFileExists(t, filepath.Join(repoRoot, "components", "terraform", "vpc", "main.tf"),
		"vpc's own tags don't include 'compute' -- eks matching must not vouch for it")
}

// TestExecuteVendorPullCommand_StackAndTagsMatchesDeclaredComponent proves the positive case of
// --stack/--tags composition: when a stack-resolved component DOES have a matching vendor.yaml
// entry, --tags keeps it (and drops any sibling that doesn't match), and it still installs via its
// own component.yaml -- --tags only narrows which stack-resolved names survive the filter, it never
// changes how a surviving one is installed (that's still entirely component.yaml-driven).
func TestExecuteVendorPullCommand_StackAndTagsMatchesDeclaredComponent(t *testing.T) {
	buildHandleStackVendorFixture(t)
	require.NoError(t, os.WriteFile("vendor.yaml", []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock-vpc:{{.Version}}
      version: v0.1.0
      tags: [networking]
      targets: ["components/terraform/vpc"]
`), 0o644))

	cmd := newVendorPullSweepTestCmd()
	require.NoError(t, cmd.Flags().Set("stack", "dev"))
	require.NoError(t, cmd.Flags().Set("tags", "networking"))

	err := ExecuteVendorPullCommand(cmd, nil)

	require.NoError(t, err)
	content, readErr := os.ReadFile(filepath.Join("components", "terraform", "vpc", "main.tf"))
	require.NoError(t, readErr)
	assert.Equal(t, "# vpc\n", string(content), "vpc must install via its own component.yaml, unaffected by the vendor.yaml entry used only for tag matching")
}
