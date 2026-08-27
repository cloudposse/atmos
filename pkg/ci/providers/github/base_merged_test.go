package github

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mergedPRFixture is a real git repository reproducing the incident shape:
// a multi-commit PR whose FINAL commit reverts an earlier org-wide change,
// so the PR's net diff differs from its last commit's diff.
//
//	main:    forkPoint ── mainAdvance ─────────── mergeCommit
//	                 \                           /
//	feature:          orgWideChange ── revert ──╯   (revert = PR head)
//
// Also provides a squash commit (parent: mainAdvance) and a synthetic
// test-merge commit (parents: mainAdvance, revert) to cover the other
// merge strategies and checkout modes.
type mergedPRFixture struct {
	dir            string
	forkPoint      string
	mainAdvance    string
	orgWideChange  string
	prHead         string
	mergeCommit    string
	squashCommit   string
	syntheticMerge string
	// Fast-forward scenario: a 2-commit branch forked from mergeCommit and
	// fast-forwarded onto the target (merge_commit_sha == head.sha).
	ffFirstCommit string
	ffHead        string
}

// runGitCmd runs a git command in dir and returns trimmed stdout.
func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
	return strings.TrimSpace(string(out))
}

// commitFixtureFile writes a file and commits it, returning the commit SHA.
func commitFixtureFile(t *testing.T, dir, name, content, msg string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	runGitCmd(t, dir, "add", name)
	runGitCmd(t, dir, "commit", "-m", msg)
	return runGitCmd(t, dir, "rev-parse", "HEAD")
}

func buildMergedPRFixture(t *testing.T) *mergedPRFixture {
	t.Helper()
	dir := t.TempDir()
	f := &mergedPRFixture{dir: dir}

	runGitCmd(t, dir, "init", "-b", "main")

	f.forkPoint = commitFixtureFile(t, dir, "defaults.yaml", "backend: dynamodb\n", "fork point")

	// PR branch: an org-wide change, then a commit that reverts it while
	// touching an unrelated file — net PR diff is workflow.yaml only.
	runGitCmd(t, dir, "checkout", "-b", "feature")
	f.orgWideChange = commitFixtureFile(t, dir, "defaults.yaml", "backend: s3-lockfile\n", "org-wide backend change")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "defaults.yaml"), []byte("backend: dynamodb\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte("lock-timeout: 300s\n"), 0o644))
	runGitCmd(t, dir, "add", "defaults.yaml", "workflow.yaml")
	runGitCmd(t, dir, "commit", "-m", "add lock-timeout; revert backend change")
	f.prHead = runGitCmd(t, dir, "rev-parse", "HEAD")

	// Target branch advances independently before the merge.
	runGitCmd(t, dir, "checkout", "main")
	f.mainAdvance = commitFixtureFile(t, dir, "unrelated.txt", "x\n", "main moved forward")

	// Merge-commit strategy.
	runGitCmd(t, dir, "merge", "--no-ff", f.prHead, "-m", "merge PR")
	f.mergeCommit = runGitCmd(t, dir, "rev-parse", "HEAD")

	// Squash strategy (on a scratch branch so main keeps the merge commit).
	runGitCmd(t, dir, "checkout", "-b", "squash-target", f.mainAdvance)
	runGitCmd(t, dir, "merge", "--squash", f.prHead)
	runGitCmd(t, dir, "commit", "-m", "squash PR")
	f.squashCommit = runGitCmd(t, dir, "rev-parse", "HEAD")

	// Synthetic refs/pull/<n>/merge-style test merge: same parents as the
	// real merge commit but a distinct commit.
	runGitCmd(t, dir, "checkout", "-b", "synthetic-target", f.mainAdvance)
	runGitCmd(t, dir, "merge", "--no-ff", f.prHead, "-m", "synthetic test merge")
	f.syntheticMerge = runGitCmd(t, dir, "rev-parse", "HEAD")

	// Fast-forward scenario: 2-commit branch off the merge commit, each
	// commit touching a different file, fast-forwarded onto main — GitHub
	// marks such PRs merged with merge_commit_sha == head.sha.
	runGitCmd(t, dir, "checkout", "-b", "ff-feature", f.mergeCommit)
	f.ffFirstCommit = commitFixtureFile(t, dir, "dev.yaml", "size: medium\n", "ff: dev change")
	f.ffHead = commitFixtureFile(t, dir, "prod.yaml", "size: xxlarge\n", "ff: prod change")
	runGitCmd(t, dir, "checkout", "main")
	runGitCmd(t, dir, "merge", "--ff-only", f.ffHead)

	return f
}

