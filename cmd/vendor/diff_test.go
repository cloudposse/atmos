package vendor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// diffBatchFailingContext wraps a real io.Context and fails exactly the Write call numbered
// failOnCall (1-indexed across all Write calls made through it), delegating every other call to
// the wrapped context so tests can target diffManyComponents' two distinct write-error branches
// (the "## <component>" header write, or separately the diff-body write) individually -- unlike
// componentUpdaterFailingContext (cmd/vendor/component_updater_test.go), which fails every call
// unconditionally.
type diffBatchFailingContext struct {
	iolib.Context
	failOnCall int
	calls      int
	err        error
}

func (c *diffBatchFailingContext) Write(stream iolib.Stream, content string) error {
	c.calls++
	if c.calls == c.failOnCall {
		return c.err
	}
	return c.Context.Write(stream, content)
}

// TestVendorDiffCommand_HonorsBasePathFlag proves resolveDiffComponents threads --base-path into
// cfg.InitCliConfig instead of discarding it via an empty schema.ConfigAndStacksInfo{} -- a stack
// declared under a project root reachable only via --base-path (not the test's cwd) must resolve.
// Before the fix, --base-path had no effect: InitCliConfig looked relative to cwd instead, which has
// no atmos.yaml/stacks here, so the error wouldn't reference root's own stacks path.
func TestVendorDiffCommand_HonorsBasePathFlag(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(
		"base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n",
	), 0o644))

	elsewhere := t.TempDir()
	chdirTest(t, elsewhere)

	resetCommandFlags(t, vendorDiffCmd)
	require.NoError(t, vendorDiffCmd.Flags().Set("stack", "does-not-exist"))

	v := viper.GetViper()
	v.Set("base-path", root)
	t.Cleanup(func() { v.Set("base-path", "") })

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoStackManifestsFound)
	assert.Contains(t, err.Error(), root, "the error must reference root's own stacks path, proving --base-path (not cwd) was used to locate stacks")
}

