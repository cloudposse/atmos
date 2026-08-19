//go:build mage

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeBinEnv is the env var that, when set to "1" on this compiled test
// binary, makes it behave as a stand-in for an external binary (golangci-lint
// / custom-gcl) instead of running the Go test suite: it records the args it
// was invoked with to fakeBinOutEnv and exits, without ever shelling out to a
// real tool or hardcoding a platform-specific binary like `true`/`sh`.
const (
	fakeBinEnv       = "ATMOS_MAGEFILES_FAKE_BIN"
	fakeBinOutEnv    = "ATMOS_MAGEFILES_FAKE_BIN_OUT"
	fakeBinExitEnv   = "ATMOS_MAGEFILES_FAKE_BIN_EXIT"
	fakeBinStdoutEnv = "ATMOS_MAGEFILES_FAKE_BIN_STDOUT"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeBinEnv) == "1" {
		runFakeBinAndExit()
	}
	os.Exit(m.Run())
}

// runFakeBinAndExit writes the process's own args (excluding argv[0]) to the
// file named by fakeBinOutEnv, one per line, and its working directory to
// that same path with a ".cwd" suffix, then exits with the code from
// fakeBinExitEnv (default 0). It never reaches the normal testing.M.Run path.
func runFakeBinAndExit() {
	if out := os.Getenv(fakeBinOutEnv); out != "" {
		_ = os.WriteFile(out, []byte(strings.Join(os.Args[1:], "\n")), 0o644)
		if cwd, err := os.Getwd(); err == nil {
			_ = os.WriteFile(out+".cwd", []byte(cwd), 0o644)
		}
	}
	if stdout := os.Getenv(fakeBinStdoutEnv); stdout != "" {
		_, _ = os.Stdout.WriteString(stdout)
	}
	code := 0
	if raw := os.Getenv(fakeBinExitEnv); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			code = parsed
		}
	}
	os.Exit(code)
}

// setUpFakeBin points binPath (typically customGCLBinaryPath(root)) at the
// currently-running test binary and arranges for it to run in fake-bin mode:
// invocations record their args to a file under t.TempDir() instead of
// shelling out to a real tool. Returns the path invocations will be recorded
// to; callers read it back with readFakeBinArgs.
func setUpFakeBin(t *testing.T, binPath string) (argsFile string) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)

	data, err := os.ReadFile(self)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(binPath), 0o755))
	require.NoError(t, os.WriteFile(binPath, data, 0o755))

	argsFile = filepath.Join(t.TempDir(), "fake-bin-args")
	t.Setenv(fakeBinEnv, "1")
	t.Setenv(fakeBinOutEnv, argsFile)
	t.Setenv(fakeBinExitEnv, "")
	return argsFile
}

// setUpFakePathBinary puts a fake binary named name (e.g. "golangci-lint")
// ahead of the real PATH so that production code shelling out to name by
// bare command (via sh.RunWithV et al., which resolve through PATH) invokes
// the fake instead. Unlike setUpFakeBin, this doesn't require a seam in
// production code for the binary's path — it's for commands invoked by bare
// name. Returns the path invocations will be recorded to.
func setUpFakePathBinary(t *testing.T, name string) (argsFile string) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	data, err := os.ReadFile(self)
	require.NoError(t, err)

	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, binName), data, 0o755))

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	argsFile = filepath.Join(t.TempDir(), "fake-bin-args")
	t.Setenv(fakeBinEnv, "1")
	t.Setenv(fakeBinOutEnv, argsFile)
	t.Setenv(fakeBinExitEnv, "")
	return argsFile
}

// readFakeBinArgs reads back the args recorded by a fake-bin invocation. It
// fails the test if the fake binary was never invoked.
func readFakeBinArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	require.NoError(t, err, "fake binary was not invoked")
	if len(data) == 0 {
		return nil
	}
	return strings.Split(string(data), "\n")
}

// readFakeBinCwd reads back the working directory the fake binary observed
// (via os.Getwd()) when it was invoked. It fails the test if the fake binary
// was never invoked.
func readFakeBinCwd(t *testing.T, argsFile string) string {
	t.Helper()
	data, err := os.ReadFile(argsFile + ".cwd")
	require.NoError(t, err, "fake binary was not invoked")
	return string(data)
}

// initGitRepoFixture creates a fresh git repo under a t.TempDir(), with a
// committed go.mod (so mageRepoRoot resolves) and identity configured for
// commits. Git is this package's own runtime dependency (it orchestrates
// `git diff`/`git rev-parse`), so exercising it against a real temp repo is
// not the "hardcoded platform binary" pattern CLAUDE.md warns against.
func initGitRepoFixture(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	runGit(t, root, "add", "go.mod")
	runGit(t, root, "commit", "-q", "-m", "init")
	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(gitCmd, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// execGitOutput runs a git command in dir and returns its trimmed stdout.
func execGitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command(gitCmd, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
