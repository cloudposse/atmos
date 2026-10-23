package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestInstalledAtmosHook(t *testing.T) {
	root := suiteRepo(t)
	mustWrite(t, filepath.Join(root, "atmos.yaml"), `base_path: .
git:
  hooks:
    pre-commit:
      steps:
        - type: script
          interpreter: starlark
          script: |
            load("scripts/checks/entry.star", "run")
            run("staged", "commit", only = "trailing-whitespace,end-of-file-fixer")
`)
	mustWrite(t, filepath.Join(root, "file.txt"), "baseline\n")
	commit(t, root)
	git(t, root, "config", "--unset", "core.hooksPath")
	if err := install(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "file.txt"), "commit  ")
	git(t, root, "add", "file.txt")
	index := git(t, root, "diff", "--cached", "--binary")
	cmd := command(t.Context(), root, "git", "-c", "commit.gpgsign=false", "commit", "-m", "must fail")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "trailing-whitespace") || !strings.Contains(string(out), "end-of-file-fixer") {
		t.Fatalf("missing failure details: %s (%v)", out, err)
	}
	if !strings.Contains(string(out), "skipped") {
		t.Fatalf("missing skipped check summary: %s", out)
	}
	if bytes.Contains(out, []byte("\x1b[?25")) {
		t.Fatal("cursor controls in noninteractive hook output")
	}
	if git(t, root, "diff", "--cached", "--binary") != index {
		t.Fatal("hook modified index")
	}
	data, _ := os.ReadFile(filepath.Join(root, "file.txt"))
	if string(data) != "commit\n" {
		t.Fatalf("both independent formatters did not finish: %q", data)
	}
	git(t, root, "add", "file.txt")
	git(t, root, "-c", "commit.gpgsign=false", "commit", "-qm", "clean")
}

func suiteRepo(t *testing.T) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	atmos := filepath.Join(source, "build", executable("atmos"))
	if _, err := os.Stat(atmos); errors.Is(err, os.ErrNotExist) {
		t.Skip("build Atmos before running end-to-end suite tests")
	}
	root := newRepo(t)
	for _, name := range []string{".atmos-git-hooks.yaml", "scripts/checks/entry.star", "scripts/checks/checks.star", "scripts/checks/suite.star"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(root, name), string(data))
	}
	mustWrite(t, filepath.Join(root, "atmos.yaml"), "base_path: .\n")
	mustWrite(t, filepath.Join(root, ".gitignore"), "build/\n")
	if err := os.MkdirAll(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(atmos, filepath.Join(root, "build", executable("atmos"))); err != nil {
		t.Skipf("binary symlinks unavailable: %v", err)
	}
	return root
}

func TestSuiteReadOnlyFixAndIndex(t *testing.T) {
	root := suiteRepo(t)
	mustWrite(t, filepath.Join(root, "file.txt"), "baseline\n")
	commit(t, root)
	mustWrite(t, filepath.Join(root, "file.txt"), "staged  \n")
	git(t, root, "add", "file.txt")
	before := git(t, root, "diff", "--cached", "--binary")
	err := runSuite(t.Context(), root, "staged", "check", "origin/main", "trailing-whitespace,end-of-file-fixer")
	if err == nil {
		t.Fatal("check accepted formatting difference")
	}
	data, _ := os.ReadFile(filepath.Join(root, "file.txt"))
	if string(data) != "staged  \n" {
		t.Fatal("check modified live file")
	}
	err = runSuite(t.Context(), root, "staged", "commit", "origin/main", "trailing-whitespace,end-of-file-fixer")
	if err == nil {
		t.Fatal("commit with fixes should fail")
	}
	data, _ = os.ReadFile(filepath.Join(root, "file.txt"))
	if string(data) != "staged\n" {
		t.Fatal("fix not applied", string(data))
	}
	if git(t, root, "diff", "--cached", "--binary") != before {
		t.Fatal("index changed")
	}
	git(t, root, "add", "file.txt")
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "trailing-whitespace,end-of-file-fixer"); err != nil {
		t.Fatal(err)
	}
}

