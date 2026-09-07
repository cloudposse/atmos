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
	fakeBinEnv          = "ATMOS_MAGEFILES_FAKE_BIN"
	fakeBinOutEnv       = "ATMOS_MAGEFILES_FAKE_BIN_OUT"
	fakeBinExitEnv      = "ATMOS_MAGEFILES_FAKE_BIN_EXIT"
	fakeBinStdoutEnv    = "ATMOS_MAGEFILES_FAKE_BIN_STDOUT"
	fakeBinFailUntilEnv = "ATMOS_MAGEFILES_FAKE_BIN_FAIL_UNTIL"
)

// fakeBinEnvAllowlist is the exhaustive list of environment variable names
// runFakeBinAndExit will ever record to the ".env" file, and the only names
// readFakeBinEnv's callers assert on. The full process environment (as
// returned by os.Environ()) can carry CI tokens, cloud credentials, and
// other user-provided secrets that these tests have no business persisting
// to disk — even under t.TempDir() — so it must never be written out;
// only vars a test actually asserts on belong here. Extend this list
// (never fall back to the full environment) when a new test needs to
// assert on another explicitly-set build variable.
var fakeBinEnvAllowlist = []string{
	"CGO_ENABLED",
	"GOFIPS140",
	"GOOS",
	"GOARCH",
	"GOFLAGS",
	"FOO", // TestRunIn's explicit extraEnv input.
}

func TestMain(m *testing.M) {
	if os.Getenv(fakeBinEnv) == "1" {
		runFakeBinAndExit()
	}
	os.Exit(m.Run())
}

// runFakeBinAndExit writes the process's own args (excluding argv[0]) to the
// file named by fakeBinOutEnv, one per line, its working directory to that
// same path with a ".cwd" suffix, and the subset of its environment named in
// fakeBinEnvAllowlist (one "K=V" pair per line, present vars only) with a
// ".env" suffix. If fakeBinFailUntilEnv is set, it then tracks an
// invocation count in a ".invocations" sibling file and exits 1 for every
// call up to and including that count, exiting 0 from the next call onward —
// letting tests exercise retry-then-succeed and retry-exhausted paths
// deterministically. Otherwise it exits with the code from fakeBinExitEnv
// (default 0). It never reaches the normal testing.M.Run path. It
// deliberately never dumps the full process environment (os.Environ()) —
// see fakeBinEnvAllowlist's doc comment.
func runFakeBinAndExit() {
	if out := os.Getenv(fakeBinOutEnv); out != "" {
		_ = os.WriteFile(out, []byte(strings.Join(os.Args[1:], "\n")), 0o644)
		if cwd, err := os.Getwd(); err == nil {
			_ = os.WriteFile(out+".cwd", []byte(cwd), 0o644)
		}
		var allowedEnv []string
		for _, name := range fakeBinEnvAllowlist {
			if value, ok := os.LookupEnv(name); ok {
				allowedEnv = append(allowedEnv, name+"="+value)
			}
		}
		_ = os.WriteFile(out+".env", []byte(strings.Join(allowedEnv, "\n")), 0o644)
	}
	if stdout := os.Getenv(fakeBinStdoutEnv); stdout != "" {
		_, _ = os.Stdout.WriteString(stdout)
	}
	if failUntilRaw := os.Getenv(fakeBinFailUntilEnv); failUntilRaw != "" {
		failUntil, _ := strconv.Atoi(failUntilRaw)
		countFile := os.Getenv(fakeBinOutEnv) + ".invocations"
		count := 0
		if data, err := os.ReadFile(countFile); err == nil {
			count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		count++
		_ = os.WriteFile(countFile, []byte(strconv.Itoa(count)), 0o644)
		if count <= failUntil {
			os.Exit(1)
		}
		os.Exit(0)
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

// setUpFakePathBinaryFailingNTimes is like setUpFakePathBinary(t, "go"), but
// the fake binary exits 1 for its first failUntil invocations and exits 0
// from the (failUntil+1)th invocation onward — used to exercise
// retry-then-succeed and retry-exhausted code paths deterministically for
// runGoModDownload, the only current caller of this retry behavior.
func setUpFakePathBinaryFailingNTimes(t *testing.T, failUntil int) (argsFile string) {
	t.Helper()
	argsFile = setUpFakePathBinary(t, "go")
	t.Setenv(fakeBinFailUntilEnv, strconv.Itoa(failUntil))
	return argsFile
}

// readFakeBinInvocationCount reads back how many times the fake binary was
// invoked in fail-until-N mode (see setUpFakePathBinaryFailingNTimes).
// Returns 0 if it was never invoked in that mode.
func readFakeBinInvocationCount(t *testing.T, argsFile string) int {
	t.Helper()
	data, err := os.ReadFile(argsFile + ".invocations")
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return count
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

// readFakeBinEnv reads back the fakeBinEnvAllowlist-filtered subset of the
// environment the fake binary observed when it was invoked, as a
// name->value map. Only names in fakeBinEnvAllowlist can ever appear here —
// add a name there (and nowhere else) before asserting on it. Fails the test
// if the fake binary was never invoked.
func readFakeBinEnv(t *testing.T, argsFile string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(argsFile + ".env")
	require.NoError(t, err, "fake binary was not invoked")
	env := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok {
			env[name] = value
		}
	}
	return env
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
