package starlark

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/script"
)

func sandboxRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip(err)
	}
	// Both the fixture and its private sandbox must use LF regardless of host Git settings.
	config := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(config, []byte("[core]\n\tautocrlf = false\n\teol = lf\n"), 0o600))
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	sandboxGit(t, dir, "init", "-q")
	sandboxGit(t, dir, "config", "user.name", "Test")
	sandboxGit(t, dir, "config", "user.email", "test@localhost")
	sandboxGit(t, dir, "config", "core.hooksPath", filepath.Join(dir, ".git", "disabled"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("staged  \n"), 0o755))
	sandboxGit(t, dir, "add", ".")
	return dir
}

func sandboxGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	result, err := atmosgit.NewExecRunner().Run(t.Context(), "git", args, atmosgit.RunOptions{Dir: dir})
	require.NoError(t, err)
	return result.Stdout
}

func TestGitSandboxCallbackScopeAndFailure(t *testing.T) {
	for _, fixes := range []string{"none", "unstaged"} {
		t.Run(fixes, func(t *testing.T) {
			root := sandboxRepo(t)
			index := sandboxGit(t, root, "ls-files", "-s")
			var log bytes.Buffer
			_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, ProcessEnv: os.Environ(), Stderr: &log, Source: `
def check(box):
    if fs.resolve(".") != box.path:
        fail("wrong filesystem directory")
    if fs.resolve(exec.run(["git", "rev-parse", "--show-toplevel"], output="capture").stdout.strip()) != box.path:
        fail("wrong command directory")
    fs.write_file("file.txt", "fixed\n", overwrite=True)
    fail("another check failed")
git.sandbox(run=check, fixes="` + fixes + `")
`})
			require.ErrorContains(t, err, "another check failed")
			require.Contains(t, log.String(), "Modified by checks")
			require.Contains(t, log.String(), "Selecting files for isolated checks")
			require.Contains(t, log.String(), "Copying staged files")
			require.NotContains(t, log.String(), "\x1b[?25", "captured preparation must not run an interactive renderer")
			data, err := os.ReadFile(filepath.Join(root, "file.txt"))
			require.NoError(t, err)
			want := "staged  \n"
			if fixes == "unstaged" {
				want = "fixed\n"
			}
			require.Equal(t, want, string(data))
			require.Equal(t, index, sandboxGit(t, root, "ls-files", "-s"))
		})
	}
}

func TestGitSandboxParallelAndDeferredScope(t *testing.T) {
	root := sandboxRepo(t)
	result, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, ProcessEnv: os.Environ(), Source: `
def branch():
    return fs.read_file("file.txt")
def check(box):
    fs.write_file("file.txt", "temporary\n", overwrite=True)
    defer(fs.write_file, "file.txt", "deferred\n", overwrite=True)
    return steps.parallel(functions=[branch, branch])
values = git.sandbox(run=check, fixes="unstaged", fail_on_changes=False)
output = [values, fs.read_file("file.txt")]
`})
	require.NoError(t, err)
	require.JSONEq(t, `[["temporary\n","temporary\n"],"deferred\n"]`, result.Value)
}

func TestGitSandboxCancellationAndCleanup(t *testing.T) {
	root := sandboxRepo(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var temporary string
	engine := New(WithWriteFile(func(path string, data []byte, mode os.FileMode) error {
		temporary = filepath.Dir(path)
		err := atomicWriteFile(path, data, mode)
		cancel()
		return err
	}))
	_, err := engine.Execute(ctx, script.Spec{WorkingDirectory: root, ProcessEnv: os.Environ(), Source: `
def check(box):
    fs.write_file("file.txt", "cancelled\n", overwrite=True)
git.sandbox(run=check, fixes="unstaged")
`})
	require.Error(t, err)
	data, err := os.ReadFile(filepath.Join(root, "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "staged  \n", string(data))
	_, err = os.Stat(temporary)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestGitSandboxArgumentValidation(t *testing.T) {
	for _, source := range []string{
		`git.sandbox()`, `git.sandbox(run=1)`, `git.sandbox(run=lambda s: None, fixes="stage")`,
		`git.sandbox(run=lambda s: None, scope="invalid")`,
	} {
		_, err := New().Execute(t.Context(), script.Spec{Source: source})
		require.Error(t, err, source)
	}
	root := sandboxRepo(t)
	for _, body := range []string{`s.track(["../escape"])`, `s.copy(["missing"])`, `git.sandbox(run=lambda s: None)`} {
		_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, ProcessEnv: os.Environ(), Source: "def check(s):\n    " + body + "\ngit.sandbox(run=check)"})
		require.Error(t, err, body)
	}
}

func TestSandboxReadOnlyDoesNotChangeProcessDirectory(t *testing.T) {
	root := sandboxRepo(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	result, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, ProcessEnv: os.Environ(), Source: `
def check(box):
    return [box.root, box.scope, box.files]
output = git.sandbox(run=check)
`})
	require.NoError(t, err)
	require.Contains(t, result.Value, "file.txt")
	after, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, cwd, after)
	require.False(t, strings.Contains(result.Value, "atmos-git-sandbox-"))
}