func TestSuiteExclusionsAndFormatting(t *testing.T) {
	root := suiteRepo(t)
	protected := []string{"tests/snapshots/a.golden", "tests/testdata/a.txt", "tests/test-cases/a.txt", "vendor/a.txt", "image.svg", "website/src/components/Screengrabs/a.txt"}
	for _, name := range append(protected, "ordinary.txt") {
		mustWrite(t, filepath.Join(root, filepath.FromSlash(name)), "text  \n\n")
	}
	git(t, root, "add", ".")
	if err := runSuite(t.Context(), root, "staged", "format", "origin/main", "trailing-whitespace,end-of-file-fixer"); err != nil {
		t.Fatal(err)
	}
	for _, name := range protected {
		data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if !bytes.Equal(data, []byte("text  \n\n")) {
			t.Fatal("protected file changed", name)
		}
	}
	data, _ := os.ReadFile(filepath.Join(root, "ordinary.txt"))
	if string(data) != "text\n" {
		t.Fatal(string(data))
	}
}

func TestSuiteEmptySelection(t *testing.T) {
	root := suiteRepo(t)
	commit(t, root)
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "trailing-whitespace,end-of-file-fixer"); err != nil {
		t.Fatal(err)
	}
}

func TestSuiteMissingFormatterFailsWithoutEditing(t *testing.T) {
	root := suiteRepo(t)
	mustWrite(t, filepath.Join(root, "main.go"), "package main\nfunc main(){}\n")
	git(t, root, "add", ".")
	bin := t.TempDir()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitBin, filepath.Join(bin, executable("git"))); err != nil {
		t.Skip(err)
	}
	t.Setenv("PATH", bin)
	if err := runSuite(t.Context(), root, "staged", "format", "origin/main", "go-fumpt"); err == nil {
		t.Fatal("missing gofumpt was ignored")
	}
	data, _ := os.ReadFile(filepath.Join(root, "main.go"))
	if string(data) != "package main\nfunc main(){}\n" {
		t.Fatal("failed formatter changed live file")
	}
}

func TestSuiteGitleaksPRRange(t *testing.T) {
	if _, err := exec.LookPath("gitleaks"); err != nil {
		t.Skip("install pinned Gitleaks for the secrets integration test")
	}
	root := suiteRepo(t)
	mustWrite(t, filepath.Join(root, ".gitleaks.toml"), `title = "Synthetic test rule"
[[rules]]
id = "test-secret"
description = "Synthetic marker, not a credential"
regex = '''ATMOS_TEST_SECRET_[A-Z0-9]{16}'''
`)
	commit(t, root)
	base := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	mustWrite(t, filepath.Join(root, "file.txt"), "ordinary content\n")
	commit(t, root)
	if err := runSuite(t.Context(), root, "pr", "check", base, "gitleaks"); err != nil {
		t.Fatal("clean PR failed:", err)
	}
	// Assemble the synthetic marker so the source does not itself resemble a secret.
	mustWrite(t, filepath.Join(root, "file.txt"), "ATMOS_TEST_"+"SECRET_"+strings.Repeat("A1B2", 4)+"\n")
	commit(t, root)
	if got := git(t, root, "diff", "--cached", "--name-only"); got != "" {
		t.Fatal("test must use an empty CI index")
	}
	if err := runSuite(t.Context(), root, "pr", "check", base, "gitleaks"); err == nil {
		t.Fatal("PR commit secret escaped detection")
	}
}

func executable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func command(ctx context.Context, dir, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = CleanEnvironment(os.Environ())
	return cmd
}

func install(ctx context.Context, root string) error {
	cmd := command(ctx, root, filepath.Join(root, "build", executable("atmos")), "git", "hooks", "install", "pre-commit")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(string(out) + err.Error())
	}
	return nil
}

func runSuite(ctx context.Context, root, scope, mode, base, only string, skips ...string) error {
	_, err := runSuiteOutput(ctx, root, scope, mode, base, only, skips...)
	return err
}

func runSuiteOutput(ctx context.Context, root, scope, mode, base, only string, skips ...string) (string, error) {
	skip := ""
	if len(skips) > 0 {
		skip = skips[0]
	}
	script := filepath.Join(root, ".git", "run-check.star")
	source := "load(" + strconv.Quote(filepath.Join(root, "scripts", "checks", "entry.star")) + ", \"run\")\nrun(" + strconv.Quote(scope) + ", " + strconv.Quote(mode) + ", " + strconv.Quote(base) + ", " + strconv.Quote(only) + ", " + strconv.Quote(skip) + ")\n"
	if err := os.WriteFile(script, []byte(source), 0o600); err != nil {
		return "", err
	}
	cmd := command(ctx, root, filepath.Join(root, "build", executable("atmos")), script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), errors.New(string(out) + err.Error())
	}
	return string(out), nil
}