// setMergedPREvent points the GitHub event env at a merged-PR payload.
func setMergedPREvent(t *testing.T, f *mergedPRFixture, mergeCommitSHA string) {
	t.Helper()
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")
	payload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"merged":           true,
			"merge_commit_sha": mergeCommitSHA,
			"head": map[string]any{
				"sha": f.prHead,
			},
			"base": map[string]any{
				"ref": "main",
				"sha": f.mainAdvance,
			},
		},
	}
	t.Setenv("GITHUB_EVENT_PATH", writeEventPayload(t, payload))
}

// TestResolveBase_MergedPR_HeadSHACheckout_MergeCommitStrategy is the
// incident regression test: a merged multi-commit PR whose final commit
// reverts an earlier org-wide change, with the PR head checked out (the
// documented workflow's checkout) and a merge-commit merge.
//
// Pre-fix behavior: merge-base(HEAD, origin/main) collapses to HEAD (the
// head is an ancestor of the post-merge main), the HEAD~1 fallback then
// resolves the base to the PR's own previous commit, and the diff reports
// the reverted org-wide change as affected — dispatching plans/applies for
// every component after the merge. The correct base is the fork point.
func TestResolveBase_MergedPR_HeadSHACheckout_MergeCommitStrategy(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.prHead)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, f.mergeCommit)

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.forkPoint, res.SHA,
		"base must be the fork point (net PR diff), NOT the PR's previous commit (last-commit-only diff)")
	assert.NotEqual(t, f.orgWideChange, res.SHA,
		"regression guard: HEAD~1 on a head.sha checkout resolves the reverted commit and re-reports the org-wide change")
	assert.Equal(t, sourceMergedForkPoint, res.Source)
	assert.Equal(t, checkoutPRHead, res.Checkout)
	assert.Equal(t, f.prHead, res.HeadSHA)
}

// TestResolveBase_MergedPR_HeadSHACheckout_SquashStrategy covers the squash
// merge: merge_commit_sha is the squash commit, whose first parent is the
// pre-merge main tip; the merge-base against it is still the fork point.
func TestResolveBase_MergedPR_HeadSHACheckout_SquashStrategy(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.prHead)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, f.squashCommit)

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.forkPoint, res.SHA)
	assert.Equal(t, sourceMergedForkPoint, res.Source)
	assert.Equal(t, checkoutPRHead, res.Checkout)
}

// TestResolveBase_MergedPR_MergeCommitCheckout covers the checkout the old
// HEAD~1 fallback was designed for: the merge commit itself is checked out,
// so its first parent is the pre-merge main tip — the exact net-diff base.
func TestResolveBase_MergedPR_MergeCommitCheckout(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.mergeCommit)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, f.mergeCommit)

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.mainAdvance, res.SHA, "first parent of the merge commit is the pre-merge main tip")
	assert.Equal(t, sourceMergeCommitParent, res.Source)
	assert.Equal(t, checkoutMergeCommit, res.Checkout)
}

// TestResolveBase_MergedPR_SyntheticMergeCheckout covers workflows that
// check out GitHub's synthetic refs/pull/<n>/merge test-merge commit: the
// second parent is the PR head, so the first parent is the target tip the
// merge was built on — resolved directly, no merge-base needed.
func TestResolveBase_MergedPR_SyntheticMergeCheckout(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.syntheticMerge)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, f.mergeCommit)

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.mainAdvance, res.SHA)
	assert.Equal(t, sourceSyntheticMergeParent, res.Source)
	assert.Equal(t, checkoutSyntheticMerge, res.Checkout)
}

// TestResolveBase_MergedPR_UnknownCheckout covers a checkout matching none
// of the payload facts (e.g., the target branch tip): no strategy is provably
// correct, so resolution falls back to the payload base.sha with a Warn
// instead of silently guessing.
func TestResolveBase_MergedPR_UnknownCheckout(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.mainAdvance)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, f.mergeCommit)

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.mainAdvance, res.SHA, "payload base.sha fallback")
	assert.Equal(t, sourcePayloadBaseSHA, res.Source)
	assert.Equal(t, checkoutUnknown, res.Checkout)
}

