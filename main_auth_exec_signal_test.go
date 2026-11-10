package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	authExecSignalHelperEnv = "_ATMOS_AUTH_EXEC_SIGNAL_HELPER"
	authExecSignalDirEnv    = "_ATMOS_AUTH_EXEC_SIGNAL_DIR"
	authExecSignalExitEnv   = "_ATMOS_AUTH_EXEC_SIGNAL_EXIT"
	authExecSignalIgnoreEnv = "_ATMOS_AUTH_EXEC_SIGNAL_IGNORE"
	authExecSignalScopeEnv  = "_ATMOS_AUTH_EXEC_AWS_FILES_BASE_PATH"
	authExecSignalWait      = 20 * time.Second
)

type authExecSignalReady struct {
	PID             int    `json:"pid"`
	PrivateDir      string `json:"private_dir"`
	CredentialsFile string `json:"credentials_file"`
	ConfigFile      string `json:"config_file"`
}

type authExecSignalProcess struct {
	command *exec.Cmd
	done    chan struct{}
	err     error
}

// These subprocesses exercise main's actual global signal handler, which skips
// command defers when it exits directly instead of waiting for the child.
func TestMainAuthExecSignalWaitsForChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("parent-only Unix signals cannot be delivered with os.Process.Signal on Windows")
	}
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		for _, exitCode := range []int{0, 7} {
			t.Run(fmt.Sprintf("signal-%d/exit-%d", sig, exitCode), func(t *testing.T) {
				dir, process, requests := startAuthExecSignalProcess(t, exitCode, false)
				ready := readAuthExecSignalReady(t, dir, process)
				require.NoError(t, process.command.Process.Signal(sig))
				awaitAuthExecSignalFile(t, filepath.Join(dir, "received"), process)
				received, err := os.ReadFile(filepath.Join(dir, "received"))
				require.NoError(t, err)
				assert.Equal(t, strconv.Itoa(int(sig)), string(received), "the original signal must reach the child")
				assertAuthExecSignalStillRunning(t, process)
				assertAuthExecSignalFiles(t, ready)
				require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0o600))
				waitAuthExecSignalProcess(t, process, exitCode)
				assert.FileExists(t, filepath.Join(dir, "child-exited"), "the child must finish its signal handler")
				assertAuthExecSignalRemoved(t, ready)
				assert.EqualValues(t, 1, requests.Load(), "only the synthetic STS exchange is needed")
			})
		}
	}
}

// A child that ignores graceful termination must be killed before credentials
// are removed, and the actual child status must survive the main signal path.
func TestMainAuthExecSignalKillsIgnoringChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("parent-only Unix signals cannot be delivered with os.Process.Signal on Windows")
	}
	dir, process, _ := startAuthExecSignalProcess(t, 0, true)
	ready := readAuthExecSignalReady(t, dir, process)
	require.NoError(t, process.command.Process.Signal(syscall.SIGTERM))
	waitAuthExecSignalProcess(t, process, 128+int(syscall.SIGKILL))
	assert.NoFileExists(t, filepath.Join(dir, "child-exited"), "the ignoring child must require escalation")
	assertAuthExecSignalRemoved(t, ready)
}

// TestAuthExecSignalHelper runs only in fresh subprocesses. The parent enters
// actual main(); its child supplies a portable program with a gated signal handler.
func TestAuthExecSignalHelper(t *testing.T) {
	switch os.Getenv(authExecSignalHelperEnv) {
	case "parent":
		binary, err := os.Executable()
		require.NoError(t, err)
		t.Setenv(authExecSignalHelperEnv, "child")
		os.Args = []string{"atmos", "auth", "exec", "--identity=runner", "--", binary, "-test.run=^TestAuthExecSignalHelper$"}
		main()
		t.Fatal("main unexpectedly returned without exiting")
	case "child":
		runAuthExecSignalChild(t)
	}
}

// runAuthExecSignalChild publishes its credential paths, handles or ignores shutdown signals, and exits with
// the configured child status.
func runAuthExecSignalChild(t *testing.T) {
	t.Helper()
	dir := os.Getenv(authExecSignalDirEnv)
	incoming := make(chan os.Signal, 2)
	if os.Getenv(authExecSignalIgnoreEnv) == "true" {
		signal.Ignore(os.Interrupt, syscall.SIGTERM)
	} else {
		signal.Notify(incoming, os.Interrupt, syscall.SIGTERM)
		t.Cleanup(func() { signal.Stop(incoming) })
	}
	ready := authExecSignalReady{
		PID: os.Getpid(), PrivateDir: os.Getenv(authExecSignalScopeEnv),
		CredentialsFile: os.Getenv("AWS_SHARED_CREDENTIALS_FILE"),
		ConfigFile:      os.Getenv("AWS_CONFIG_FILE"),
	}
	require.NotEmpty(t, ready.PrivateDir, "the child must inherit invocation-owned storage")
	assertAuthExecSignalFiles(t, ready)
	payload, err := json.Marshal(ready)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ready.tmp"), payload, 0o600))
	require.NoError(t, os.Rename(filepath.Join(dir, "ready.tmp"), filepath.Join(dir, "ready")))
	select {
	case sig := <-incoming:
		number, ok := sig.(syscall.Signal)
		require.True(t, ok)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "received"), []byte(strconv.Itoa(int(number))), 0o600))
	case <-time.After(authExecSignalWait):
		t.Fatal("the child did not receive a signal or termination")
	}
	waitAuthExecSignalRelease(t, dir, ready)
	exitCode, err := strconv.Atoi(os.Getenv(authExecSignalExitEnv))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "child-exited"), nil, 0o600))
	os.Exit(exitCode)
}