func TestSuiteOnlyOmitsUnrequestedChecks(t *testing.T) {
	root := suiteRepo(t)
	commit(t, root)
	base := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	out, err := runSuiteOutput(t.Context(), root, "pr", "check", base, "trailing-whitespace,end-of-file-fixer,check-yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"No files selected", "committed changes", "trailing-whitespace", "end-of-file-fixer", "check-yaml", "3 skipped"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("missing %q in output: %s", expected, out)
		}
	}
	for _, excluded := range []string{"branch-file-limit", "go-fumpt", "gitleaks", "golangci-lint", "gomodcheck", "check-symlinks", "check-added-large-files"} {
		if strings.Contains(out, excluded) {
			t.Fatalf("unrequested check %q in output: %s", excluded, out)
		}
	}
}

func TestSuiteTextFormattingAndIdempotence(t *testing.T) {
	root := suiteRepo(t)
	cases := map[string]struct{ input, want string }{
		"spaces":  {"hello \t\nworld\t", "hello\nworld\n"},
		"crlf":    {"hello \r\nworld\t\r\n", "hello\r\nworld\r\n"},
		"extra":   {"hello\n\n", "hello\n"},
		"blank":   {"\n\n", ""},
		"empty":   {"", ""},
		"binary":  {"\x00hello \n", "\x00hello \n"},
		"nonutf8": {"\xff", "\xff"},
		platformFilename("{{x}} with space\nand newline"): {"template \n", "template\n"},
	}
	for name, tc := range cases {
		mustWrite(t, filepath.Join(root, name), tc.input)
	}
	git(t, root, "add", ".")
	index := git(t, root, "ls-files", "-s")
	if err := runSuite(t.Context(), root, "all", "format", "origin/main", "trailing-whitespace,end-of-file-fixer"); err != nil {
		t.Fatal(err)
	}
	for name, tc := range cases {
		data, _ := os.ReadFile(filepath.Join(root, name))
		if string(data) != tc.want {
			t.Fatalf("%s: got %q want %q", name, data, tc.want)
		}
	}
	if err := runSuite(t.Context(), root, "all", "check", "origin/main", "trailing-whitespace,end-of-file-fixer"); err != nil {
		t.Fatal("not idempotent:", err)
	}
	if got := git(t, root, "ls-files", "-s"); got != index {
		t.Fatal("index changed")
	}
}

func TestSuiteLargeFileBoundaryAndLFS(t *testing.T) {
	root := suiteRepo(t)
	path := filepath.Join(root, "limit")
	mustWrite(t, path, strings.Repeat("a", 500*1024))
	git(t, root, "add", ".")
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "check-added-large-files"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, strings.Repeat("a", 500*1024+1))
	git(t, root, "add", ".")
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "check-added-large-files"); err == nil {
		t.Fatal("oversized accepted")
	}
	mustWrite(t, filepath.Join(root, ".gitattributes"), "limit filter=lfs\n")
	git(t, root, "add", ".gitattributes")
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "check-added-large-files"); err != nil {
		t.Fatal("LFS rejected:", err)
	}
}

func TestInstallPreservesConfiguredHooksPath(t *testing.T) {
	for _, force := range []bool{false, true} {
		for _, absolute := range []bool{false, true} {
			t.Run("force="+strconv.FormatBool(force)+"/absolute="+strconv.FormatBool(absolute), func(t *testing.T) {
				root := suiteRepo(t)
				mustWrite(t, filepath.Join(root, "atmos.yaml"), "base_path: .\ngit:\n  hooks:\n    pre-commit:\n      command: echo checks\n    pre-push:\n      command: echo checks\n")
				hooksPath := "managed-hooks"
				if absolute {
					hooksPath = filepath.Join(root, hooksPath)
				}
				git(t, root, "config", "core.hooksPath", hooksPath)
				original := "#!/bin/sh\n# managed by another tool\n"
				hook := filepath.Join(root, "managed-hooks", "pre-commit")
				mustWrite(t, hook, original)
				args := []string{"git", "hooks", "install"}
				if force {
					args = append(args, "--force")
				}
				cmd := command(t.Context(), root, filepath.Join(root, "build", executable("atmos")), args...)
				out, err := cmd.CombinedOutput()
				if err == nil || !strings.Contains(string(out), "core.hooksPath") {
					t.Fatalf("expected hooksPath conflict: %v\n%s", err, out)
				}
				data, err := os.ReadFile(hook)
				if err != nil || string(data) != original {
					t.Fatalf("existing hook changed: %v %q", err, data)
				}
				for _, dir := range []string{"managed-hooks", ".git/hooks"} {
					if _, err := os.Stat(filepath.Join(root, dir, "pre-push")); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("installed hook despite conflict in %s: %v", dir, err)
					}
				}
				if got := strings.TrimSpace(git(t, root, "config", "--get", "core.hooksPath")); got != hooksPath {
					t.Fatalf("changed core.hooksPath: %q", got)
				}
			})
		}
	}
}

