//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/magefile/mage/sh"
)

// gitCmd is the git binary name, shared by the git plumbing calls below.
const gitCmd = "git"

// dirPerm is the permission mode used for the isolated cache/tmp directory
// created below.
const dirPerm = 0o755

// stagedPatch bundles the staged-changes patch file path and the affected
// package list returned by buildStagedPatchAndPackages.
type stagedPatch struct {
	path     string
	packages []string
}

// runGolangciLintPrecommit runs the pre-built custom-gcl binary scoped to
// staged/changed Go files, porting scripts/run-custom-golangci-lint.sh: per-
// worktree cache/tmp isolation, arg assembly, and the staged-vs-new-from-rev
// branching. The binPath argument must already exist; this function never
// builds it.
func runGolangciLintPrecommit(root, binPath string) error {
	env, err := setWorktreeIsolationEnv(root)
	if err != nil {
		return err
	}

	args := golangciLintArgs()

	if shouldUseNewFromRev(root) {
		rev := os.Getenv("GOLANGCI_NEW_FROM_REV")
		if rev == "" {
			rev = "origin/main"
		}
		args = append(args, "--new-from-rev="+rev)
		return sh.RunWithV(env, binPath, args...)
	}

	tmpDir := env["TMPDIR"]
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	patch, cleanup, err := buildStagedPatchAndPackages(root, tmpDir)
	if err != nil {
		return err
	}
	defer cleanup()

	args = append(args, "--new-from-patch="+patch.path)
	args = append(args, patch.packages...)
	return sh.RunWithV(env, binPath, args...)
}

// setWorktreeIsolationEnv computes the per-worktree GOLANGCI_LINT_CACHE/temp
// directory overrides that keep parallel worktree lints from serializing on
// golangci-lint's machine-global single-instance lock file (a fixed path
// under os.TempDir()). Returns an empty map (no overrides) if the caller
// opted back into the shared machine-global cache via
// ATMOS_LINT_SHARED_CACHE=1.
//
// Unlike the previous bash (which only pointed TMPDIR at the isolated dir),
// this also sets TMP/TEMP on Windows: Go's os.TempDir() ignores TMPDIR on
// Windows and checks TMP/TEMP/USERPROFILE instead, so the bash version's
// isolation silently didn't apply there. This is a deliberate behavior fix,
// not just a port.
func setWorktreeIsolationEnv(root string) (map[string]string, error) {
	env := map[string]string{}
	if os.Getenv("ATMOS_LINT_SHARED_CACHE") == "1" {
		return env, nil
	}

	cacheDir := os.Getenv("GOLANGCI_LINT_CACHE")
	if cacheDir == "" {
		cacheDir = filepath.Join(root, ".golangci-cache")
	}
	env["GOLANGCI_LINT_CACHE"] = cacheDir

	tmpDir := filepath.Join(root, ".golangci-tmp")
	if err := os.MkdirAll(tmpDir, dirPerm); err != nil {
		return nil, fmt.Errorf("mage: mkdir %s: %w", tmpDir, err)
	}
	env["TMPDIR"] = tmpDir
	if runtime.GOOS == "windows" {
		env["TMP"] = tmpDir
		env["TEMP"] = tmpDir
	}
	return env, nil
}

// golangciLintArgs assembles the shared `run` args: config, optional
// concurrency override, and the serial/parallel-runner lock flags.
func golangciLintArgs() []string {
	args := []string{"run", "--config=.golangci.yml"}
	if c := os.Getenv("GOLANGCI_CONCURRENCY"); c != "" {
		args = append(args, "--concurrency="+c)
	}
	// --allow-serial-runners: within a single checkout, queue concurrent runs
	// around the cache lock (wait) instead of failing fast.
	args = append(args, "--allow-serial-runners")
	// --allow-parallel-runners drops the lock entirely, so it stays opt-in.
	if p := os.Getenv("GOLANGCI_ALLOW_PARALLEL"); p != "" && p != "0" {
		args = append(args, "--allow-parallel-runners")
	}
	return args
}

// shouldUseNewFromRev reports whether the run should scope findings via
// --new-from-rev (no staged .go changes, or an in-progress merge) rather
// than a staged-changes patch.
func shouldUseNewFromRev(root string) bool {
	if gitDiffCachedQuiet(root) {
		return true
	}
	return gitHasMergeHead(root)
}

// gitDiffCachedQuiet reports whether there are no staged .go changes.
func gitDiffCachedQuiet(root string) bool {
	cmd := exec.Command(gitCmd, "diff", "--cached", "--quiet", "--", "*.go")
	cmd.Dir = root
	return cmd.Run() == nil
}

// gitHasMergeHead reports whether a merge is in progress.
func gitHasMergeHead(root string) bool {
	cmd := exec.Command(gitCmd, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	cmd.Dir = root
	return cmd.Run() == nil
}

// buildStagedPatchAndPackages writes the staged .go diff to a temp file
// under tmpDir and computes the deduped, sorted list of affected package
// directories, mirroring the bash `git diff --cached --binary` / `git diff
// --cached --name-only --diff-filter=ACMR` pipeline. The returned cleanup
// func removes the patch file; callers must defer it.
func buildStagedPatchAndPackages(root, tmpDir string) (stagedPatch, func(), error) {
	patchFile, err := os.CreateTemp(tmpDir, "atmos-golangci-staged.*")
	if err != nil {
		return stagedPatch{}, nil, fmt.Errorf("mage: create patch temp file: %w", err)
	}
	// patchFile.Name() is the path os.CreateTemp itself just generated under
	// tmpDir, not externally supplied input.
	cleanup := func() { _ = os.Remove(patchFile.Name()) } //nolint:gosec // path comes from os.CreateTemp, not external input

	diffCmd := exec.Command(gitCmd, "diff", "--cached", "--binary", "--", "*.go")
	diffCmd.Dir = root
	diffCmd.Stdout = patchFile
	diffCmd.Stderr = os.Stderr
	if runErr := diffCmd.Run(); runErr != nil {
		_ = patchFile.Close()
		cleanup()
		return stagedPatch{}, nil, fmt.Errorf("mage: git diff --cached --binary: %w", runErr)
	}
	if closeErr := patchFile.Close(); closeErr != nil {
		cleanup()
		return stagedPatch{}, nil, fmt.Errorf("mage: close patch file: %w", closeErr)
	}

	namesCmd := exec.Command(gitCmd, "diff", "--cached", "--name-only", "--diff-filter=ACMR", "--", "*.go")
	namesCmd.Dir = root
	out, outErr := namesCmd.Output()
	if outErr != nil {
		cleanup()
		return stagedPatch{}, nil, fmt.Errorf("mage: git diff --cached --name-only: %w", outErr)
	}

	packages := uniqueSortedPackageDirs(strings.Split(strings.TrimSpace(string(out)), "\n"))
	return stagedPatch{path: patchFile.Name(), packages: packages}, cleanup, nil
}

// uniqueSortedPackageDirs converts a list of git-relative file paths (always
// forward-slash, regardless of OS) into a deduped, sorted list of Go package
// patterns, e.g. "pkg/foo/bar.go" -> "./pkg/foo", "main.go" -> ".".
func uniqueSortedPackageDirs(files []string) []string {
	seen := map[string]struct{}{}
	for _, file := range files {
		if file == "" {
			continue
		}
		dir := path.Dir(file)
		pkg := "./" + dir
		if dir == "." {
			pkg = "."
		}
		seen[pkg] = struct{}{}
	}
	pkgs := make([]string, 0, len(seen))
	for pkg := range seen {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return pkgs
}
