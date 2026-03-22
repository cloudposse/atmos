package tests

import (
"bytes"
"context"
"encoding/base64"
"fmt"
"os"
"os/exec"
"path/filepath"
"runtime"
"sort"
"strings"
"testing"
"time"

"github.com/otiai10/copy"

log "github.com/cloudposse/atmos/pkg/logger"
"github.com/cloudposse/atmos/pkg/telemetry"
"github.com/cloudposse/atmos/tests/testhelpers"
)

func runCLICommandTest(t *testing.T, tc TestCase) {
	// Enable parallel test execution unless the test explicitly opts out.
	// Parallel execution is the default to reduce overall test suite runtime.
	if tc.Parallel == nil || *tc.Parallel {
		t.Parallel()
	}

	// Clone Env immediately to prevent data races: the map inside tc is shared
	// between goroutines when tests run in parallel.
	envCopy := make(map[string]string, len(tc.Env))
	for k, v := range tc.Env {
		envCopy[k] = v
	}
	tc.Env = envCopy

	// Skip long tests in short mode
	if testing.Short() && tc.Short != nil && !*tc.Short {
		t.Skipf("Skipping long-running test in short mode (use 'go test' without -short to run)")
	}

	// Check preconditions before running the test
	checkPreconditions(t, tc.Preconditions)

	// Skip tests that require the Atmos binary when it could not be built in TestMain.
	if tc.Command == "atmos" && atmosRunner == nil {
		t.Skipf("Atmos binary not available (build failed during TestMain setup)")
	}

	// Create a context with timeout if specified, defaulting to 10 minutes
	// to prevent any single test from consuming the entire test budget.
	const defaultTestTimeout = 10 * time.Minute

	var ctx context.Context
	var cancel context.CancelFunc

	if tc.Expect.Timeout != "" {
		// Parse the timeout from the Expectation
		timeout, err := parseTimeout(tc.Expect.Timeout)
		if err != nil {
			t.Fatalf("Failed to parse timeout for test %s: %v", tc.Name, err)
		}
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), timeout)
		} else {
			ctx, cancel = context.WithTimeout(context.Background(), defaultTestTimeout)
		}
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), defaultTestTimeout)
	}
	defer cancel()

	// Create a temporary HOME directory for the test case that's clean
	// Otherwise a test may pass/fail due to existing files in the user's HOME directory
	tempDir := t.TempDir()

	// ALWAYS set XDG_CACHE_HOME to a clean temp directory for test isolation.
	// Each parallel test gets its own XDG dirs so there is no shared state.
	// No xdg.Reload() or t.Setenv() needed: all vars are passed directly to the
	// subprocess via cmd.Env rather than via the process environment.
	xdgCacheHome := filepath.Join(tempDir, ".cache")
	tc.Env["XDG_CACHE_HOME"] = xdgCacheHome

	// Clear github_username environment variables for consistent snapshots.
	// These are automatically bound to settings.github_username but cause
	// environment-dependent output. Skip clearing only for vendor tests that need GitHub auth.
	// The vars are cleared from the subprocess env later in the envVars filtering step.
	clearGitHubVarsForSnapshots := !strings.Contains(tc.Name, "vendor")

	// Prevent git from hanging waiting for credentials or interactive input.
	// On macOS CI, the actions/checkout step configures git credentials as local config
	// in the repo's .git/config, but vendor tests clone from different directories
	// where this local config is not available, causing git to hang.
	if _, exists := tc.Env["GIT_TERMINAL_PROMPT"]; !exists {
		tc.Env["GIT_TERMINAL_PROMPT"] = "0"
	}

	// Configure git for non-interactive use via GIT_CONFIG_* env vars (Git 2.31+).
	// macOS ships with credential.helper=osxkeychain in the system-level git config
	// (/Library/Developer/CommandLineTools/.../gitconfig). This is NOT in ~/.gitconfig,
	// so it persists even when HOME is overridden to a temp directory. When git clone
	// runs, the osxkeychain helper tries to store/retrieve credentials:
	//   - On CI (headless): hangs forever because there's no UI for Keychain
	//   - Locally on macOS: shows a Keychain popup asking permission
	// We fix this by disabling credential.helper and injecting GITHUB_TOKEN directly.
	if _, exists := tc.Env["GIT_CONFIG_COUNT"]; !exists {
		if githubToken := os.Getenv("GITHUB_TOKEN"); githubToken != "" {
			// Disable credential helper (prevents osxkeychain hangs/popups) and inject token.
			basicAuth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + githubToken))
			tc.Env["GIT_CONFIG_COUNT"] = "2"
			tc.Env["GIT_CONFIG_KEY_0"] = "credential.helper"
			tc.Env["GIT_CONFIG_VALUE_0"] = ""
			tc.Env["GIT_CONFIG_KEY_1"] = "http.https://github.com/.extraheader"
			tc.Env["GIT_CONFIG_VALUE_1"] = "AUTHORIZATION: basic " + basicAuth
		} else {
			// No token available — just disable the credential helper to prevent hangs/popups.
			tc.Env["GIT_CONFIG_COUNT"] = "1"
			tc.Env["GIT_CONFIG_KEY_0"] = "credential.helper"
			tc.Env["GIT_CONFIG_VALUE_0"] = ""
		}
	}

	if runtime.GOOS == "darwin" && isCIEnvironment() {
		// For some reason the empty HOME directory causes issues on macOS in GitHub Actions
		// Copying over the `.gitconfig` was not enough to fix the issue
		logger.Info("skipping empty home dir on macOS in CI", "GOOS", runtime.GOOS)
	} else {
		// Set environment variables for the test case.
		// These are written to tc.Env so they reach the subprocess via cmd.Env;
		// no t.Setenv() is needed or used.
		tc.Env["HOME"] = tempDir
		tc.Env["XDG_CONFIG_HOME"] = filepath.Join(tempDir, ".config")
		tc.Env["XDG_DATA_HOME"] = filepath.Join(tempDir, ".local", "share")
		// Copy some files to the temporary HOME directory
		originalHome := os.Getenv("HOME")
		filesToCopy := []string{".gitconfig", ".ssh", ".netrc"} // Expand list if needed
		for _, file := range filesToCopy {
			src := filepath.Join(originalHome, file)
			dest := filepath.Join(tempDir, file)

			if _, err := os.Stat(src); err == nil { // Check if the file/directory exists
				// t.Logf("Copying %s to %s\n", src, dest)
				// Skip socket files (e.g., SSH agent sockets) that cannot be copied.
				copyOpts := copy.Options{
					Skip: func(info os.FileInfo, src, dest string) (bool, error) {
						return info.Mode()&os.ModeSocket != 0, nil
					},
				}
				if err := copy.Copy(src, dest, copyOpts); err != nil {
					t.Fatalf("Failed to copy %s to test folder: %v", src, err)
				}
			}
		}
	}

	// Resolve the absolute workdir once; used throughout the function.
	// filepath.Abs is evaluated against the test process's starting directory
	// (tests/) which is stable across parallel tests since we never call t.Chdir().
	var absoluteWorkdir string
	if tc.Workdir != "" {
		var err error
		absoluteWorkdir, err = filepath.Abs(tc.Workdir)
		if err != nil {
			t.Fatalf("failed to resolve absolute path of workdir %q: %v", tc.Workdir, err)
		}

		// Setup sandbox environment if enabled
		var sandboxEnv *testhelpers.SandboxEnvironment
		switch v := tc.Sandbox.(type) {
		case bool:
			if v {
				// Boolean true = isolated sandbox for this test only
				logger.Info("Setting up isolated sandbox", "test", tc.Name, "workdir", absoluteWorkdir)
				sandboxEnv = createIsolatedSandbox(t, absoluteWorkdir)
				// Clean up immediately after test
				defer func() {
					logger.Debug("Cleaning up isolated sandbox", "tempdir", sandboxEnv.TempDir)
					sandboxEnv.Cleanup()
				}()
			}
		case string:
			if v != "" {
				// Named sandbox = shared across related tests
				logger.Info("Using named sandbox", "test", tc.Name, "name", v, "workdir", absoluteWorkdir)
				sandboxEnv = getOrCreateNamedSandbox(t, v, absoluteWorkdir)
				// Cleanup handled by TestMain
			}
		}

		// Add sandbox environment variables to override component paths
		if sandboxEnv != nil {
			if tc.Env == nil {
				tc.Env = make(map[string]string)
			}
			for k, v := range sandboxEnv.GetEnvironmentVariables() {
				logger.Debug("Setting sandbox env var", "key", k, "value", v)
				tc.Env[k] = v
			}
		}

		// Clean the directory if enabled
		if tc.Clean {
			logger.Info("Cleaning directory", "workdir", tc.Workdir)
			if err := cleanDirectory(t, absoluteWorkdir); err != nil {
				t.Fatalf("Failed to clean directory %q: %v", tc.Workdir, err)
			}
		}

		// Set ATMOS_CLI_CONFIG_PATH to ensure test isolation.
		// This forces Atmos to use the workdir's atmos.yaml instead of searching
		// up the directory tree or using ~/.config/atmos/atmos.yaml.
		// BUT: Skip this for tests that use --chdir/-C, since they need to test
		// config loading in the target directory.
		usesChdirFlag := false
		for _, arg := range tc.Args {
			if arg == "--chdir" || arg == "-C" || strings.HasPrefix(arg, "--chdir=") {
				usesChdirFlag = true
				break
			}
		}
		if !usesChdirFlag {
			atmosConfigPath := filepath.Join(absoluteWorkdir, "atmos.yaml")
			if _, err := os.Stat(atmosConfigPath); err == nil {
				if tc.Env == nil {
					tc.Env = make(map[string]string)
				}
				tc.Env["ATMOS_CLI_CONFIG_PATH"] = absoluteWorkdir
				logger.Debug("Setting ATMOS_CLI_CONFIG_PATH for test isolation", "path", absoluteWorkdir)
			}
		} else {
			logger.Debug("Skipping ATMOS_CLI_CONFIG_PATH for --chdir test")
		}
	}

	// Include the system PATH in the test environment
	tc.Env["PATH"] = os.Getenv("PATH")

	// Set the test Git root to a clean temporary directory
	// This makes each test scenario act as if it's its own Git repository
	// preventing the actual repository's .atmos.d from being loaded
	// This is especially important for tests that use workdir: "../"
	testGitRoot := filepath.Join(tempDir, "mock-git-root")
	if err := os.MkdirAll(testGitRoot, 0o755); err == nil {
		tc.Env["TEST_GIT_ROOT"] = testGitRoot
	}

	// Also set an environment variable to exclude the repository's .atmos.d
	// This is needed for tests that change to parent directories
	tc.Env["TEST_EXCLUDE_ATMOS_D"] = repoRoot

	// Disable git root base path discovery in tests.
	// Tests expect BasePath to be "." by default, not the repository root.
	tc.Env["ATMOS_GIT_ROOT_BASEPATH"] = "false"

	// Force consistent color/terminal environment for reproducible ANSI codes across platforms.
	// Test cases can still override these by explicitly setting them.
	if _, exists := tc.Env["TERM"]; !exists {
		tc.Env["TERM"] = "xterm-256color"
	}
	if _, exists := tc.Env["COLORTERM"]; !exists {
		tc.Env["COLORTERM"] = "" // Explicitly empty to prevent truecolor (force 256-color)
	}
	if _, exists := tc.Env["COLUMNS"]; !exists {
		tc.Env["COLUMNS"] = "80" // Force consistent terminal width for table rendering
	}

	// Prepare the command based on what's being tested
	var cmd *exec.Cmd
	if tc.Command == "atmos" {
		cmd = prepareAtmosCommand(t, ctx, tc.Args...)
	} else {
		// For non-atmos commands, use regular exec
		binaryPath, err := exec.LookPath(tc.Command)
		if err != nil {
			t.Fatalf("Binary not found: %s", tc.Command)
		}
		cmd = exec.CommandContext(ctx, binaryPath, tc.Args...)
	}

	// Set the subprocess working directory explicitly.
	// This replaces the former t.Chdir() call, which is incompatible with t.Parallel()
	// because the Go testing framework disallows t.Parallel() + t.Chdir() together.
	// Setting cmd.Dir directly achieves the same isolation without touching the
	// test goroutine's working directory.
	//
	// NOTE: cmd.Dir only changes the subprocess's CWD — the test process's CWD remains
	// the tests/ directory. Any file-existence or file-content checks that use
	// relative paths must be resolved against absoluteWorkdir explicitly (see
	// resolveFilePaths / resolveFilePathsMap helpers below).
	if absoluteWorkdir != "" {
		cmd.Dir = absoluteWorkdir
	}

	// Register cleanup for relative GITHUB_OUTPUT / GITHUB_STEP_SUMMARY files.
	// These are written to absoluteWorkdir by the subprocess and must be removed
	// after the test completes so that a subsequent run does not read a stale file
	// written by a previous (possibly failed) run.  Absolute paths are left alone.
	for _, envKey := range ghaOutputEnvVars {
		if relPath, ok := tc.Env[envKey]; ok && !filepath.IsAbs(relPath) && absoluteWorkdir != "" {
			absPath := filepath.Join(absoluteWorkdir, relPath)
			t.Cleanup(func() { _ = os.Remove(absPath) })
		}
	}

	// Preserve GOCOVERDIR if it's already set by atmosRunner
	existingEnv := cmd.Env
	if existingEnv == nil {
		existingEnv = []string{}
	}

	// Filter CI environment variables from the inherited (process) environment BEFORE
	// merging test-specific vars.  This mirrors the old PreserveCIEnvVars() approach:
	// CI vars are stripped from what the runner inherits from the shell, but test cases
	// that explicitly set a CI var (e.g. "CI: true") will have it restored below via the
	// tc.Env override loop.  Using a pure-function filter instead of mutating the process
	// environment makes this approach safe for parallel tests.
	existingEnv = telemetry.FilterCIEnvVars(existingEnv)

	// Preserve all environment variables from AtmosRunner (including PATH and GOCOVERDIR)
	// and add/override with test-specific environment variables
	var envVars []string

	// Start with the environment from AtmosRunner if available
	if len(existingEnv) > 0 {
		envVars = append(envVars, existingEnv...)
	}

	// Add/override test-specific environment variables
	for key, value := range tc.Env {
		// NEVER allow test cases to override PATH - AtmosRunner's PATH must be preserved
		if key == "PATH" {
			continue
		}

		// Remove all occurrences of the key before adding the new value.  Duplicates
		// can arise when the AtmosRunner env and the base env both carry the same key.
		filtered := make([]string, 0, len(envVars))
		for _, env := range envVars {
			if !strings.HasPrefix(env, key+"=") {
				filtered = append(filtered, env)
			}
		}
		envVars = append(filtered, fmt.Sprintf("%s=%s", key, value))
	}

	// Ensure NO_COLOR is not inherited unless test explicitly sets it (presence disables color).
	if _, exists := tc.Env["NO_COLOR"]; !exists {
		filtered := make([]string, 0, len(envVars))
		for _, env := range envVars {
			if !strings.HasPrefix(env, "NO_COLOR=") {
				filtered = append(filtered, env)
			}
		}
		envVars = filtered
	}

	// Filter out ATMOS_* environment variables that shouldn't be inherited from developer's shell.
	// This ensures test reproducibility between local and CI environments.
	// Only allow ATMOS_* vars explicitly set in tc.Env.
	atmosVarsToFilter := []string{
		"ATMOS_LOGS_LEVEL", // Can change log verbosity and affect snapshot output
		"ATMOS_CHDIR",      // Can change working directory resolution
		"ATMOS_LOGS_FILE",  // Can redirect logs to unexpected locations
	}

	for _, atmosVar := range atmosVarsToFilter {
		// Skip if test explicitly sets this variable
		if _, exists := tc.Env[atmosVar]; exists {
			continue
		}

		// Remove all occurrences from the inherited environment (duplicates can arise
		// when the AtmosRunner env and os.Environ() both carry the same variable).
		filtered := make([]string, 0, len(envVars))
		for _, env := range envVars {
			if !strings.HasPrefix(env, atmosVar+"=") {
				filtered = append(filtered, env)
			}
		}
		envVars = filtered
	}

	// Clear GitHub username variables for consistent snapshots.
	// These are automatically bound to settings.github_username in Atmos and
	// produce environment-dependent output.  Vendor tests are exempted because
	// they require GitHub auth.
	if clearGitHubVarsForSnapshots {
		for _, varName := range gitHubUsernameVars {
			// Only clear if the test hasn't explicitly set the variable.
			if _, exists := tc.Env[varName]; exists {
				continue
			}
			// Remove all occurrences (duplicates can arise when os.Environ() and the
			// AtmosRunner env both carry the same variable name).
			filtered := make([]string, 0, len(envVars))
			for _, env := range envVars {
				if !strings.HasPrefix(env, varName+"=") {
					filtered = append(filtered, env)
				}
			}
			envVars = filtered
		}
	}

	// Set up XDG directories in temp locations to ensure test isolation.
	// This prevents tests from:
	// 1. Reading user's telemetry acknowledgment state (causing inconsistent telemetry notices)
	// 2. Writing to user's cache/config/data directories
	// 3. Being affected by user's XDG environment settings
	//
	// Use the existing tempDir to ensure XDG paths share the same root directory.
	// This preserves isolation and avoids bypass issues when tc.Env already contains XDG vars.
	xdgTempDir := filepath.Join(tempDir, "xdg")
	xdgVars := map[string]string{
		"XDG_CACHE_HOME":        filepath.Join(xdgTempDir, "cache"),
		"XDG_CONFIG_HOME":       filepath.Join(xdgTempDir, "config"),
		"XDG_DATA_HOME":         filepath.Join(xdgTempDir, "data"),
		"ATMOS_XDG_CACHE_HOME":  filepath.Join(xdgTempDir, "cache"),
		"ATMOS_XDG_CONFIG_HOME": filepath.Join(xdgTempDir, "config"),
		"ATMOS_XDG_DATA_HOME":   filepath.Join(xdgTempDir, "data"),
	}

	// Add XDG vars to environment unless test explicitly sets them.
	// Build the sorted list of XDG var names once for deterministic ordering.
	xdgVarNames := make([]string, 0, len(xdgVars))
	for k := range xdgVars {
		if _, overridden := tc.Env[k]; !overridden {
			xdgVarNames = append(xdgVarNames, k)
		}
	}
	sort.Strings(xdgVarNames)

	if len(xdgVarNames) > 0 {
		// Build a prefix-set for the XDG vars that need replacing so we can remove
		// all of them in a single O(n) pass instead of one pass per variable.
		xdgPrefixes := make(map[string]struct{}, len(xdgVarNames))
		for _, name := range xdgVarNames {
			xdgPrefixes[name+"="] = struct{}{}
		}

		// Remove all inherited occurrences of the XDG vars we will inject.
		// Duplicates can arise when AtmosRunner env and os.Environ() both carry
		// the same variable name.
		filtered := make([]string, 0, len(envVars))
		for _, env := range envVars {
			keep := true
			for prefix := range xdgPrefixes {
				if strings.HasPrefix(env, prefix) {
					keep = false
					break
				}
			}
			if keep {
				filtered = append(filtered, env)
			}
		}
		envVars = filtered

		// Append each isolated XDG var in sorted order.
		for _, xdgVar := range xdgVarNames {
			envVars = append(envVars, fmt.Sprintf("%s=%s", xdgVar, xdgVars[xdgVar]))
		}
	}

	// Give each parallel test subprocess its own GOCOVERDIR subdirectory to prevent
	// rename races. When multiple parallel tests share one GOCOVERDIR, they all try
	// to write the same covmeta.<hash> file on exit (the hash is deterministic from
	// the binary), and the atomic rename fails on macOS. Using a per-test directory
	// avoids the conflict; coverage files are merged into the shared dir afterward.
	if coverDir != "" {
		perTestCoverDir := filepath.Join(tempDir, "gocoverdir")
		if err := os.MkdirAll(perTestCoverDir, 0o755); err == nil {
			// Update the existing GOCOVERDIR entry if present, or append a new one.
			// The entry may be absent for non-atmos commands whose cmd.Env starts empty.
			found := false
			for i, env := range envVars {
				if strings.HasPrefix(env, "GOCOVERDIR=") {
					envVars[i] = fmt.Sprintf("GOCOVERDIR=%s", perTestCoverDir)
					found = true
					break
				}
			}
			if !found {
				envVars = append(envVars, fmt.Sprintf("GOCOVERDIR=%s", perTestCoverDir))
			}
			// After the subprocess exits, merge its coverage data into the shared dir.
			defer mergeIntoCoverDir(perTestCoverDir, coverDir)
		}
	}

	cmd.Env = envVars

	var stdout, stderr bytes.Buffer
	var exitCode int

	if tc.Tty {
		// Run the command in TTY mode
		ptyOutput, err := simulateTtyCommand(t, cmd, "")

		// Check if the context timeout was exceeded
		if ctx.Err() == context.DeadlineExceeded {
			t.Errorf("Reason: Test timed out after %s", tc.Expect.Timeout)
			t.Errorf("Captured stdout:\n%s", stdout.String())
			t.Errorf("Captured stderr:\n%s", stderr.String())
			return
		}

		if err != nil {
			// Check if the error is an ExitError
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Capture the actual exit code
				exitCode = exitErr.ExitCode()

				if exitCode < 0 {
					// Negative exit code indicates interruption by a signal
					t.Errorf("TTY Command interrupted by signal: %s, Signal: %d, Error: %v", tc.Command, -exitCode, err)
				}
			} else {
				// Handle other types of errors
				t.Fatalf("Failed to simulate TTY command: %v", err)
			}
		}
		stdout.WriteString(ptyOutput)
	} else {
		// Run the command in non-TTY mode

		// Attach stdout and stderr buffers for non-TTY execution
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if ctx.Err() == context.DeadlineExceeded {
			// Handle the timeout case first
			t.Errorf("Reason: Test timed out after %s", tc.Expect.Timeout)
			t.Errorf("Captured stdout:\n%s", stdout.String())
			t.Errorf("Captured stderr:\n%s", stderr.String())
			return
		}

		if err != nil {
			// Handle other command execution errors
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Capture the actual exit code
				exitCode = exitErr.ExitCode()

				if exitCode < 0 {
					// Negative exit code indicates termination by a signal
					t.Errorf("Non-TTY Command terminated by signal: %s, Signal: %d, Error: %v", tc.Command, -exitCode, err)
				}
			} else {
				// Handle other non-exec-related errors
				t.Fatalf("Failed to run command; Error: %v", err)
			}
		} else {
			// Successful command execution
			exitCode = 0
		}
	}

	// Validate outputs
	if !verifyExitCode(t, tc.Expect.ExitCode, exitCode) {
		t.Errorf("Description: %s", tc.Description)
	}

	// Validate output based on TTY mode
	verifyTestOutputs(t, &tc, stdout.String(), stderr.String())

	// Validate format (YAML/JSON)
	if len(tc.Expect.Valid) > 0 {
		if !verifyFormatValidation(t, stdout.String(), tc.Expect.Valid) {
			t.Errorf("Format validation failed for test: %s", tc.Name)
			t.Errorf("Description: %s", tc.Description)
		}
	}

	// Validate file existence.
	// Resolve relative paths against absoluteWorkdir so they work correctly when
	// running in parallel (we use cmd.Dir instead of t.Chdir, so the test process
	// CWD is NOT the workdir).
	if !verifyFileExists(t, resolveFilePaths(tc.Expect.FileExists, absoluteWorkdir)) {
		t.Errorf("Description: %s", tc.Description)
	}

	// Validate file not existence
	if !verifyFileNotExists(t, resolveFilePaths(tc.Expect.FileNotExists, absoluteWorkdir)) {
		t.Errorf("Description: %s", tc.Description)
	}

	// Validate file contents
	if !verifyFileContains(t, resolveFilePathsMap(tc.Expect.FileContains, absoluteWorkdir)) {
		t.Errorf("Description: %s", tc.Description)
	}

	// Validate snapshots
	if !verifySnapshot(t, tc, stdout.String(), stderr.String(), *regenerateSnapshots) {
		t.Errorf("Description: %s", tc.Description)
	}
}
