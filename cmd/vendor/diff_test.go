package vendor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

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