// TestResolveBase_MergedPR_QueueMergedClosedEvent models the closed event
// GitHub fires after a merge queue lands a PR: structurally a merge-commit
// merge with the PR head checked out. The merged-PR path must resolve the
// fork point — merge queues do not exempt the closed event from this bug.
func TestResolveBase_MergedPR_QueueMergedClosedEvent(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.prHead)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, f.mergeCommit)

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.forkPoint, res.SHA)
	assert.Equal(t, checkoutPRHead, res.Checkout)
}

// TestResolveBase_MergedPR_FastForwardMerge covers externally-merged /
// fast-forwarded PRs, where GitHub reports merge_commit_sha == head.sha
// (e.g. the branch was pushed directly to the target and the PR auto-closed
// as merged). Anchoring on merge_commit_sha^1 would resolve the PR's OWN
// previous commit and silently drop every earlier commit's changes from the
// diff (under-detection). The correct base is the fork point, reached by
// anchoring on the payload's base.sha instead.
func TestResolveBase_MergedPR_FastForwardMerge(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.ffHead)
	t.Chdir(f.dir)

	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")
	payload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"merged":           true,
			"merge_commit_sha": f.ffHead, // == head.sha: the FF signature.
			"head":             map[string]any{"sha": f.ffHead},
			"base":             map[string]any{"ref": "main", "sha": f.mergeCommit},
		},
	}
	t.Setenv("GITHUB_EVENT_PATH", writeEventPayload(t, payload))

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.mergeCommit, res.SHA,
		"base must be the fork point (pre-FF target tip), so BOTH commits' changes are in the diff")
	assert.NotEqual(t, f.ffFirstCommit, res.SHA,
		"regression guard: anchoring on merge_commit_sha^1 silently drops the first commit's changes")
	assert.Equal(t, checkoutPRHead, res.Checkout)
}

// TestClassifyPRCheckout_InitialCommit verifies a parentless initial commit
// classifies without error: an initial commit is a valid commit (empty
// parents, nil error from git.CommitParents), and the head.sha match still
// applies to it.
func TestClassifyPRCheckout_InitialCommit(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-b", "main")
	initial := commitFixtureFile(t, dir, "a.txt", "a", "initial")
	t.Chdir(dir)

	checkout, parents := classifyPRCheckout(initial, "")
	assert.Equal(t, checkoutPRHead, checkout)
	assert.Empty(t, parents, "initial commit has no parents — valid, not an error")

	checkout, _ = classifyPRCheckout("someothersha000000000000000000000000dead", "")
	assert.Equal(t, checkoutUnknown, checkout)
}

// TestResolveBase_ClosedUnmergedPR verifies closed-without-merge PRs do NOT
// take the merged-PR path: the branch was never folded into the target, so
// they resolve like open PRs (merge-base, then payload base.sha).
func TestResolveBase_ClosedUnmergedPR(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.prHead)
	t.Chdir(f.dir)

	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")
	payload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"merged":           false,
			"merge_commit_sha": nil,
			"head":             map[string]any{"sha": f.prHead},
			"base":             map[string]any{"ref": "main", "sha": f.mainAdvance},
		},
	}
	t.Setenv("GITHUB_EVENT_PATH", writeEventPayload(t, payload))

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	// No origin remote in the fixture, so merge-base auto-fetch fails and
	// the open-PR chain lands on the payload base.sha — never on a
	// merged-PR source.
	assert.Equal(t, sourcePayloadBaseSHA, res.Source)
	assert.Equal(t, f.mainAdvance, res.SHA)
}

// TestResolveMergedPRBase_HeadCheckout_NoMergeCommitSHA covers a merged PR
// with the head SHA checked out whose event payload has no
// merge_commit_sha (a degenerate payload): mergedPRForkPoint has nothing to
// anchor on (ErrNoMergeCommitSHA), mergedPRHeadAnchoredBase propagates the
// error, and resolveMergedPRBase must Warn and fall back to the payload
// base.sha tier instead of silently guessing a base.
func TestResolveMergedPRBase_HeadCheckout_NoMergeCommitSHA(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.prHead)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, "") // No merge_commit_sha in the payload.

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.mainAdvance, res.SHA, "falls back to payload base.sha")
	assert.Equal(t, sourcePayloadBaseSHA, res.Source)
	assert.Equal(t, checkoutPRHead, res.Checkout)
}

