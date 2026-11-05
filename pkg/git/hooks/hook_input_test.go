package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestShimContentFor(t *testing.T) {
	t.Run("pins the installing binary and falls back to PATH", func(t *testing.T) {
		content := ShimContentFor("pre-push", "/opt/atmos/bin/atmos")
		assert.Contains(t, content, "#!/bin/sh")
		assert.Contains(t, content, ShimMarker)
		assert.Contains(t, content, "atmos_bin='/opt/atmos/bin/atmos'")
		assert.Contains(t, content, `[ -x "$atmos_bin" ] || atmos_bin=atmos`)
		assert.Contains(t, content, `exec "$atmos_bin" git hooks run pre-push "$@"`)
	})
	t.Run("quotes a path with spaces and quotes", func(t *testing.T) {
		content := ShimContentFor("pre-commit", "/Users/o'brien/my tools/atmos")
		assert.Contains(t, content, `atmos_bin='/Users/o'\''brien/my tools/atmos'`)
	})
	t.Run("an unknown path yields the PATH-only shim", func(t *testing.T) {
		assert.Equal(t, ShimContent("pre-commit"), ShimContentFor("pre-commit", ""))
		assert.Contains(t, ShimContent("pre-commit"), `exec atmos git hooks run pre-commit "$@"`)
	})
}

func TestInstalledAtmosPathIsAbsolute(t *testing.T) {
	path := installedAtmosPath()
	require.NotEmpty(t, path)
	assert.True(t, filepath.IsAbs(filepath.FromSlash(path)), path)
}

func TestInstallRewritesAShimThatPointsAtAnotherBinary(t *testing.T) {
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Command: "true"}}}
	dest := filepath.Join(repoHooksDir(repoDir), "pre-commit")
	original := installedAtmosPath
	t.Cleanup(func() { installedAtmosPath = original })

	installedAtmosPath = func() string { return "/old/location/atmos" }
	require.NoError(t, Install(t.Context(), cfg, nil, false))
	content, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Contains(t, string(content), "atmos_bin='/old/location/atmos'")

	installedAtmosPath = func() string { return "/new/location/atmos" }
	require.NoError(t, Install(t.Context(), cfg, nil, false))
	content, err = os.ReadFile(dest)
	require.NoError(t, err)
	assert.Contains(t, string(content), "atmos_bin='/new/location/atmos'", "an Atmos-managed shim follows the binary that installs it")
	assert.NotContains(t, string(content), "/old/location/atmos")

	installedAtmosPath = func() string { return "" }
	require.NoError(t, Install(t.Context(), cfg, nil, false))
	content, err = os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, ShimContent("pre-commit"), string(content), "without a known path the shim relies on PATH")
}

func TestHookEnvironment(t *testing.T) {
	assert.Equal(t, []string{"ATMOS_GIT_HOOK_ARGS="}, hookEnvironment(nil, ""))
	assert.Equal(t, []string{"ATMOS_GIT_HOOK_ARGS=origin git@example.com:org/repo.git", "ATMOS_GIT_HOOK_STDIN=/tmp/in"},
		hookEnvironment([]string{"origin", "git@example.com:org/repo.git"}, "/tmp/in"))
}

func TestRunSteps_ArgumentsReachShellSteps(t *testing.T) {
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"commit-msg": {Steps: schema.Tasks{
		shellStep("show", `echo "count=$# first=[$1] all=[$*] env=[$ATMOS_GIT_HOOK_ARGS]"; for a in "$@"; do echo "arg=[$a]"; done`),
	}}}}

	var stdout, stderr bytes.Buffer
	err := Run(cfg, "commit-msg", []string{"msg file.txt", "second"}, WithOutputWriters(&stdout, &stderr))
	require.NoError(t, err, stderr.String())
	out := stdout.String()
	assert.Contains(t, out, "count=2 first=[msg file.txt] all=[msg file.txt second] env=[msg file.txt second]")
	assert.Contains(t, out, "arg=[msg file.txt]\n")
	assert.Contains(t, out, "arg=[second]\n", "arguments with spaces stay whole as positional parameters")
}

