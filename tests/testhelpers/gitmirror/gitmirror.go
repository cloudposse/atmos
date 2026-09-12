// Package gitmirror builds a local git mirror of this checkout's examples/
// directory and serves it over git's real smart-HTTP protocol (see Serve),
// with GIT_CONFIG_GLOBAL insteadOf rules (see WriteGitConfig) redirecting
// "github.com/cloudposse/atmos.git//examples/..." fetches to it -- so the
// acceptance suite never depends on live GitHub connectivity for
// vendor/import fixtures, while atmos itself remains unaware of the mirror:
// it authenticates and gets rewritten by git exactly as it would against the
// real host, running the same token-injection code path a production clone
// does.
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
	"strings"

	"github.com/otiai10/copy"

	"github.com/cloudposse/atmos/tests/testhelpers"
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

// gitConfig is the git subcommand used to set the throwaway scratch repo's local options.
const gitConfig = "config"

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
	// Skip nested .git directories AND .git files (gitlinks): either would turn part of the copy
	// into a submodule entry pointing at a commit this repository does not contain.
	skipGitDirs := func(info os.FileInfo, _, _ string) (bool, error) {
		return info.Name() == ".git", nil
	}
	if err := copy.Copy(filepath.Join(repoRoot, "examples"), examplesDest, copy.Options{Skip: skipGitDirs}); err != nil {
		return fmt.Errorf("gitmirror: copy examples/: %w", err)
	}

	if err := commitWorkingRepo(work); err != nil {
		return err
	}

	// Fail early, and loudly, if the commit did not land: a later push would only report a
	// confusing "nonexistent object" for refs/heads/main.
	if err := runGit(work, "rev-parse", "--verify", gitQuiet, "HEAD"); err != nil {
		return fmt.Errorf("gitmirror: scratch repo has no commit on HEAD (%s): %w", gitStatus(work), err)
	}

	ownerDir := filepath.Join(root, Owner)
	if err := os.MkdirAll(ownerDir, dirPerm); err != nil {
		return fmt.Errorf("gitmirror: create owner dir: %w", err)
	}
	bareDir := filepath.Join(ownerDir, Repo+".git")
	// Publish into a fresh bare repository with a push rather than `git clone --bare <path>`. A
	// local clone copies (or hardlinks) the scratch repo's object files directly and then writes
	// refs pointing at them; a push moves the objects through the pack protocol, which does not
	// depend on the on-disk layout of the scratch object store. On the macOS runners the local
	// clone failed with "trying to write ref 'refs/heads/main' with nonexistent object".
	if err := runGit("", "init", gitQuiet, "--bare", bareDir); err != nil {
		return fmt.Errorf("gitmirror: init bare mirror: %w", err)
	}
	if err := runGit(work, "push", gitQuiet, bareDir, "HEAD:refs/heads/main"); err != nil {
		return fmt.Errorf("gitmirror: push to bare mirror: %w", err)
	}
	if err := runGit(bareDir, "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		return fmt.Errorf("gitmirror: set mirror HEAD: %w", err)
	}
	return nil
}

// gitStatus returns `git status --short --branch` for dir, for error diagnostics only.
func gitStatus(dir string) string {
	cmd := exec.Command("git", "status", "--short", "--branch")
	cmd.Dir = dir
	cmd.Env = buildEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "git status failed: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}

// commitWorkingRepo turns a plain directory into a single-commit git repo on
// branch "main". `checkout -b main` right after `git init` works on every
// supported git version (HEAD is unborn, so this just names the pending
// initial branch) -- unlike `git init -b main`, which requires git 2.28+.
func commitWorkingRepo(dir string) error {
	steps := [][]string{
		{"init", gitQuiet},
		{"checkout", gitQuiet, "-b", "main"},
		{gitConfig, "user.email", "gitmirror@atmos.test"},
		{gitConfig, "user.name", "Atmos Test Git Mirror"},
		// Never sign commits in throwaway test repos: signing is slow, needs no
		// verification here, and hangs on dev machines whose global git config
		// enables commit.gpgsign (e.g. a 1Password agent).
		{gitConfig, "commit.gpgsign", "false"},
		// Never let `git commit` spawn a detached `gc --auto` in a throwaway repo.
		{gitConfig, "gc.auto", "0"},
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
// Every call site is mirror construction (init/config/commit/push), so it
// always runs with buildEnv, never the caller's global/system git config.
func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = buildEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gitmirror: git %v failed: %w: %s", args, err, out)
	}
	return nil
}

// gitEnv returns the process environment without the variables that make git operate on a
// repository other than the one in its working directory (GIT_DIR, GIT_WORK_TREE,
// GIT_INDEX_FILE, GIT_OBJECT_DIRECTORY, GIT_ALTERNATE_OBJECT_DIRECTORIES). The mirror is built
// from throwaway directories, and any of those leaking in from the caller's environment would
// silently redirect the scratch repo's objects or refs elsewhere.
func gitEnv() []string {
	env := os.Environ()
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES":
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

// buildEnv returns gitEnv, with the caller's global, system, and command-scope git configuration
// stripped, for mirror construction (git init/config/commit/push, all run via runGit). Build runs
// before TestMain points GIT_CONFIG_GLOBAL at the mirror's own generated config (see
// cli_test.go), so without this, a developer's or CI image's ambient config -- e.g.
// core.hooksPath or init.templateDir installing a hook -- could run arbitrary code during `git
// commit`/`git init` or hang Build. GIT_CONFIG_NOSYSTEM similarly excludes the machine-wide
// /etc/gitconfig, and dropping any ambient GIT_CONFIG_COUNT/KEY_n/VALUE_n as well as
// GIT_CONFIG_PARAMETERS (git's own encoding of command-scope `-c`/env config, which git reads
// regardless of the COUNT/KEY/VALUE form) prevents a stray command-scope override from doing the
// same. Neither change affects gitEnv's own callers, which construct their own environment.
func buildEnv() []string {
	env := gitEnv()
	kept := make([]string, 0, len(env)+2)
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		switch {
		case key == "GIT_CONFIG_COUNT", key == "GIT_CONFIG_PARAMETERS":
			continue
		case strings.HasPrefix(key, "GIT_CONFIG_KEY_"), strings.HasPrefix(key, "GIT_CONFIG_VALUE_"):
			continue
		}
		kept = append(kept, kv)
	}
	return append(kept, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
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