func TestBranchFileLimitIncludesPendingCommit(t *testing.T) {
	root := suiteRepo(t)
	commit(t, root)
	base := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	for i := range 149 {
		mustWrite(t, filepath.Join(root, "files", strconv.Itoa(i)), "added\n")
	}
	commit(t, root)
	// One additional staged file reaches the boundary. Untracked/unstaged files do not count.
	mustWrite(t, filepath.Join(root, "staged"), "pending\n")
	git(t, root, "add", "staged")
	mustWrite(t, filepath.Join(root, "untracked"), "not in commit\n")
	if err := runSuite(t.Context(), root, "staged", "check", base, "branch-file-limit"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "one too many"), "pending\n")
	git(t, root, "add", "one too many")
	index := git(t, root, "ls-files", "-s")
	if err := runSuite(t.Context(), root, "staged", "commit", base, "branch-file-limit"); err == nil || !strings.Contains(err.Error(), "151 files") {
		t.Fatal(err)
	}
	if index != git(t, root, "ls-files", "-s") {
		t.Fatal("limit check changed the index")
	}
	// Undoing a branch addition reduces the net branch diff, rather than counting history.
	git(t, root, "rm", "files/0")
	if err := runSuite(t.Context(), root, "staged", "check", base, "branch-file-limit"); err != nil {
		t.Fatal(err)
	}
	if err := runSuite(t.Context(), root, "staged", "check", "absent-base", "branch-file-limit"); err == nil || !strings.Contains(err.Error(), "comparison base") {
		t.Fatal(err)
	}
}

func TestBranchFileLimitCountsRenamesAndDeletions(t *testing.T) {
	root := suiteRepo(t)
	for i := range 150 {
		mustWrite(t, filepath.Join(root, "files", strconv.Itoa(i)), "unique "+strconv.Itoa(i)+"\n")
	}
	commit(t, root)
	base := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	git(t, root, "mv", "files/0", platformFilename("renamed\nfile"))
	for i := 1; i < 150; i++ {
		if err := os.Remove(filepath.Join(root, "files", strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}
	git(t, root, "add", "-u")
	if err := runSuite(t.Context(), root, "staged", "check", base, "branch-file-limit"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "extra"), "new\n")
	git(t, root, "add", "extra")
	if err := runSuite(t.Context(), root, "staged", "check", base, "branch-file-limit"); err == nil || !strings.Contains(err.Error(), "151 files") {
		t.Fatal(err)
	}
}

func TestBranchFileLimitInitialCommit(t *testing.T) {
	root := suiteRepo(t)
	git(t, root, "add", ".")
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "branch-file-limit"); err != nil {
		t.Fatal(err)
	}
}

func TestSuiteExplicitSkipAndYAMLSyntax(t *testing.T) {
	root := suiteRepo(t)
	mustWrite(t, filepath.Join(root, "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "valid.yaml"), "a: !custom value\n---\nb: !env OTHER\n")
	mustWrite(t, filepath.Join(root, "tests", "fixtures", "invalid.yaml"), "a: [invalid\n")
	git(t, root, "add", ".")
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "golangci-lint,check-yaml", "golangci-lint"); err != nil {
		t.Fatal(err)
	}
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "check-yaml", "unknown"); err == nil {
		t.Fatal("unknown skip accepted")
	}
	mustWrite(t, filepath.Join(root, "valid.yaml"), "a: valid\n---\nb: [invalid\n")
	git(t, root, "add", "valid.yaml")
	if err := runSuite(t.Context(), root, "staged", "check", "origin/main", "check-yaml"); err == nil {
		t.Fatal("invalid later YAML document accepted")
	}
}