func TestRunSteps_PrePushStdinIsCapturedOnce(t *testing.T) {
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-push": {Steps: schema.Tasks{
		shellStep("direct", `read line; echo "first step read: $line"`),
		shellStep("second", `read line; echo "second step read: [$line]"`),
		shellStep("file", `read line < "$ATMOS_GIT_HOOK_STDIN"; echo "file: $line"; echo "$ATMOS_GIT_HOOK_STDIN"`),
	}}}}

	var stdout, stderr bytes.Buffer
	err := Run(cfg, "pre-push", []string{"origin", "url"},
		WithStdin(strings.NewReader("refs/heads/main 1111 refs/heads/main 0000\nsecond line\n")),
		WithOutputWriters(&stdout, &stderr))
	require.NoError(t, err, stderr.String())
	out := stdout.String()
	assert.Contains(t, out, "first step read: refs/heads/main 1111 refs/heads/main 0000")
	assert.Contains(t, out, "second step read: []", "only the first shell step receives the piped input")
	assert.Contains(t, out, "file: refs/heads/main 1111 refs/heads/main 0000", "later steps read the captured copy")

	lines := strings.Split(strings.TrimSpace(out), "\n")
	path := lines[len(lines)-1]
	assert.NoFileExists(t, path, "the captured copy is removed when the hook ends")
}

func TestRunSteps_StdinIsOnlyCapturedForHooksGitWritesTo(t *testing.T) {
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{
		shellStep("show", `echo "stdin file: [${ATMOS_GIT_HOOK_STDIN-unset}]"`),
	}}}}

	var stdout, stderr bytes.Buffer
	err := Run(cfg, "pre-commit", nil, WithStdin(strings.NewReader("ignored")), WithOutputWriters(&stdout, &stderr))
	require.NoError(t, err, stderr.String())
	assert.Contains(t, stdout.String(), "stdin file: [unset]")
}

func TestCaptureHookStdin(t *testing.T) {
	t.Run("a terminal is never read", func(t *testing.T) {
		original := stdinIsTerminal
		t.Cleanup(func() { stdinIsTerminal = original })
		stdinIsTerminal = func() bool { return true }
		input, err := captureHookStdin("pre-push", nil)
		require.NoError(t, err)
		assert.Nil(t, input.stdin)
		assert.Empty(t, input.path)
		input.cleanup()
	})
	t.Run("empty input still produces an empty file", func(t *testing.T) {
		input, err := captureHookStdin("pre-push", strings.NewReader(""))
		require.NoError(t, err)
		t.Cleanup(input.cleanup)
		require.NotEmpty(t, input.path)
		data, err := os.ReadFile(input.path)
		require.NoError(t, err)
		assert.Empty(t, data)
	})
	t.Run("the file is private to the user", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows does not use Unix permission bits")
		}
		input, err := captureHookStdin("pre-push", strings.NewReader("x"))
		require.NoError(t, err)
		t.Cleanup(input.cleanup)
		info, err := os.Stat(input.path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(stdinFileMode), info.Mode().Perm())
	})
}

// gitIn runs git in dir with the test identity and returns its combined output.
func gitIn(t *testing.T, dir string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", gitTestArgs(args...)...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com"), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// installFakeAtmosHooks writes the configuration for the fake atmos, installs shims for every hook
// in cfg, and returns the environment git must run with so the shims reach the test binary.
func installFakeAtmosHooks(t *testing.T, repoDir string, cfg *schema.GitConfig) []string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	configPath := filepath.Join(t.TempDir(), "git-config.json")
	require.NoError(t, os.WriteFile(configPath, data, 0o600))

	original := installedAtmosPath
	t.Cleanup(func() { installedAtmosPath = original })
	installedAtmosPath = func() string { return filepath.ToSlash(exe) }
	t.Chdir(repoDir)
	require.NoError(t, Install(t.Context(), cfg, nil, false))
	return []string{fakeAtmosEnv + "=1", fakeAtmosConfigEnv + "=" + configPath}
}