// waitAuthExecSignalRelease keeps checking credential availability while the child waits for permission to
// finish its signal handler.
func waitAuthExecSignalRelease(t *testing.T, dir string, ready authExecSignalReady) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(authExecSignalWait)
	defer deadline.Stop()
	for {
		assertAuthExecSignalFiles(t, ready)
		if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("the signal handler was not released")
		}
	}
}

// startAuthExecSignalProcess starts an isolated Atmos subprocess with mock STS and arranges bounded cleanup
// of the test processes.
func startAuthExecSignalProcess(t *testing.T, exitCode int, ignore bool) (string, *authExecSignalProcess, *atomic.Int32) {
	t.Helper()
	dir := t.TempDir()
	requests := serveAuthExecSignalSTS(t, dir)
	binary, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(binary, "-test.run=^TestAuthExecSignalHelper$")
	command.Dir = dir
	command.Env = authExecSignalEnvironment(t, dir, exitCode, ignore)
	// Keep subprocess output private even if a future regression logs its synthetic credentials.
	command.Stdout, command.Stderr = io.Discard, io.Discard
	process := &authExecSignalProcess{command: command, done: make(chan struct{})}
	require.NoError(t, command.Start())
	go func() {
		process.err = command.Wait()
		close(process.done)
	}()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		if t.Failed() {
			killAuthExecSignalChild(dir)
		}
		select {
		case <-process.done:
		case <-time.After(5 * time.Second):
			t.Error("the test parent could not be reaped")
		}
	})
	return dir, process, requests
}