// TestVendorDiffCommand_MalformedLabelsErrors proves an unparsable --labels value (missing the "="
// or ":" key/value separator) is rejected before any manifest/config resolution is attempted.
func TestVendorDiffCommand_MalformedLabelsErrors(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	require.NoError(t, vendorDiffCmd.Flags().Set("labels", "notkeyvalue"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
}

// TestVendorDiffCommand_ComponentAndStackRejected proves --component and --stack are mutually
// exclusive base selectors, mirroring the existing TestVendorDiffCommand_ComponentAndTagsRejected
// (which covers --component+--tags, a composing pair, not this rejected pair).
func TestVendorDiffCommand_ComponentAndStackRejected(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	require.NoError(t, vendorDiffCmd.Flags().Set("component", "vpc"))
	require.NoError(t, vendorDiffCmd.Flags().Set("stack", "dev"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

// newDiffGitRepoFixture creates a local (non-bare) Git repository under a fresh temp directory with
// two tagged commits ("v1.0.0" and "v1.1.0") that change main.tf's contents, entirely on local disk
// -- no network access. Mirrors pkg/vendoring/diff_test.go's TestGoGitDiffer_DiffLocalRepository
// fixture technique, the established precedent in this repo for exercising vendoring.Diff's real
// GoGitDiffer without hitting an actual Git remote. The resourceName parameter lets callers
// building multiple fixtures (batch tests) produce distinguishable diff bodies per repo.
func newDiffGitRepoFixture(t *testing.T, resourceName string) (repoDir string) {
	t.Helper()
	repoDir = t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	require.NoError(t, err)

	commit := func(tag, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.tf"), []byte(body), 0o644))
		wt, err := repo.Worktree()
		require.NoError(t, err)
		_, err = wt.Add("main.tf")
		require.NoError(t, err)
		hash, err := wt.Commit(tag, &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Atmos Test",
				Email: "atmos-test@example.com",
				When:  time.Unix(1000, 0),
			},
		})
		require.NoError(t, err)
		_, err = repo.CreateTag(tag, hash, nil)
		require.NoError(t, err)
	}

	commit("v1.0.0", fmt.Sprintf("resource \"null_resource\" \"%s_old\" {}\n", resourceName))
	commit("v1.1.0", fmt.Sprintf("resource \"null_resource\" \"%s_new\" {}\n", resourceName))

	return repoDir
}

// gitSourceFor turns a local fixture repo directory into a vendor manifest "source:" value that
// version.IsGitSource recognizes as Git (it only matches a "git::" prefix or a
// github.com/gitlab.com/bitbucket.org host -- a bare local filesystem path matches neither), while
// version.ExtractGitURI strips the "git::" prefix back down to the plain local path go-git's
// PlainCloneContext accepts directly. Running the result through filepath.ToSlash keeps the
// resulting manifest valid YAML and the path parseable on Windows, where a bare backslash path
// could be misread.
func gitSourceFor(repoDir string) string {
	return "git::" + filepath.ToSlash(repoDir)
}

// TestVendorDiffCommand_SingleComponentSuccess exercises diffOneComponent's real success path
// (vendoring.Diff actually clones and diffs, then RunE writes the raw result via data.Writeln) --
// every other diff fixture in this package deliberately uses a non-Git oci:// source so it fails
// fast before ever reaching this code, to avoid a real network call. This local git-fixture
// (no network) is what makes exercising the actual success path possible at all.
func TestVendorDiffCommand_SingleComponentSuccess(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	repoDir := newDiffGitRepoFixture(t, "vpc")
	file := writeCommandVendorManifest(t, fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: "%s"
      version: v1.0.0
      targets: ["components/terraform/vpc"]
`, gitSourceFor(repoDir)))

	stdout := captureVendorStdout(t)
	require.NoError(t, vendorDiffCmd.Flags().Set("file", file))
	require.NoError(t, vendorDiffCmd.Flags().Set("component", "vpc"))
	require.NoError(t, vendorDiffCmd.Flags().Set("to", "v1.1.0"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "diff --git", "the real unified diff must be written to the data channel")
	assert.Contains(t, out, "vpc_new", "the diff body (from -> to content change) must be present")
}

// TestVendorDiffCommand_BatchSuccess exercises diffManyComponents' real success path (both the
// "## <component>" header write and the diff-body write, for every resolved component) against two
// local git fixtures, no network access. Batch mode was previously only ever proven against
// deliberately-failing oci:// sources (TestVendorDiffCommand_TagsSelectorBatchMode), which never
// reached either of diffManyComponents' two success-write lines.
func TestVendorDiffCommand_BatchSuccess(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	vpcRepo := newDiffGitRepoFixture(t, "vpc")
	eksRepo := newDiffGitRepoFixture(t, "eks")
	file := writeCommandVendorManifest(t, fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: "%s"
      version: v1.0.0
      tags: [networking]
      targets: ["components/terraform/vpc"]
    - component: eks
      source: "%s"
      version: v1.0.0
      tags: [networking]
      targets: ["components/terraform/eks"]
`, gitSourceFor(vpcRepo), gitSourceFor(eksRepo)))

	stdout := captureVendorStdout(t)
	require.NoError(t, vendorDiffCmd.Flags().Set("file", file))
	require.NoError(t, vendorDiffCmd.Flags().Set("tags", "networking"))
	require.NoError(t, vendorDiffCmd.Flags().Set("to", "v1.1.0"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "## vpc", "vpc's batch header must be written")
	assert.Contains(t, out, "## eks", "eks's batch header must be written")
	assert.Contains(t, out, "vpc_new", "vpc's diff body must be written")
	assert.Contains(t, out, "eks_new", "eks's diff body must be written")
}

// TestVendorDiffCommand_BatchWriteErrorPropagates proves a data-writer failure inside
// diffManyComponents surfaces as a command error instead of being silently swallowed, and does so
// for BOTH of its distinct write call sites: the "## <component>" header write (failOnCall: 1,
// short-circuiting before any component is processed) and the diff-body write (failOnCall: 2, after
// the first component's header was already written successfully).
func TestVendorDiffCommand_BatchWriteErrorPropagates(t *testing.T) {
	tests := []struct {
		name       string
		failOnCall int
	}{
		{name: "header write", failOnCall: 1},
		{name: "diff-body write", failOnCall: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCommandFlags(t, vendorDiffCmd)
			vpcRepo := newDiffGitRepoFixture(t, "vpc")
			eksRepo := newDiffGitRepoFixture(t, "eks")
			file := writeCommandVendorManifest(t, fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: "%s"
      version: v1.0.0
      tags: [networking]
      targets: ["components/terraform/vpc"]
    - component: eks
      source: "%s"
      version: v1.0.0
      tags: [networking]
      targets: ["components/terraform/eks"]
`, gitSourceFor(vpcRepo), gitSourceFor(eksRepo)))

			ioCtx, err := iolib.NewContext()
			require.NoError(t, err)
			data.InitWriter(&diffBatchFailingContext{Context: ioCtx, failOnCall: tt.failOnCall, err: fmt.Errorf("broken pipe")})
			t.Cleanup(data.Reset)

			require.NoError(t, vendorDiffCmd.Flags().Set("file", file))
			require.NoError(t, vendorDiffCmd.Flags().Set("tags", "networking"))
			require.NoError(t, vendorDiffCmd.Flags().Set("to", "v1.1.0"))

			err = vendorDiffCmd.RunE(vendorDiffCmd, nil)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "broken pipe")
		})
	}
}

// writeDiffStackLabelsFixture writes an atmos.yaml + stacks/dev.yaml declaring stackYAML's
// terraform components and chdirs into the fixture root, mirroring clean_test.go's
// writeCleanStackLabelsFixture/verify_test.go's writeVerifyStackLabelsFixture. Callers additionally
// drop a vendor.yaml at the fixture root (DefaultVendorManifest, resolved via the default "./"
// lookup, not --file) declaring sources for the same component names so resolveDiffComponents'
// stack-resolved names have something to diff.
func writeDiffStackLabelsFixture(t *testing.T, stackYAML string) {
	t.Helper()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(
		"base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n  helmfile:\n    base_path: components/helmfile\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(stackYAML), 0o644))
	chdirTest(t, root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
}

// TestVendorDiffCommand_StackSuccess proves --stack resolves the stack's declared component names
// and diffs each of them through resolveDiffComponents' stack-resolution path -- diff had zero
// --stack coverage (happy path) before this test.
func TestVendorDiffCommand_StackSuccess(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	vpcRepo := newDiffGitRepoFixture(t, "vpc")
	eksRepo := newDiffGitRepoFixture(t, "eks")
	writeDiffStackLabelsFixture(t, "vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc: {}\n    eks: {}\n")
	require.NoError(t, os.WriteFile(DefaultVendorManifest, []byte(fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: "%s"
      version: v1.0.0
      targets: ["components/terraform/vpc"]
    - component: eks
      source: "%s"
      version: v1.0.0
      targets: ["components/terraform/eks"]
`, gitSourceFor(vpcRepo), gitSourceFor(eksRepo))), 0o644))

	stdout := captureVendorStdout(t)
	require.NoError(t, vendorDiffCmd.Flags().Set("stack", "dev"))
	require.NoError(t, vendorDiffCmd.Flags().Set("to", "v1.1.0"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "## vpc", "vpc's batch header must be written")
	assert.Contains(t, out, "## eks", "eks's batch header must be written")
	assert.Contains(t, out, "vpc_new", "vpc's diff body must be written")
	assert.Contains(t, out, "eks_new", "eks's diff body must be written")
}

// TestVendorDiffCommand_StackAndEnvTypeSuccess proves --stack narrows by --type even when --type was
// never passed as a command-line flag but only via its bound environment variable (ATMOS_TYPE) --
// resolveDiffComponents' TypeChanged must reflect the env-set effective type, not just
// cmd.Flags().Changed("type"), or an env-only type override would be silently ignored (violating
// flag precedence: CLI flags > env vars > config > defaults).
func TestVendorDiffCommand_StackAndEnvTypeSuccess(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	vpcRepo := newDiffGitRepoFixture(t, "vpc")
	eksRepo := newDiffGitRepoFixture(t, "eks")
	writeDiffStackLabelsFixture(t,
		"vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc: {}\n  helmfile:\n    eks: {}\n")
	require.NoError(t, os.WriteFile(DefaultVendorManifest, []byte(fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: "%s"
      version: v1.0.0
      targets: ["components/terraform/vpc"]
    - component: eks
      source: "%s"
      version: v1.0.0
      targets: ["components/helmfile/eks"]
`, gitSourceFor(vpcRepo), gitSourceFor(eksRepo))), 0o644))

	stdout := captureVendorStdout(t)
	require.NoError(t, vendorDiffCmd.Flags().Set("stack", "dev"))
	require.NoError(t, vendorDiffCmd.Flags().Set("to", "v1.1.0"))
	// ATMOS_TYPE, not --type: the whole point of this test is that no --type flag was ever set on
	// the command line. viper.GetString only consults an env var once AutomaticEnv/SetEnvPrefix are
	// configured (normally done once, globally, by cmd/root.go's init() -- not reachable from this
	// package's tests), so replicate that setup on a fresh viper instance for this test only.
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetEnvPrefix("ATMOS")
	viper.AutomaticEnv()
	t.Setenv("ATMOS_TYPE", "helmfile")

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "diff --git", "the type-matched component's diff must be written")
	assert.Contains(t, out, "eks_new", "eks (helmfile) must be selected and diffed")
	assert.NotContains(t, out, "vpc_new", "vpc (terraform) must be excluded by the env-set --type")
}

// TestVendorDiffCommand_LabelsSuccess proves --labels resolves and diffs only the stack-declared
// component(s) whose stack metadata.labels match, through resolveDiffComponents' label-resolution
// path -- diff had zero --labels coverage (happy path) before this test.
func TestVendorDiffCommand_LabelsSuccess(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	vpcRepo := newDiffGitRepoFixture(t, "vpc")
	eksRepo := newDiffGitRepoFixture(t, "eks")
	writeDiffStackLabelsFixture(t,
		"vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc:\n      metadata:\n        labels:\n          tier: \"1\"\n    eks:\n      metadata:\n        labels:\n          tier: \"2\"\n")
	require.NoError(t, os.WriteFile(DefaultVendorManifest, []byte(fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: "%s"
      version: v1.0.0
      targets: ["components/terraform/vpc"]
    - component: eks
      source: "%s"
      version: v1.0.0
      targets: ["components/terraform/eks"]
`, gitSourceFor(vpcRepo), gitSourceFor(eksRepo))), 0o644))

	stdout := captureVendorStdout(t)
	require.NoError(t, vendorDiffCmd.Flags().Set("labels", "tier=1"))
	require.NoError(t, vendorDiffCmd.Flags().Set("to", "v1.1.0"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "diff --git", "the single resolved component's diff must be written")
	assert.Contains(t, out, "vpc_new", "vpc (tier=1) must be selected and diffed")
	assert.NotContains(t, out, "eks_new", "eks (tier=2) must not be selected")
}

// runDiffTagsNarrowsBaseSelector is the shared body for TestVendorDiffCommand_StackAndTagsSuccess
// and _LabelsAndTagsSuccess: writes a vendor.yaml declaring vpc (tagged networking) and eks (tagged
// compute), resolves stackYAML's components via the given base selector flag/value, narrows by
// --tags networking, and asserts only vpc's diff is written -- proving --tags narrows whichever
// base selector resolved rather than being ignored after resolution.
func runDiffTagsNarrowsBaseSelector(t *testing.T, stackYAML, selectorFlag, selectorValue string) {
	t.Helper()
	resetCommandFlags(t, vendorDiffCmd)
	vpcRepo := newDiffGitRepoFixture(t, "vpc")
	eksRepo := newDiffGitRepoFixture(t, "eks")
	writeDiffStackLabelsFixture(t, stackYAML)
	require.NoError(t, os.WriteFile(DefaultVendorManifest, []byte(fmt.Sprintf(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: "%s"
      version: v1.0.0
      tags: [networking]
      targets: ["components/terraform/vpc"]
    - component: eks
      source: "%s"
      version: v1.0.0
      tags: [compute]
      targets: ["components/terraform/eks"]
`, gitSourceFor(vpcRepo), gitSourceFor(eksRepo))), 0o644))

	stdout := captureVendorStdout(t)
	require.NoError(t, vendorDiffCmd.Flags().Set(selectorFlag, selectorValue))
	require.NoError(t, vendorDiffCmd.Flags().Set("tags", "networking"))
	require.NoError(t, vendorDiffCmd.Flags().Set("to", "v1.1.0"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "diff --git", "the tag-matched component's diff must be written")
	assert.Contains(t, out, "vpc_new", "vpc (tagged networking) must be selected and diffed")
	assert.NotContains(t, out, "eks_new", "eks (tagged compute) must not be selected")
}

// TestVendorDiffCommand_StackAndTagsSuccess proves --tags narrows --stack's resolved component set
// rather than being ignored after stack resolution -- both vpc and eks are declared in the stack.
func TestVendorDiffCommand_StackAndTagsSuccess(t *testing.T) {
	runDiffTagsNarrowsBaseSelector(t,
		"vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc: {}\n    eks: {}\n",
		"stack", "dev")
}

// TestVendorDiffCommand_LabelsAndTagsSuccess proves --tags narrows --labels' resolved component set
// too -- both vpc and eks share the same stack metadata.labels, so --labels alone would resolve
// both.
func TestVendorDiffCommand_LabelsAndTagsSuccess(t *testing.T) {
	runDiffTagsNarrowsBaseSelector(t,
		"vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc:\n      metadata:\n        labels:\n          tier: \"1\"\n    eks:\n      metadata:\n        labels:\n          tier: \"1\"\n",
		"labels", "tier=1")
}