// TestResolveMergedPRBase_HeadCheckout_MergeCommitNotFetchable covers a
// merge_commit_sha that names a commit neither present locally nor
// fetchable (the fixture repo has no "origin" remote): mergedPRForkPoint's
// CommitParents lookup fails, the FetchCommit recovery attempt also fails,
// and the original error must propagate rather than falling through with a
// wrong SHA.
func TestResolveMergedPRBase_HeadCheckout_MergeCommitNotFetchable(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.prHead)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.mainAdvance, res.SHA, "falls back to payload base.sha when the merge commit can't be resolved or fetched")
	assert.Equal(t, sourcePayloadBaseSHA, res.Source)
}

// TestResolveMergedPRBase_HeadCheckout_MergeCommitHasNoParents covers a
// merge_commit_sha that resolves locally but has no parents — using the
// fixture's own fork-point commit (its very first, parentless commit) as a
// stand-in for a degenerate/rewritten-history merge commit — where
// mergedPRForkPoint must return ErrCommitHasNoParents rather than computing
// merge-base against a nonexistent parent.
func TestResolveMergedPRBase_HeadCheckout_MergeCommitHasNoParents(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.prHead)
	t.Chdir(f.dir)
	setMergedPREvent(t, f, f.forkPoint) // forkPoint has no parent.

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, f.mainAdvance, res.SHA, "falls back to payload base.sha when the merge commit has no parent to anchor on")
	assert.Equal(t, sourcePayloadBaseSHA, res.Source)
}

// TestResolveMergedPRBase_FastForwardMerge_NoPayloadBaseSHA covers a
// fast-forward-merged PR (merge_commit_sha == head.sha) whose event payload
// is missing base.sha: mergedPRHeadAnchoredBase has no anchor to compute
// the fork point from and must return ErrNoPayloadBaseSHA, triggering the
// Warn-and-fallback path down to the last-resort target-branch ref.
func TestResolveMergedPRBase_FastForwardMerge_NoPayloadBaseSHA(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.ffHead)
	t.Chdir(f.dir)

	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")
	payload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"merged":           true,
			"merge_commit_sha": f.ffHead, // == head.sha: the FF signature.
			"head":             map[string]any{"sha": f.ffHead},
			"base":             map[string]any{"ref": "main"}, // NOTE: no "sha".
		},
	}
	t.Setenv("GITHUB_EVENT_PATH", writeEventPayload(t, payload))

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.SHA)
	assert.Equal(t, "refs/remotes/origin/main", res.Ref)
	assert.Equal(t, sourceGitHubBaseRef, res.Source)
	assert.Equal(t, checkoutPRHead, res.Checkout)
}

// TestResolveMergedPRBase_FastForwardMerge_BaseSHANotFetchable covers a
// fast-forward-merged PR whose payload base.sha names a commit that is
// neither present locally nor fetchable: forkPointFromAnchor's CommitParents
// lookup and FetchCommit recovery both fail, and mergedPRHeadAnchoredBase
// must propagate that error rather than silently falling through.
func TestResolveMergedPRBase_FastForwardMerge_BaseSHANotFetchable(t *testing.T) {
	f := buildMergedPRFixture(t)
	runGitCmd(t, f.dir, "checkout", f.ffHead)
	t.Chdir(f.dir)

	const fakeBaseSHA = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")
	payload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"merged":           true,
			"merge_commit_sha": f.ffHead,
			"head":             map[string]any{"sha": f.ffHead},
			"base":             map[string]any{"ref": "main", "sha": fakeBaseSHA},
		},
	}
	t.Setenv("GITHUB_EVENT_PATH", writeEventPayload(t, payload))

	res, err := NewProvider().ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, fakeBaseSHA, res.SHA, "falls back to the payload base.sha itself when its fork-point anchor can't be resolved or fetched")
	assert.Equal(t, sourcePayloadBaseSHA, res.Source)
}