// serveAuthExecSignalSTS serves synthetic role credentials and writes an Atmos configuration pointing
// exclusively to the local server.
func serveAuthExecSignalSTS(t *testing.T, dir string) *atomic.Int32 {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ParseForm() != nil || r.Form.Get("Action") != "AssumeRoleWithWebIdentity" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprintf(w, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleWithWebIdentityResult><Credentials><AccessKeyId>synthetic-signal-key</AccessKeyId><SecretAccessKey>synthetic-signal-secret</SecretAccessKey><SessionToken>synthetic-signal-session</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleWithWebIdentityResult></AssumeRoleWithWebIdentityResponse>`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	t.Cleanup(server.Close)
	config := fmt.Sprintf(`base_path: .
errors:
  sentry:
    enabled: false
settings:
  telemetry:
    enabled: false
auth:
  realm: gitlab-signal-test
  keyring:
    type: memory
  providers:
    gitlab:
      kind: gitlab/oidc
  identities:
    runner:
      kind: aws/assume-role
      via:
        provider: gitlab
      principal:
        assume_role: arn:aws:iam::123456789012:role/GitLabSignalTest
        region: eu-west-1
      spec:
        endpoint_url: %q
`, server.URL)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(config), 0o600))
	return requests
}

// authExecSignalEnvironment builds a subprocess environment with isolated configuration, credential storage,
// and temporary directories.
func authExecSignalEnvironment(t *testing.T, dir string, exitCode int, ignore bool) []string {
	t.Helper()
	environ := make([]string, 0)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "ATMOS_") || strings.HasPrefix(key, "_ATMOS_") || strings.HasPrefix(key, "AWS_") || strings.HasPrefix(key, "XDG_") {
			continue
		}
		if key != "HOME" && key != "USERPROFILE" && key != "TMPDIR" && key != "TMP" && key != "TEMP" {
			environ = append(environ, entry)
		}
	}
	for _, name := range []string{"home", "config", "cache", "data", "tmp", "sdk"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o700))
	}
	return append(environ,
		authExecSignalHelperEnv+"=parent", authExecSignalDirEnv+"="+dir,
		authExecSignalExitEnv+"="+strconv.Itoa(exitCode), authExecSignalIgnoreEnv+"="+strconv.FormatBool(ignore),
		"HOME="+filepath.Join(dir, "home"), "USERPROFILE="+filepath.Join(dir, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(dir, "config"), "ATMOS_XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
		"XDG_CACHE_HOME="+filepath.Join(dir, "cache"), "ATMOS_XDG_CACHE_HOME="+filepath.Join(dir, "cache"),
		"XDG_DATA_HOME="+filepath.Join(dir, "data"), "ATMOS_XDG_DATA_HOME="+filepath.Join(dir, "data"),
		"TMPDIR="+filepath.Join(dir, "tmp"), "TMP="+filepath.Join(dir, "tmp"), "TEMP="+filepath.Join(dir, "tmp"),
		"ATMOS_CLI_CONFIG_PATH="+dir, "ATMOS_KEYRING_TYPE=memory", "ATMOS_OIDC_TOKEN=synthetic-signal-token",
		"ATMOS_TELEMETRY_ENABLED=false", "ATMOS_VERSION_CHECK_ENABLED=false", "ATMOS_FORCE_COLOR=false", "NO_COLOR=1", "CI=true",
		"AWS_CONFIG_FILE="+filepath.Join(dir, "sdk", "config"), "AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(dir, "sdk", "credentials"),
		"AWS_EC2_METADATA_DISABLED=true")
}

func readAuthExecSignalReady(t *testing.T, dir string, process *authExecSignalProcess) authExecSignalReady {
	t.Helper()
	path := filepath.Join(dir, "ready")
	awaitAuthExecSignalFile(t, path, process)
	payload, err := os.ReadFile(path)
	require.NoError(t, err)
	var ready authExecSignalReady
	require.NoError(t, json.Unmarshal(payload, &ready))
	require.Positive(t, ready.PID, "the helper must record its own process")
	privatePath, err := filepath.Rel(filepath.Join(dir, "tmp"), ready.PrivateDir)
	require.NoError(t, err)
	require.True(t, filepath.IsLocal(privatePath), "the invocation must use the isolated temporary root")
	assertAuthExecSignalFiles(t, ready)
	return ready
}

// awaitAuthExecSignalFile waits for a helper marker and fails promptly if Atmos exits before creating it.
func awaitAuthExecSignalFile(t *testing.T, path string, process *authExecSignalProcess) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(authExecSignalWait)
	defer deadline.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-process.done:
			t.Fatalf("Atmos exited before the helper reached %s: %v", filepath.Base(path), process.err)
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for the helper's %s marker", filepath.Base(path))
		}
	}
}

// assertAuthExecSignalStillRunning checks that Atmos has not exited while its child is still handling
// shutdown.
func assertAuthExecSignalStillRunning(t *testing.T, process *authExecSignalProcess) {
	t.Helper()
	select {
	case <-process.done:
		t.Fatalf("Atmos exited before the child was released: %v", process.err)
	case <-time.After(100 * time.Millisecond):
	}
}

// waitAuthExecSignalProcess bounds shutdown waiting and verifies that Atmos preserves the child exit status.
func waitAuthExecSignalProcess(t *testing.T, process *authExecSignalProcess, wantExitCode int) {
	t.Helper()
	select {
	case <-process.done:
	case <-time.After(10 * time.Second):
		t.Fatal("Atmos did not finish bounded child shutdown")
	}
	require.NotNil(t, process.command.ProcessState)
	assert.Equal(t, wantExitCode, process.command.ProcessState.ExitCode(), "Atmos must preserve the waited child status")
}

// assertAuthExecSignalFiles verifies that both AWS files exist inside the invocation-owned directory during
// child shutdown.
func assertAuthExecSignalFiles(t *testing.T, ready authExecSignalReady) {
	t.Helper()
	for _, path := range []string{ready.CredentialsFile, ready.ConfigFile} {
		require.NotEmpty(t, path)
		relative, err := filepath.Rel(ready.PrivateDir, path)
		require.NoError(t, err)
		require.True(t, filepath.IsLocal(relative), "AWS files must remain in invocation-owned storage")
		info, err := os.Stat(path)
		require.NoError(t, err, "AWS files must remain available while the child handles the signal")
		require.True(t, info.Mode().IsRegular())
	}
}

// assertAuthExecSignalRemoved verifies credential cleanup and waits for the child process to be reaped.
func assertAuthExecSignalRemoved(t *testing.T, ready authExecSignalReady) {
	t.Helper()
	assert.NoDirExists(t, ready.PrivateDir)
	assert.NoFileExists(t, ready.CredentialsFile)
	assert.NoFileExists(t, ready.ConfigFile)
	require.Eventually(t, func() bool {
		child, err := os.FindProcess(ready.PID)
		if err != nil {
			return true
		}
		defer child.Release()
		return child.Signal(syscall.Signal(0)) != nil
	}, 5*time.Second, 10*time.Millisecond, "the child must be reaped before Atmos exits")
}

// killAuthExecSignalChild best-effort terminates a helper identified by its ready marker after a test
// failure.
func killAuthExecSignalChild(dir string) {
	payload, err := os.ReadFile(filepath.Join(dir, "ready"))
	if err != nil {
		return
	}
	var ready authExecSignalReady
	if json.Unmarshal(payload, &ready) != nil || ready.PID <= 0 {
		return
	}
	if child, err := os.FindProcess(ready.PID); err == nil {
		_ = child.Kill()
		_ = child.Release()
	}
}
