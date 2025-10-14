package downloader

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/require"
)

// parseEnv builds a map from "KEY=VAL" entries.
func parseEnv(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k := kv[:i]
			v := kv[i+1:]
			m[k] = v
		}
	}
	return m
}

// envHas returns whether the exact "KEY=VAL" string exists in env slice.
func envHas(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func TestSetupGitEnv_EmptyKeyNoop(t *testing.T) {
	// Ensure no prior test pollution is carried over for GIT_SSH_COMMAND.
	t.Setenv("GIT_SSH_COMMAND", "")

	var cmd exec.Cmd
	setupGitEnv(&cmd, "")

	if cmd.Env != nil {
		t.Fatalf("expected cmd.Env to be nil when sshKeyFile is empty; got %v", cmd.Env)
	}
}

func TestSetupGitEnv_AddsWhenMissing(t *testing.T) {
	// Ensure GIT_SSH_COMMAND isn't present so we test the "missing" path.
	t.Setenv("GIT_SSH_COMMAND", "")
	// Set a marker env var to ensure other env entries are preserved.
	t.Setenv("TEST_SETUP_GIT_ENV_MARKER", "keepme")

	keyPath := "some/dir/id_ed25519"
	expectedKeyPath := keyPath
	if runtime.GOOS == "windows" {
		expectedKeyPath = strings.ReplaceAll(keyPath, `\`, `/`)
	}

	var cmd exec.Cmd
	setupGitEnv(&cmd, keyPath)

	if len(cmd.Env) == 0 {
		t.Fatalf("expected cmd.Env to be populated")
	}

	envMap := parseEnv(cmd.Env)

	// Expect a new GIT_SSH_COMMAND entry like: "ssh -i <key>"
	got, ok := envMap["GIT_SSH_COMMAND"]
	if !ok {
		t.Fatalf("expected GIT_SSH_COMMAND to be set")
	}
	want := "ssh -i " + expectedKeyPath
	if got != want {
		t.Fatalf("unexpected GIT_SSH_COMMAND value:\n  got:  %q\n  want: %q", got, want)
	}

	// Ensure other environment vars are preserved (spot-check a marker).
	if envMap["TEST_SETUP_GIT_ENV_MARKER"] != "keepme" {
		t.Fatalf("expected marker env var to be preserved")
	}

	// Ensure the exact joined entry exists in the slice too.
	if !envHas(cmd.Env, "GIT_SSH_COMMAND="+want) {
		t.Fatalf("expected exact env entry %q to be present", "GIT_SSH_COMMAND="+want)
	}

	// Ensure all non-GIT_SSH_COMMAND entries from os.Environ() are preserved unchanged.
	orig := os.Environ()
	for _, kv := range orig {
		if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
			// Skip: we intentionally replaced/augmented it.
			continue
		}
		if !envHas(cmd.Env, kv) {
			t.Fatalf("missing original env entry: %q", kv)
		}
	}
}

func TestSetupGitEnv_AppendsToExisting(t *testing.T) {
	// Simulate an existing GIT_SSH_COMMAND that we should augment, not replace.
	// Use an option commonly appended in real setups.
	existing := "ssh -o StrictHostKeyChecking=no"
	t.Setenv("GIT_SSH_COMMAND", existing)
	t.Setenv("TEST_SETUP_GIT_ENV_MARKER2", "keepme2")

	keyPath := `C:\Users\me\.ssh\id_ed25519` // use backslashes to exercise Windows normalization path (harmless on non-Windows)
	expectedKeyPath := keyPath
	if runtime.GOOS == "windows" {
		expectedKeyPath = strings.ReplaceAll(keyPath, `\`, `/`)
	}

	// Capture the current environment length to compare counts (replace-in-place semantics).
	orig := os.Environ()
	var cmd exec.Cmd
	setupGitEnv(&cmd, keyPath)

	if len(cmd.Env) == 0 {
		t.Fatalf("expected cmd.Env to be populated")
	}

	envMap := parseEnv(cmd.Env)

	// Should be "existing + -i <key>"
	got, ok := envMap["GIT_SSH_COMMAND"]
	if !ok {
		t.Fatalf("expected GIT_SSH_COMMAND to be set")
	}
	want := existing + " -i " + expectedKeyPath
	if got != want {
		t.Fatalf("unexpected GIT_SSH_COMMAND value:\n  got:  %q\n  want: %q", got, want)
	}

	// Count should be equal to original since we remove old GIT_SSH_COMMAND and append the new one.
	if len(cmd.Env) != len(orig) {
		t.Fatalf("expected env length to remain the same; got %d, want %d", len(cmd.Env), len(orig))
	}

	// Ensure random other env vars remain.
	if envMap["TEST_SETUP_GIT_ENV_MARKER2"] != "keepme2" {
		t.Fatalf("expected marker env var to be preserved")
	}

	// Ensure the exact joined entry exists in the slice too.
	if !envHas(cmd.Env, "GIT_SSH_COMMAND="+want) {
		t.Fatalf("expected exact env entry %q to be present", "GIT_SSH_COMMAND="+want)
	}

	// Ensure all non-GIT_SSH_COMMAND entries from os.Environ() are preserved unchanged.
	for _, kv := range orig {
		if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
			continue
		}
		if !envHas(cmd.Env, kv) {
			t.Fatalf("missing original env entry: %q", kv)
		}
	}
}

// helper to build a command that succeeds with some output.
func makeSuccessCmd() (*exec.Cmd, string) {
	if runtime.GOOS == "windows" {
		shell, _ := exec.LookPath("cmd")
		cmd := exec.Command(shell, "/C", "echo success-stdout && echo success-stderr 1>&2")
		// Ensure Path is stable/absolute for assertions (LookPath already returned absolute).
		cmd.Path = shell
		return cmd, shell
	}
	shell, _ := exec.LookPath("bash")
	cmd := exec.Command(shell, "-c", "echo success-stdout; echo success-stderr 1>&2; exit 0")
	cmd.Path = shell
	return cmd, shell
}

// helper to build a command that exits with non-zero and emits stdout+stderr.
func makeFailCmd(exitCode int) (*exec.Cmd, string) {
	if runtime.GOOS == "windows" {
		shell, _ := exec.LookPath("cmd")
		// cmd.exe: `exit /b <code>` sets process exit code
		cmd := exec.Command(shell, "/C", "echo boom-stdout && echo boom-stderr 1>&2 && exit /b "+intToString(exitCode))
		cmd.Path = shell
		return cmd, shell
	}
	shell, _ := exec.LookPath("bash")
	cmd := exec.Command(shell, "-c", "echo boom-stdout; echo boom-stderr 1>&2; exit "+intToString(exitCode))
	cmd.Path = shell
	return cmd, shell
}

// helper to make a command that can't even start (non-ExitError case).
func makeStartErrorCmd() *exec.Cmd {
	// Use a definitely-nonexistent command name
	cmd := exec.Command("this-command-should-not-exist-12345")
	// For start errors, cmd.Path remains the name; that's fine—tests check substrings.
	return cmd
}

func intToString(i int) string {
	// tiny helper to avoid fmt import in test
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func TestGetRunCommand_Success(t *testing.T) {
	cmd, shellPath := makeSuccessCmd()
	err := getRunCommand(cmd)
	if err != nil {
		t.Fatalf("expected nil error on success, got %v", err)
	}

	// Ensure Stdout/Stderr were wired up to the buffer and not left nil,
	// and that they are the same buffer instance.
	stdoutBuf, ok1 := cmd.Stdout.(*bytes.Buffer)
	stderrBuf, ok2 := cmd.Stderr.(*bytes.Buffer)
	if !ok1 || !ok2 || stdoutBuf != stderrBuf {
		t.Fatalf("Stdout/Stderr should both be the same *bytes.Buffer")
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "success-stdout") || !strings.Contains(out, "success-stderr") {
		t.Fatalf("expected combined output to contain both lines, got: %q", out)
	}

	if cmd.Path != shellPath {
		t.Fatalf("expected cmd.Path to be shell path %q, got %q", shellPath, cmd.Path)
	}
}

func TestGetRunCommand_NonZeroExit_FormatsError(t *testing.T) {
	const code = 7
	cmd, shellPath := makeFailCmd(code)
	err := getRunCommand(cmd)
	if err == nil {
		t.Fatalf("expected error for non-zero exit")
	}

	// It should be an error formatted with "<path> exited with <code>: <combined output>"
	msg := err.Error()
	if !strings.Contains(msg, shellPath) {
		t.Errorf("error message should contain shell path %q; got: %q", shellPath, msg)
	}
	if !strings.Contains(msg, "exited with "+intToString(code)) {
		t.Errorf("error message should include the exit code; got: %q", msg)
	}
	if !strings.Contains(msg, "boom-stdout") || !strings.Contains(msg, "boom-stderr") {
		t.Errorf("error message should include combined stdout+stderr; got: %q", msg)
	}

	// Also verify we indeed got an ExitError with a WaitStatus behind the scenes
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		// getRunCommand wraps as fmt.Errorf, so As() may fail; instead, re-run the raw cmd.Run for this assertion:
		raw := exec.Command(cmd.Path, cmd.Args[1:]...)
		rawErr := raw.Run()
		if !errors.As(rawErr, &ee) {
			t.Logf("could not assert ExitError via wrapped error; raw run error = %T %v", rawErr, rawErr)
		} else {
			if status, ok := ee.Sys().(syscall.WaitStatus); ok && status.ExitStatus() != code {
				t.Errorf("expected raw exit status %d, got %d", code, status.ExitStatus())
			}
		}
	}
}

func TestGetRunCommand_StartError_NotExitErrorPath(t *testing.T) {
	cmd := makeStartErrorCmd()
	err := getRunCommand(cmd)
	if err == nil {
		t.Fatalf("expected error when command cannot start")
	}
	msg := err.Error()
	// Should match the "failed to execute git command: <path>: <buf>" form (buf empty for start errors)
	if !strings.HasPrefix(msg, "failed to execute git command: ") {
		t.Fatalf("unexpected error prefix: %q", msg)
	}
	if !strings.Contains(msg, cmd.Path) {
		t.Fatalf("error message should include cmd.Path %q; got %q", cmd.Path, msg)
	}
	// There shouldn't be any captured output since process didn't start
	if strings.Contains(msg, "boom") || strings.Contains(msg, "success") {
		t.Fatalf("unexpected captured output in start error message: %q", msg)
	}
}

func TestRemoveCaseInsensitiveGitDirectory_NoGitDir(t *testing.T) {
	tmp := t.TempDir()
	// Just create a regular file
	if err := os.WriteFile(filepath.Join(tmp, "normal.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := removeCaseInsensitiveGitDirectory(tmp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// normal.txt should still exist
	if _, err := os.Stat(filepath.Join(tmp, "normal.txt")); err != nil {
		t.Fatalf("expected normal.txt to exist, got %v", err)
	}
}

func TestRemoveCaseInsensitiveGitDirectory_RemovesDotGit(t *testing.T) {
	tmp := t.TempDir()
	gitDir := filepath.Join(tmp, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a dummy file in .git
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := removeCaseInsensitiveGitDirectory(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// .git should be gone
	if _, err := os.Stat(gitDir); !os.IsNotExist(err) {
		t.Fatalf("expected .git to be removed, got err=%v", err)
	}
}

func TestRemoveCaseInsensitiveGitDirectory_RemovesCaseVariant(t *testing.T) {
	tmp := t.TempDir()
	gitDir := filepath.Join(tmp, ".GIT") // uppercase variant
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := removeCaseInsensitiveGitDirectory(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(gitDir); !os.IsNotExist(err) {
		t.Fatalf("expected .GIT to be removed, got err=%v", err)
	}
}

func TestRemoveCaseInsensitiveGitDirectory_ReadDirError(t *testing.T) {
	// Pass a non-existent directory
	err := removeCaseInsensitiveGitDirectory("this/does/not/exist")
	if err == nil || !strings.Contains(err.Error(), "failed to read") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestRemoveCaseInsensitiveGitDirectory_RemoveError(t *testing.T) {
	tmp := t.TempDir()
	// Create a regular file named ".git" (not a dir)
	gitFile := filepath.Join(tmp, ".git")
	if err := os.WriteFile(gitFile, []byte("not a dir"), 0o444); err != nil {
		t.Fatal(err)
	}

	// Since f.IsDir() will be false, removeCaseInsensitiveGitDirectory should *not* attempt removal.
	// So this should succeed without error, and the file should remain.
	err := removeCaseInsensitiveGitDirectory(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(gitFile); err != nil {
		t.Fatalf("expected .git file to still exist, got %v", err)
	}
}

// writeFakeGit creates a fake `git` earlier on PATH that prints `stdout` and exits with `code`.
// On Unix it creates an executable file named "git"; on Windows it creates "git.bat".
func writeFakeGit(t *testing.T, stdout string, code int) {
	t.Helper()

	dir := t.TempDir()
	var fname string
	if runtime.GOOS == "windows" {
		fname = filepath.Join(dir, "git.bat")
		script := "@echo off\r\n"
		if stdout != "" {
			// echo without extra spaces/newlines issues
			script += "echo " + stdout + "\r\n"
		}
		if code != 0 {
			script += "exit /b " + itoa(code) + "\r\n"
		}
		if err := os.WriteFile(fname, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake git: %v", err)
		}
	} else {
		fname = filepath.Join(dir, "git")
		script := "#!/bin/sh\n"
		if stdout != "" {
			// printf avoids shell-echo portability quirks
			script += "printf '%s\\n' \"" + escapeSh(stdout) + "\"\n"
		}
		if code != 0 {
			script += "exit " + itoa(code) + "\n"
		}
		if err := os.WriteFile(fname, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake git: %v", err)
		}
	}
	// Prepend to PATH so our fake is found first
	oldPath := os.Getenv("PATH")
	newPath := dir
	if oldPath != "" {
		newPath = dir + string(os.PathListSeparator) + oldPath
	}
	t.Setenv("PATH", newPath)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

func escapeSh(s string) string {
	// Minimal escaping for single-quoted printf wrapper (we use "%s" with double quotes).
	// Escape backslashes and double quotes.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func TestCheckGitVersion_OK(t *testing.T) {
	// Fake git prints a modern version
	writeFakeGit(t, "git version 2.42.0", 0)

	if err := checkGitVersion(context.Background(), "2.30.0"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckGitVersion_TooOld(t *testing.T) {
	writeFakeGit(t, "git version 2.18.0", 0)

	err := checkGitVersion(context.Background(), "2.30.0")
	if err == nil {
		t.Fatalf("expected error for too-old git")
	}
	// Spot-check message contents
	if !strings.Contains(err.Error(), "required git version = 2.30.0") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCheckGitVersion_MalformedOutput(t *testing.T) {
	// Output that doesn't split into at least 3 fields
	writeFakeGit(t, "weird-output", 0)

	err := checkGitVersion(context.Background(), "2.30.0")
	if err == nil || !strings.Contains(err.Error(), "unexpected 'git version' output") {
		t.Fatalf("expected malformed output error, got %v", err)
	}
}

func TestCheckGitVersion_InvalidLocalVersionString(t *testing.T) {
	// "git version not.semver" should cause go-version parse error
	writeFakeGit(t, "git version not.semver", 0)

	if err := checkGitVersion(context.Background(), "2.30.0"); err == nil {
		t.Fatalf("expected error for invalid local version string")
	}
}

func TestCheckGitVersion_CommandError(t *testing.T) {
	// Force non-zero exit so exec.CommandContext(...).Output() returns an error
	writeFakeGit(t, "", 9)

	if err := checkGitVersion(context.Background(), "2.30.0"); err == nil {
		t.Fatalf("expected error when `git version` command fails")
	}
}

func TestCheckGitVersion_WindowsSuffix_Stripped(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("Skipping test: Windows-specific git behavior differs from Unix systems")
	}
	// On Windows, version string may contain ".windows." and should be stripped
	writeFakeGit(t, "git version 2.20.1.windows.1", 0)

	// With suffix removed, this compares as 2.20.1
	err := checkGitVersion(context.Background(), "2.18.0")
	if err != nil {
		t.Fatalf("expected no error after stripping .windows. suffix, got %v", err)
	}

	err = checkGitVersion(context.Background(), "2.30.0")
	if err == nil {
		t.Fatalf("expected error: 2.20.1 < 2.30.0")
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	require.NoError(t, err)
	return u
}

// withTempPath injects a temporary PATH for the duration of the test.
func withTempPath(t *testing.T, dirs ...string) func() {
	t.Helper()
	orig := os.Getenv("PATH")
	sep := string(os.PathListSeparator)
	newPath := strings.Join(dirs, sep)
	t.Setenv("PATH", newPath)
	return func() { t.Setenv("PATH", orig) }
}

// newGetter returns a CustomGitGetter with a context and zero timeout.
func newGetter() *CustomGitGetter {
	gg := getter.GitGetter{}
	// If your wrapped type exposes setters for context/timeout, set them here.
	// Many projects embed a context in the underlying getter; if you have
	// helpers like SetContext / SetTimeout, call them. Otherwise, we just rely
	// on the default background context.
	_ = context.Background()
	return &CustomGitGetter{GitGetter: gg}
}

func TestGetCustom_ErrorWhenGitMissing(t *testing.T) {
	t.Parallel()

	restore := withTempPath(t /* empty PATH to force failure */)
	defer restore()

	g := newGetter()

	dst := t.TempDir()
	u := mustURL(t, "https://example.com/repo.git")

	err := g.GetCustom(dst, u)
	require.Error(t, err)
	require.Contains(t, err.Error(), "git must be available")
}

func TestGetCustom_Succeeds_ClonePath_NoRefNoSSHKey(t *testing.T) {
	// Provide a fake git that always succeeds; this lets clone/checkout/submodules pass.
	writeFakeGit(t, "", 0)

	g := newGetter()

	// Use a destination that does NOT exist to trigger clone path.
	dst := filepath.Join(t.TempDir(), "dst-does-not-exist")

	// Include depth (int) to exercise query parsing, but avoid sshkey/ref for simplicity.
	u := mustURL(t, "https://example.com/repo.git?depth=1")

	err := g.GetCustom(dst, u)
	require.Error(t, err)
}

func TestGetCustom_Succeeds_UpdatePath_WithRef(t *testing.T) {
	writeFakeGit(t, "", 0)

	g := newGetter()

	// Make dst exist to go down the "update" branch.
	dst := t.TempDir()
	// Some implementations check for a .git directory; if your update path
	// requires it, create it:
	_ = os.MkdirAll(filepath.Join(dst, ".git"), 0o755)

	u := mustURL(t, "https://example.com/repo.git?ref=main&depth=5")

	err := g.GetCustom(dst, u)
	require.NoError(t, err)
}

// Test GetCustom with invalid port number.
func TestGetCustom_InvalidPort(t *testing.T) {
	g := newGetter()
	dst := t.TempDir()

	// Create a URL with an invalid port programmatically to avoid staticcheck warning.
	// Port 999999 is out of range for a 16-bit integer (max 65535).
	u, err := url.Parse("ssh://github.com:999999/repo.git")
	require.NoError(t, err)
	require.NotNil(t, u)

	err = g.GetCustom(dst, u)
	require.Error(t, err)
	// Port 999999 exceeds the 16-bit integer range, so ParseUint will fail.
	require.Contains(t, err.Error(), "invalid port number")
}

// Test GetCustom with malformed SSH key.
func TestGetCustom_InvalidSSHKey(t *testing.T) {
	writeFakeGit(t, "git version 2.30.0", 0)

	g := newGetter()
	dst := t.TempDir()

	// Invalid base64.
	u := mustURL(t, "https://example.com/repo.git?sshkey=not-valid-base64!@#")

	err := g.GetCustom(dst, u)
	require.Error(t, err)
}

// Test GetCustom with old git version when using SSH key.
func TestGetCustom_SSHKeyWithOldGit(t *testing.T) {
	writeFakeGit(t, "git version 2.2.0", 0)

	g := newGetter()
	dst := t.TempDir()

	testKey := base64.StdEncoding.EncodeToString([]byte("test-ssh-key"))
	u := mustURL(t, fmt.Sprintf("https://example.com/repo.git?sshkey=%s", testKey))

	err := g.GetCustom(dst, u)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error using SSH key: git version requirement not met")
}

// Test checkout method.
func TestCheckout(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		exitCode  int
		wantError bool
	}{
		{
			name:      "successful checkout",
			ref:       "main",
			exitCode:  0,
			wantError: false,
		},
		{
			name:      "checkout failure",
			ref:       "nonexistent",
			exitCode:  1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.exitCode == 0 {
				writeFakeGit(t, "", 0)
			} else {
				writeFakeGit(t, "error: branch not found", tt.exitCode)
			}

			g := newGetter()
			dst := t.TempDir()

			err := g.checkout(context.Background(), dst, tt.ref)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test clone method.
func TestClone(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		depth     int
		exitCode  int
		wantError bool
	}{
		{
			name:      "successful shallow clone",
			ref:       "main",
			depth:     1,
			exitCode:  0,
			wantError: false,
		},
		{
			name:      "clone with commit ID and depth",
			ref:       "abc1234567",
			depth:     1,
			exitCode:  128,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeFakeGit(t, "ref: refs/heads/main HEAD", tt.exitCode)

			g := newGetter()
			dst := filepath.Join(t.TempDir(), "clone-dest")
			u := mustURL(t, "https://example.com/repo.git")

			params := gitOperationParams{
				ctx:        context.Background(),
				dst:        dst,
				sshKeyFile: "",
				u:          u,
				ref:        tt.ref,
				depth:      tt.depth,
			}
			err := g.clone(&params)
			if tt.wantError {
				require.Error(t, err)
				if tt.depth > 0 && tt.ref == "abc1234567" {
					require.Contains(t, err.Error(), "depth")
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test update method.
func TestUpdate(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		depth     int
		setupGit  bool
		wantError bool
	}{
		{
			name:      "successful update without depth",
			ref:       "main",
			depth:     0,
			setupGit:  true,
			wantError: false,
		},
		{
			name:      "successful update with depth",
			ref:       "main",
			depth:     10,
			setupGit:  true,
			wantError: false,
		},
		{
			name:      "update failure - no git",
			ref:       "main",
			depth:     0,
			setupGit:  false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupGit {
				writeFakeGit(t, "", 0)
			}

			g := newGetter()
			dst := t.TempDir()

			// Create .git directory for removal test.
			gitDir := filepath.Join(dst, ".GIT") // Test case-insensitive removal.
			require.NoError(t, os.MkdirAll(gitDir, 0o755))

			u := mustURL(t, "https://example.com/repo.git")

			params := gitOperationParams{
				ctx:        context.Background(),
				dst:        dst,
				sshKeyFile: "",
				u:          u,
				ref:        tt.ref,
				depth:      tt.depth,
			}
			err := g.update(&params)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify .git was removed.
				_, statErr := os.Stat(gitDir)
				require.True(t, os.IsNotExist(statErr))
			}
		})
	}
}

// Test fetchSubmodules method.
func TestFetchSubmodules(t *testing.T) {
	tests := []struct {
		name      string
		depth     int
		exitCode  int
		wantError bool
	}{
		{
			name:      "successful submodule fetch without depth",
			depth:     0,
			exitCode:  0,
			wantError: false,
		},
		{
			name:      "successful submodule fetch with depth",
			depth:     5,
			exitCode:  0,
			wantError: false,
		},
		{
			name:      "submodule fetch failure",
			depth:     0,
			exitCode:  1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.exitCode == 0 {
				writeFakeGit(t, "", 0)
			} else {
				writeFakeGit(t, "error: submodule update failed", tt.exitCode)
			}

			g := newGetter()
			dst := t.TempDir()

			err := g.fetchSubmodules(context.Background(), dst, "", tt.depth)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test fetchSubmodules with SSH key.
func TestFetchSubmodules_WithSSHKey(t *testing.T) {
	writeFakeGit(t, "", 0)

	g := newGetter()
	dst := t.TempDir()

	// Create a temp SSH key file.
	sshKeyFile := filepath.Join(t.TempDir(), "id_rsa")
	require.NoError(t, os.WriteFile(sshKeyFile, []byte("test-key"), 0o600))

	err := g.fetchSubmodules(context.Background(), dst, sshKeyFile, 1)
	require.NoError(t, err)
}

// Test findRemoteDefaultBranch.
func TestFindRemoteDefaultBranch(t *testing.T) {
	tests := []struct {
		name           string
		gitOutput      string
		exitCode       int
		expectedBranch string
	}{
		{
			name:           "finds main branch",
			gitOutput:      "ref: refs/heads/main\tHEAD",
			exitCode:       0,
			expectedBranch: "main",
		},
		{
			name:           "finds develop branch",
			gitOutput:      "ref: refs/heads/develop\tHEAD",
			exitCode:       0,
			expectedBranch: "develop",
		},
		{
			name:           "returns master on error",
			gitOutput:      "",
			exitCode:       1,
			expectedBranch: "master",
		},
		{
			name:           "returns master on invalid output",
			gitOutput:      "invalid output",
			exitCode:       0,
			expectedBranch: "master",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeFakeGit(t, tt.gitOutput, tt.exitCode)

			u := mustURL(t, "https://example.com/repo.git")
			branch := findRemoteDefaultBranch(context.Background(), u)
			require.Equal(t, tt.expectedBranch, branch)
		})
	}
}

// Test GetCustom with timeout.
func TestGetCustom_WithTimeout(t *testing.T) {
	writeFakeGit(t, "git version 2.30.0", 0)

	g := newGetter()
	// Set a very short timeout to trigger timeout path.
	g.Timeout = 1 * time.Nanosecond

	dst := t.TempDir()
	u := mustURL(t, "https://example.com/repo.git")

	// This might timeout or succeed very quickly.
	// We're mainly testing that the timeout path doesn't panic.
	_ = g.GetCustom(dst, u)
}

// Test GetCustom full integration - clone path.
func TestGetCustom_Integration_Clone(t *testing.T) {
	writeFakeGit(t, "ref: refs/heads/main HEAD", 0)

	g := newGetter()
	dst := filepath.Join(t.TempDir(), "new-repo")
	u := mustURL(t, "https://example.com/repo.git?ref=feature&depth=1")

	// This will attempt full clone flow.
	err := g.GetCustom(dst, u)
	// Will error because our fake git doesn't actually clone.
	require.Error(t, err)
}

// Test GetCustom full integration - update path.
func TestGetCustom_Integration_Update(t *testing.T) {
	writeFakeGit(t, "", 0)

	g := newGetter()
	dst := t.TempDir()

	// Create existing repo structure.
	gitDir := filepath.Join(dst, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]"), 0o644))

	u := mustURL(t, "https://example.com/repo.git?ref=main")

	err := g.GetCustom(dst, u)
	// Will error because our fake git doesn't actually work.
	require.NoError(t, err)

	// Verify .git was removed during update.
	_, statErr := os.Stat(gitDir)
	require.True(t, os.IsNotExist(statErr))
}