// A real Git client runs the installed shim: commit-msg sees its file as $1, a failing hook stops
// the commit, and pre-push sees the refs on stdin and the remote as its arguments.
func TestInstalledShimsRunUnderGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shim is a POSIX shell script; Git for Windows hook execution is covered by CI shell semantics")
	}
	repoDir := initTempRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	_, err := gitIn(t, repoDir, nil, "init", "--bare", "-b", "main", remote)
	require.NoError(t, err)
	_, err = gitIn(t, repoDir, nil, "remote", "add", "origin", remote)
	require.NoError(t, err)

	out := t.TempDir()
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{
		"commit-msg": {Steps: schema.Tasks{shellStep("prefix", `read first < "$1"; case "$first" in feat:*) echo ok > "`+filepath.ToSlash(filepath.Join(out, "msg-ok"))+`";; *) echo "bad prefix: $first" >&2; exit 3;; esac`)}},
		"pre-push": {Steps: schema.Tasks{
			shellStep("direct", `cat > "`+filepath.ToSlash(filepath.Join(out, "stdin-direct"))+`"`),
			shellStep("file", `cat "$ATMOS_GIT_HOOK_STDIN" > "`+filepath.ToSlash(filepath.Join(out, "stdin-file"))+`"`),
			shellStep("args", `echo "$1|$ATMOS_GIT_HOOK_ARGS" > "`+filepath.ToSlash(filepath.Join(out, "push-args"))+`"`),
		}},
	}}
	env := installFakeAtmosHooks(t, repoDir, cfg)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "a.txt"), []byte("a"), 0o644))
	_, err = gitIn(t, repoDir, env, "add", "a.txt")
	require.NoError(t, err)

	output, err := gitIn(t, repoDir, env, "commit", "-m", "wrong prefix")
	require.Error(t, err, "a commit-msg hook that exits non-zero rejects the commit")
	assert.Contains(t, output, "bad prefix: wrong prefix")

	output, err = gitIn(t, repoDir, env, "commit", "-m", "feat: right prefix")
	require.NoError(t, err, output)
	assert.FileExists(t, filepath.Join(out, "msg-ok"))

	output, err = gitIn(t, repoDir, env, "push", "origin", "main")
	require.NoError(t, err, output)
	direct, err := os.ReadFile(filepath.Join(out, "stdin-direct"))
	require.NoError(t, err)
	assert.Contains(t, string(direct), "refs/heads/main", "the first shell step still reads the refs from stdin")
	captured, err := os.ReadFile(filepath.Join(out, "stdin-file"))
	require.NoError(t, err)
	assert.Equal(t, string(direct), string(captured), "a later step reads the same data from ATMOS_GIT_HOOK_STDIN")
	args, err := os.ReadFile(filepath.Join(out, "push-args"))
	require.NoError(t, err)
	assert.Equal(t, "origin|origin "+remote, strings.TrimSpace(string(args)))
}

// When the recorded binary is gone, the shim runs atmos from PATH.
func TestShimFallsBackToPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shim is a POSIX shell script")
	}
	repoDir := initTempRepo(t)
	out := filepath.Join(t.TempDir(), "ran")
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{
		"pre-commit": {Steps: schema.Tasks{shellStep("mark", `echo ran > "`+filepath.ToSlash(out)+`"`)}},
	}}
	env := installFakeAtmosHooks(t, repoDir, cfg)

	// Point the shim at a binary that does not exist and put a copy of the test binary named atmos on PATH.
	hook := filepath.Join(repoHooksDir(repoDir), "pre-commit")
	content, err := os.ReadFile(hook)
	require.NoError(t, err)
	exe, err := os.Executable()
	require.NoError(t, err)
	missing := strings.ReplaceAll(string(content), filepath.ToSlash(exe), filepath.ToSlash(filepath.Join(t.TempDir(), "gone", "atmos")))
	require.NotEqual(t, string(content), missing)
	require.NoError(t, os.WriteFile(hook, []byte(missing), 0o755))

	binDir := t.TempDir()
	data, err := os.ReadFile(exe)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "atmos"), data, 0o755))
	env = append(env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "b.txt"), []byte("b"), 0o644))
	_, err = gitIn(t, repoDir, env, "add", "b.txt")
	require.NoError(t, err)
	output, err := gitIn(t, repoDir, env, "commit", "-m", "x")
	require.NoError(t, err, output)
	assert.FileExists(t, out)
}
