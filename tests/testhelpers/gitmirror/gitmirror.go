// Package gitmirror builds a local, file://-served git mirror of this
// checkout's examples/ directory and the per-owner GIT_CONFIG insteadOf rules
// that redirect "github.com/cloudposse/atmos.git//examples/..." fetches to
// it, so the acceptance suite never depends on live GitHub connectivity for
// vendor/import fixtures.
//
// It also carries the "clone a throwaway local git repo instead of the real
// network" helpers the source-provisioner JIT tests use for non-atmos
// upstreams (see JITRepo), folded in from the former
// tests/jit_source_local_repo_test.go so both mechanisms live in one place.
package gitmirror

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/otiai10/copy"

	"github.com/cloudposse/atmos/tests/testhelpers"
	"github.com/cloudposse/atmos/tests/testhelpers/gitconfigenv"
)

// Owner and Repo identify the single upstream this package mirrors today:
// github.com/cloudposse/atmos.
const (
	Owner = "cloudposse"
	Repo  = "atmos"
)

// dirPerm is the permission mode for directories this package creates. Only used by throwaway
// test-local git repositories, never anything shared outside the test process.
const dirPerm = 0o755

// filePerm is the permission mode for files this package writes into throwaway test-local git
// repositories. Owner-only (0o600) since nothing else needs to read or write these fixtures.
const filePerm = 0o600

// gitQuiet suppresses git's normal progress/status chatter on the commands this package shells
// out to, since none of it is useful in a test log.
const gitQuiet = "--quiet"

// Build creates a bare git mirror of this checkout's examples/ directory at
// <root>/cloudposse/atmos.git, on branch main. Only examples/ is copied (the
// only subtree any test-case fixture vendors from cloudposse/atmos), so the
// mirror stays small and fast to build.
func Build(root string) error {
	repoRoot, err := testhelpers.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("gitmirror: locate atmos repo root: %w", err)
	}

	work, err := os.MkdirTemp("", "gitmirror-work-*")
	if err != nil {
		return fmt.Errorf("gitmirror: create scratch dir: %w", err)
	}
	defer os.RemoveAll(work)

	examplesDest := filepath.Join(work, "examples")
	skipGitDirs := func(info os.FileInfo, _, _ string) (bool, error) {
		return info.IsDir() && info.Name() == ".git", nil
	}
	if err := copy.Copy(filepath.Join(repoRoot, "examples"), examplesDest, copy.Options{Skip: skipGitDirs}); err != nil {
		return fmt.Errorf("gitmirror: copy examples/: %w", err)
	}

	if err := commitWorkingRepo(work); err != nil {
		return err
	}

	ownerDir := filepath.Join(root, Owner)
	if err := os.MkdirAll(ownerDir, dirPerm); err != nil {
		return fmt.Errorf("gitmirror: create owner dir: %w", err)
	}
	bareDir := filepath.Join(ownerDir, Repo+".git")
	if err := runGit("", "clone", gitQuiet, "--bare", work, bareDir); err != nil {
		return fmt.Errorf("gitmirror: bare clone: %w", err)
	}
	return nil
}

// commitWorkingRepo turns a plain directory into a single-commit git repo on
// branch "main". `checkout -b main` right after `git init` works on every
// supported git version (HEAD is unborn, so this just names the pending
// initial branch) -- unlike `git init -b main`, which requires git 2.28+.
func commitWorkingRepo(dir string) error {
	steps := [][]string{
		{"init", gitQuiet},
		{"checkout", gitQuiet, "-b", "main"},
		{"config", "user.email", "gitmirror@atmos.test"},
		{"config", "user.name", "Atmos Test Git Mirror"},
		// Never sign commits in throwaway test repos: signing is slow, needs no
		// verification here, and hangs on dev machines whose global git config
		// enables commit.gpgsign (e.g. a 1Password agent).
		{"config", "commit.gpgsign", "false"},
		{"add", "-A"},
		{"commit", gitQuiet, "-m", "gitmirror: snapshot of examples/"},
	}
	for _, args := range steps {
		if err := runGit(dir, args...); err != nil {
			return err
		}
	}
	return nil
}

// runGit runs git with args in dir (the process's own directory when dir is
// empty) and returns its combined output wrapped into the error on failure.
func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gitmirror: git %v failed: %w: %s", args, err, out)
	}
	return nil
}

// FileURI converts a filesystem path into a file:// URI usable as a git remote or GIT_CONFIG
// insteadOf target. On Windows it correctly forms a URI like file:///C:/repo (a volume name
// needs an extra leading slash after the scheme).
func FileURI(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if filepath.VolumeName(path) != "" && cleaned != "" && cleaned[0] != '/' {
		cleaned = "/" + cleaned
	}
	return (&url.URL{Scheme: "file", Path: cleaned}).String()
}

// InsteadOfRules returns the GIT_CONFIG entries that redirect https and ssh git fetches of
// exactly one github.com repository (owner/repo, in its canonical ".git" URL form) to the bare
// mirror at <root>/<owner>/<repo>.git on disk.
//
// The rules are repo-scoped, not owner-wide, on purpose. A rule for "https://github.com/<owner>/"
// would also capture every other repository of that owner that anything in the test process
// fetches through git -- including terraform's own module downloads (for example a
// `git::https://github.com/cloudposse/terraform-null-label?ref=<sha>` module source, pinned to an
// upstream commit no synthetic mirror can reproduce) -- and point them at paths that do not
// exist. Scoping to the one mirrored repository leaves every other fetch exactly as it was.
//
// The rules are still recognized by pkg/downloader/custom_git_detector.go, which decides whether
// to skip injecting a token into the URL by matching an insteadOf value's host and *owner*: with
// these rules present, atmos skips URL token injection for every github.com/<owner>/... URL and
// lets git's rewrite (or, for the non-mirrored repositories, the harness's extraheader) win. That
// matters because a URL with injected userinfo would no longer start with the rule's value and
// would silently go out to the network instead of the mirror.
func InsteadOfRules(root, owner, repo string) []gitconfigenv.GitConfigEntry {
	base := FileURI(filepath.Join(root, owner, repo+".git"))
	key := "url." + base + ".insteadOf"
	entries := make([]gitconfigenv.GitConfigEntry, 0, len(insteadOfTargets("github.com", owner, repo)))
	for _, target := range insteadOfTargets("github.com", owner, repo) {
		entries = append(entries, gitconfigenv.GitConfigEntry{Key: key, Value: target})
	}
	return entries
}

// insteadOfTargets returns the URL forms the mirror rule should rewrite for one repository:
// https and ssh, both in the canonical ".git" form go-getter hands to git for
// github.com/<owner>/<repo>.git shorthand, git::https:// and ssh:// references alike. This
// mirrors the shape of pkg/auth/integrations/github/sts.go's insteadOfTargets (the real broker's
// equivalent, which is owner-scoped because a token covers a whole owner), duplicated here rather
// than imported because that package is production auth logic this test helper has no business
// depending on.
func insteadOfTargets(host, owner, repo string) []string {
	return []string{
		"https://" + host + "/" + owner + "/" + repo + ".git",
		"ssh://git@" + host + "/" + owner + "/" + repo + ".git",
	}
}
