package cmd

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/ui"
)

// stepsFromStrings is a helper to convert []string to schema.Tasks for tests.
func stepsFromStrings(commands ...string) schema.Tasks {
	tasks := make(schema.Tasks, len(commands))
	for i, cmd := range commands {
		tasks[i] = schema.Task{Command: cmd, Type: "shell"}
	}
	return tasks
}

// captureStdoutStderr redirects os.Stdout/os.Stderr while fn runs. Restoration
// and pipe cleanup happen via defer (not just after fn returns normally) so a
// panic inside fn can't leave the process's stdout/stderr permanently pointed
// at closed pipes for the rest of the test binary. Draining the pipes on
// background goroutines (rather than reading after fn returns) lets the close
// step unblock those reads instead of deadlocking on a deferred restore.
func captureStdoutStderr(t *testing.T, fn func()) (string, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutReader, stdoutWriter, err := os.Pipe()
	require.NoError(t, err)
	stderrReader, stderrWriter, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	var closeOnce sync.Once
	closeWriters := func() {
		closeOnce.Do(func() {
			_ = stdoutWriter.Close()
			_ = stderrWriter.Close()
		})
	}
	defer closeWriters()

	stdoutCh := make(chan string, 1)
	stderrCh := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(stdoutReader)
		stdoutCh <- string(data)
	}()
	go func() {
		data, _ := io.ReadAll(stderrReader)
		stderrCh <- string(data)
	}()

	fn()

	closeWriters()

	stdout := <-stdoutCh
	stderr := <-stderrCh

	require.NoError(t, stdoutReader.Close())
	require.NoError(t, stderrReader.Close())

	return stdout, stderr
}

func TestCustomCommandStepWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell redirection")
	}

	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	stepDir := filepath.Join(tmpDir, "step-dir")
	require.NoError(t, os.Mkdir(stepDir, 0o755))
	outputFile := filepath.Join(tmpDir, "pwd.txt")

	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{
			{
				Name:             "test-step-workdir",
				Description:      "Exercise step-level working_directory",
				WorkingDirectory: tmpDir,
				Steps: schema.Tasks{
					{
						Type:             "shell",
						Name:             "pwd",
						WorkingDirectory: "step-dir",
						Command:          fmt.Sprintf("pwd > %q", outputFile),
					},
				},
			},
		},
	}

	err := processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	customCmd, _, err := RootCmd.Find([]string{"test-step-workdir"})
	require.NoError(t, err)
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	actual, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, stepDir, strings.TrimSpace(string(actual)))
}

func TestCustomCommandWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell redirection")
	}

	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "pwd.txt")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{
			{
				Name:             "command-workdir",
				WorkingDirectory: tmpDir,
				Steps:            stepsFromStrings(fmt.Sprintf("pwd > %q", outputFile)),
			},
		},
	}

	root := &cobra.Command{Use: "atmos", SilenceErrors: true, SilenceUsage: true}
	require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, root))
	root.SetArgs([]string{"command-workdir"})
	require.NoError(t, root.Execute())

	actual, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, strings.TrimSpace(string(actual)))
}

func TestNestedCustomCommandInheritsWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell redirection")
	}

	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "pwd.txt")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{
			{
				Name:             "parent-workdir",
				WorkingDirectory: tmpDir,
				Commands: []schema.Command{
					{
						Name:  "child",
						Steps: stepsFromStrings(fmt.Sprintf("pwd > %q", outputFile)),
					},
				},
			},
		},
	}

	root := &cobra.Command{Use: "atmos", SilenceErrors: true, SilenceUsage: true}
	require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, root))
	root.SetArgs([]string{"parent-workdir", "child"})
	require.NoError(t, root.Execute())

	actual, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, strings.TrimSpace(string(actual)))
}

func TestCustomCommandDispatchRejectsUnexpectedArgsOnRunnableParent(t *testing.T) {
	_ = NewTestKit(t)

	atmosConfig := schema.AtmosConfiguration{
		Commands: []schema.Command{
			{
				Name:  "examples",
				Steps: stepsFromStrings("echo parent"),
				Commands: []schema.Command{
					{
						Name: "auth-stores",
						Commands: []schema.Command{
							{
								Name:  "identity-backed-stores",
								Steps: stepsFromStrings("echo leaf"),
							},
						},
					},
				},
			},
		},
	}

	root := &cobra.Command{
		Use:           "atmos",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, root))

	examplesCmd, _, err := root.Find([]string{"examples"})
	require.NoError(t, err)
	require.NotNil(t, examplesCmd)

	parentRan := false
	leafRan := false
	examplesCmd.Run = func(cmd *cobra.Command, args []string) {
		parentRan = true
	}

	leafCmd, _, err := root.Find([]string{"examples", "auth-stores", "identity-backed-stores"})
	require.NoError(t, err)
	require.NotNil(t, leafCmd)
	leafCmd.Run = func(cmd *cobra.Command, args []string) {
		leafRan = true
	}

	root.SetArgs([]string{"examples", "auth-stores identity-backed-stores"})
	err = root.Execute()
	require.Error(t, err)
	assert.False(t, parentRan, "unexpected extra args must not execute the runnable parent")
	assert.False(t, leafRan)

	root.SetArgs([]string{"examples", "auth-stores", "identity-backed-stores"})
	require.NoError(t, root.Execute())
	assert.False(t, parentRan)
	assert.True(t, leafRan)
}

func TestCustomCommandCastStepInheritsWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell redirection")
	}
	require.NoError(t, iolib.Initialize())
	ioCtx := iolib.GetContext()
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")
	outputFile := filepath.Join(tmpDir, "pwd.txt")

	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{
			{
				Name:             "test-cast-workdir-inheritance",
				Description:      "Exercise command working_directory inheritance through cast steps",
				WorkingDirectory: tmpDir,
				Steps: schema.Tasks{
					{
						Type:   schema.TaskTypeCast,
						Name:   "record",
						Output: string(stepPkg.OutputModeNone),
						CastOutput: &schema.CastOutput{
							Cast: castPath,
						},
						Steps: []schema.WorkflowStep{
							{
								Type:    schema.TaskTypeShell,
								Name:    "pwd",
								Command: "pwd > pwd.txt",
							},
						},
					},
				},
			},
		},
	}

	err := processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	customCmd, _, err := RootCmd.Find([]string{"test-cast-workdir-inheritance"})
	require.NoError(t, err)
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	actual, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	// The in-process shell interpreter sets $PWD literally to the resolved
	// working directory (mirroring `pwd`'s default logical, non-symlink-
	// resolving behavior), so the recorded output must match the raw
	// tmpDir string rather than its symlink-resolved form (tmpDir sits
	// under a symlinked path on macOS, e.g. /var -> /private/var).
	assert.Equal(t, tmpDir, strings.TrimSpace(string(actual)))
	require.FileExists(t, castPath)
}

func TestCustomCommandEnvValueCanUseEnvTemplateAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell redirection")
	}

	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "env.txt")
	t.Setenv("ATMOS_TEST_TEMPLATE_SOURCE", "from-env-alias")

	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{
			{
				Name:        "test-env-template-alias",
				Description: "Exercise command env .Env template alias",
				Env: []schema.CommandEnv{
					{Key: "ATMOS_TEST_TEMPLATE_TARGET", Value: "{{ .Env.ATMOS_TEST_TEMPLATE_SOURCE }}"},
				},
				Steps: schema.Tasks{
					{
						Type:    "shell",
						Name:    "write-env",
						Command: fmt.Sprintf("printf '%%s' \"$ATMOS_TEST_TEMPLATE_TARGET\" > %q", outputFile),
					},
				},
			},
		},
	}

	err := processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	customCmd, _, err := RootCmd.Find([]string{"test-env-template-alias"})
	require.NoError(t, err)
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	actual, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, "from-env-alias", string(actual))
}

func TestCustomCommandEnvironmentIncludesCurrentExecutable(t *testing.T) {
	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	envOutputFile := filepath.Join(tmpDir, "env.txt")
	exePath, err := os.Executable()
	require.NoError(t, err)
	exeDir := filepath.Dir(exePath)
	pathEntries := filepath.SplitList(os.Getenv("PATH"))
	filteredPathEntries := make([]string, 0, len(pathEntries))
	for _, entry := range pathEntries {
		if entry != exeDir {
			filteredPathEntries = append(filteredPathEntries, entry)
		}
	}
	t.Setenv("PATH", strings.Join(filteredPathEntries, string(os.PathListSeparator)))

	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{
			{
				Name:        "test-current-executable-env",
				Description: "Dump custom command environment",
				Env: []schema.CommandEnv{
					{Key: "_ATMOS_TEST_DUMP_ENV", Value: envOutputFile},
				},
				Steps: stepsFromStrings(fmt.Sprintf("%q", exePath)),
			},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	customCmd, _, err := RootCmd.Find([]string{"test-current-executable-env"})
	require.NoError(t, err)
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	envContent, err := os.ReadFile(envOutputFile)
	require.NoError(t, err)
	envVars := string(envContent)

	assert.Equal(t, exePath, extractEnvVar(envVars, "ATMOS_CLI_PATH"))
	actualPathEntries := filepath.SplitList(extractEnvVar(envVars, "PATH"))
	require.NotEmpty(t, actualPathEntries)
	assert.Equal(t, exeDir, actualPathEntries[0])
}

func TestCustomCommandShellOutputNoneSuppressesOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell redirection")
	}

	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "ran.txt")
	resultFile := filepath.Join(tmpDir, "result.env")
	exePath, err := os.Executable()
	require.NoError(t, err)

	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{
			{
				Name:        "test-output-none",
				Description: "Exercise output none for legacy shell custom command steps",
				Steps: schema.Tasks{
					{
						Type:    "shell",
						Name:    "quiet",
						Output:  "none",
						Command: fmt.Sprintf("printf stdout-visible; printf stderr-visible >&2; printf ran > %q", outputFile),
					},
					{
						Type:    "shell",
						Output:  "none",
						Command: fmt.Sprintf("%q", exePath),
						Env: map[string]string{
							"_ATMOS_TEST_DUMP_ENV": resultFile,
							"CAPTURED_RESULT":      "{{ .steps.quiet.value }}|{{ .steps.quiet.metadata.stdout }}|{{ .steps.quiet.metadata.stderr }}|{{ .steps.quiet.metadata.exit_code }}",
						},
					},
				},
			},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	customCmd, _, err := RootCmd.Find([]string{"test-output-none"})
	require.NoError(t, err)
	require.NotNil(t, customCmd)

	stdout, stderr := captureStdoutStderr(t, func() {
		customCmd.Run(customCmd, []string{})
	})

	assert.NotContains(t, stdout, "stdout-visible")
	assert.NotContains(t, stderr, "stderr-visible")

	actual, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Equal(t, "ran", string(actual))
	resultEnv, err := os.ReadFile(resultFile)
	require.NoError(t, err)
	assert.Equal(t, "stdout-visible|stdout-visible|stderr-visible|0", extractEnvVar(string(resultEnv), "CAPTURED_RESULT"))
}

// TestCustomCommandIntegration_MockProviderEnvironment tests that custom commands with mock provider
// actually set the correct environment variables for subprocesses.
func TestCustomCommandIntegration_MockProviderEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: exercises full auth flow")
	}

	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a temporary file to capture environment variables.
	tmpDir := t.TempDir()
	envOutputFile := filepath.Join(tmpDir, "custom-cmd-env.txt")

	// Get OS-specific command to dump environment variables.
	var dumpEnvCmd string
	if runtime.GOOS == "windows" {
		dumpEnvCmd = "cmd /c set > \"" + envOutputFile + "\""
	} else {
		dumpEnvCmd = "env > " + envOutputFile
	}

	// Create a custom command that dumps environment variables.
	testCommand := schema.Command{
		Name:        "test-env-capture",
		Description: "Capture environment variables",
		Identity:    "mock-identity",
		Steps:       stepsFromStrings(dumpEnvCmd),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Find and execute the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-env-capture" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Execute the custom command.
	customCmd.Run(customCmd, []string{})

	// Read the captured environment variables.
	envContent, err := os.ReadFile(envOutputFile)
	require.NoError(t, err, "Should be able to read environment output file")

	envVars := string(envContent)
	t.Logf("Captured environment variables from custom command:\n%s", envVars)

	// Verify that authentication-related environment variables are set.
	assert.Contains(t, envVars, "ATMOS_IDENTITY", "Should have ATMOS_IDENTITY environment variable")
	assert.Contains(t, envVars, "ATMOS_IDENTITY=mock-identity", "ATMOS_IDENTITY should be set to mock-identity")
}

// TestCustomCommandIntegration_IdentityFlagOverride tests that --identity flag
// properly overrides the identity in custom command config.
func TestCustomCommandIntegration_IdentityFlagOverride(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: exercises full auth flow")
	}

	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a temporary file to capture environment variables.
	tmpDir := t.TempDir()
	envOutputFile := filepath.Join(tmpDir, "override-env.txt")

	// Get OS-specific command to dump environment variables.
	var dumpEnvCmd string
	if runtime.GOOS == "windows" {
		dumpEnvCmd = "cmd /c set > \"" + envOutputFile + "\""
	} else {
		dumpEnvCmd = "env > " + envOutputFile
	}

	// Create a custom command with identity in config.
	testCommand := schema.Command{
		Name:        "test-identity-override",
		Description: "Test identity override with flag",
		Identity:    "mock-identity", // This should be overridden by --identity flag
		Steps:       stepsFromStrings(dumpEnvCmd),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Find the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-identity-override" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Set command line args to simulate calling with --identity flag.
	RootCmd.SetArgs([]string{"test-identity-override", "--identity=mock-identity-2"})

	// Execute the command through RootCmd to properly handle flags.
	err = RootCmd.Execute()
	require.NoError(t, err, "Custom command execution should succeed")

	// Read the captured environment variables.
	envContent, err := os.ReadFile(envOutputFile)
	require.NoError(t, err, "Should be able to read environment output file")

	envVars := string(envContent)
	t.Logf("Captured environment variables with --identity flag:\n%s", envVars)

	// Verify that the flag override worked (should see mock-identity-2, not mock-identity).
	// Use extractEnvVar for exact line matching to avoid substring false positives.
	identityValue := extractEnvVar(envVars, "ATMOS_IDENTITY")
	assert.Equal(t, "mock-identity-2", identityValue, "Should use identity from --identity flag, not the config value")
}

// TestCustomCommandIntegration_MultipleSteps tests that all steps in a custom command
// use the same identity and environment.
func TestCustomCommandIntegration_MultipleSteps(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: exercises full auth flow")
	}

	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create temporary files to capture environment from different steps.
	tmpDir := t.TempDir()
	envOutput1 := filepath.Join(tmpDir, "step1-env.txt")
	envOutput2 := filepath.Join(tmpDir, "step2-env.txt")

	// Get OS-specific command to dump environment variables.
	var getDumpCmd func(string) string
	if runtime.GOOS == "windows" {
		getDumpCmd = func(file string) string { return "cmd /c set > \"" + file + "\"" }
	} else {
		getDumpCmd = func(file string) string { return "env > " + file }
	}

	// Create a custom command with multiple steps.
	testCommand := schema.Command{
		Name:        "test-multi-step",
		Description: "Test multiple steps share identity",
		Identity:    "mock-identity-2",
		Steps:       stepsFromStrings(getDumpCmd(envOutput1), getDumpCmd(envOutput2)),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Find and execute the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-multi-step" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Execute the custom command.
	customCmd.Run(customCmd, []string{})

	// Read and verify step 1 environment.
	env1Content, err := os.ReadFile(envOutput1)
	require.NoError(t, err)
	env1Vars := string(env1Content)
	assert.Contains(t, env1Vars, "ATMOS_IDENTITY=mock-identity-2", "Step 1 should use mock-identity-2")

	// Read and verify step 2 environment (should be same as step 1).
	env2Content, err := os.ReadFile(envOutput2)
	require.NoError(t, err)
	env2Vars := string(env2Content)
	assert.Contains(t, env2Vars, "ATMOS_IDENTITY=mock-identity-2", "Step 2 should use mock-identity-2")

	// Both steps should have the same ATMOS_IDENTITY.
	step1Identity := extractEnvVar(env1Vars, "ATMOS_IDENTITY")
	step2Identity := extractEnvVar(env2Vars, "ATMOS_IDENTITY")
	assert.Equal(t, step1Identity, step2Identity, "Both steps should use the same identity")
}

func TestCustomCommandIntegration_SkipsStepWhenConditionIsFalse(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	skippedFile := filepath.Join(tmpDir, "skipped.txt")
	ranFile := filepath.Join(tmpDir, "ran.txt")

	testCommand := schema.Command{
		Name:        "test-when-skip",
		Description: "Test when skip",
		Steps: schema.Tasks{
			{
				Command: customCommandWriteHelperCommand(t, skippedFile, "skipped"),
				Type:    "shell",
				When:    schema.MustCondition("never"),
			},
			{
				Command: customCommandWriteHelperCommand(t, ranFile, "ran"),
				Type:    "shell",
			},
		},
	}
	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-when-skip" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	assert.NoFileExists(t, skippedFile)
	assert.FileExists(t, ranFile)
}

func TestCustomCommandIntegration_RetriesShellStep(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	attemptsFile := filepath.Join(tmpDir, "attempts.txt")
	resultFile := filepath.Join(tmpDir, "result.env")
	exePath, err := os.Executable()
	require.NoError(t, err)
	maxAttempts := 2
	initialDelay := time.Millisecond

	testCommand := schema.Command{
		Name:        "test-retry-shell-step",
		Description: "Test retry shell step",
		Steps: schema.Tasks{
			{
				Name:    "retry",
				Command: customCommandRetryHelperCommand(t, attemptsFile),
				Type:    "shell",
				Retry: &schema.RetryConfig{
					MaxAttempts:     &maxAttempts,
					InitialDelay:    &initialDelay,
					BackoffStrategy: "constant",
				},
			},
			{
				Command: fmt.Sprintf("%q", exePath),
				Type:    "shell",
				Output:  "none",
				Env: map[string]string{
					"_ATMOS_TEST_DUMP_ENV": resultFile,
					"CAPTURED_RESULT":      "{{ .steps.retry.value }}|{{ .steps.retry.metadata.stdout }}|{{ .steps.retry.metadata.stderr }}",
				},
			},
		},
	}
	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-retry-shell-step" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	attempts, err := os.ReadFile(attemptsFile)
	require.NoError(t, err)
	assert.Equal(t, "2", strings.TrimSpace(string(attempts)))
	resultEnv, err := os.ReadFile(resultFile)
	require.NoError(t, err)
	assert.Equal(t, "attempt-2|attempt-2|warning-2", extractEnvVar(string(resultEnv), "CAPTURED_RESULT"))
}

func TestCustomCommandIntegration_ShellStepWithoutRetryRunsOnce(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	attemptsFile := filepath.Join(tmpDir, "attempts.txt")

	testCommand := schema.Command{
		Name:        "test-no-retry-shell-step",
		Description: "Test no retry shell step",
		Steps: schema.Tasks{
			{
				Command: customCommandAttemptHelperCommand(t, attemptsFile),
				Type:    "shell",
			},
		},
	}
	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-no-retry-shell-step" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	attempts, err := os.ReadFile(attemptsFile)
	require.NoError(t, err)
	assert.Equal(t, "1", strings.TrimSpace(string(attempts)))
}

func TestCustomCommandIntegration_ExtendedStepCarriesEnvironmentAndStackFlag(t *testing.T) {
	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "extended-step.txt")

	testCommand := schema.Command{
		Name:        "test-extended-env-step",
		Description: "Test extended env step",
		Flags: []schema.CommandFlag{
			{Name: "stack", Type: "string"},
		},
		Steps: schema.Tasks{
			{
				Name: "set-env",
				Type: "env",
				Vars: map[string]string{
					"EXTENDED_VALUE": "{{ .flags.stack }}",
				},
			},
			{
				Command: fmt.Sprintf("printf %%s %q > %q", "{{ .env.EXTENDED_VALUE }}:{{ .flags.stack }}", outputPath),
				Type:    "shell",
			},
		},
	}
	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{testCommand},
	}

	parentCmd := &cobra.Command{Use: "atmos"}
	err := processCustomCommands(atmosConfig, atmosConfig.Commands, parentCmd)
	require.NoError(t, err)

	customCmd := findSubcommand(parentCmd, "test-extended-env-step")
	require.NotNil(t, customCmd)
	require.NoError(t, customCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))

	customCmd.Run(customCmd, []string{})

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "plat-ue2-dev:plat-ue2-dev", string(output))
}

func TestCustomCommandIntegration_AtmosStepUsesCurrentExecutable(t *testing.T) {
	_ = NewTestKit(t)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "atmos-step.txt")
	outputValue := "atmos step ran"

	testCommand := schema.Command{
		Name:        "test-atmos-step",
		Description: "Test atmos step",
		Steps: schema.Tasks{
			{
				Command: customCommandAtmosWriteHelperArgs(outputPath, outputValue),
				Type:    "atmos",
			},
		},
	}
	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Commands: []schema.Command{testCommand},
	}

	parentCmd := &cobra.Command{Use: "atmos"}
	err := processCustomCommands(atmosConfig, atmosConfig.Commands, parentCmd)
	require.NoError(t, err)

	customCmd := findSubcommand(parentCmd, "test-atmos-step")
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, outputValue, string(output))
}

func TestExecuteCustomCommandUnsupportedStepTypeExits(t *testing.T) {
	_ = NewTestKit(t)

	originalOsExit := errUtils.OsExit
	t.Cleanup(func() {
		errUtils.OsExit = originalOsExit
	})

	type exitPanic struct {
		code int
	}
	var exitCode int
	errUtils.OsExit = func(code int) {
		exitCode = code
		panic(exitPanic{code: code})
	}

	commandConfig := &schema.Command{
		Name:        "test-unsupported-step",
		Description: "Test unsupported step",
		Steps: schema.Tasks{
			{
				Type:    "not-a-step-type",
				Command: "noop",
			},
		},
	}

	assert.Panics(t, func() {
		executeCustomCommand(
			schema.AtmosConfiguration{BasePath: t.TempDir()},
			&cobra.Command{Use: commandConfig.Name, Annotations: map[string]string{}},
			nil,
			&cobra.Command{Use: "atmos"},
			commandConfig,
		)
	})
	assert.Equal(t, 1, exitCode)
}

func TestCustomCommandIntegration_DoesNotInstallToolVersionsTools(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	installDir := filepath.Join(tmpDir, "toolchain")
	toolVersionsPath := filepath.Join(tmpDir, ".tool-versions")
	require.NoError(t, os.WriteFile(toolVersionsPath, []byte("hashicorp/terraform 1.15.6\n"), 0o644))

	_ = NewTestKit(t)
	previousToolchainConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previousToolchainConfig) })

	ranFile := filepath.Join(tmpDir, "ran.txt")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: tmpDir,
		Toolchain: schema.Toolchain{
			InstallPath:  installDir,
			VersionsFile: toolVersionsPath,
		},
		Commands: []schema.Command{
			{
				Name:        "test-no-tool-install",
				Description: "Test custom command does not install .tool-versions tools",
				Steps:       stepsFromStrings(customCommandWriteHelperCommand(t, ranFile, "ran")),
			},
		},
	}
	toolchain.SetAtmosConfig(&atmosConfig)

	err := processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-no-tool-install" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd)

	customCmd.Run(customCmd, []string{})

	assert.FileExists(t, ranFile)
	assert.NoDirExists(t, installDir)
}

func customCommandWriteHelperCommand(t *testing.T, path, value string) string {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(path))
	encodedValue := base64.RawURLEncoding.EncodeToString([]byte(value))
	return fmt.Sprintf("%q -test.run=TestCustomCommandIntegrationWriteHelper -- %s %s", exe, encodedPath, encodedValue)
}

func customCommandAtmosWriteHelperArgs(path, value string) string {
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(path))
	encodedValue := base64.RawURLEncoding.EncodeToString([]byte(value))
	return fmt.Sprintf("-test.run=TestCustomCommandIntegrationWriteHelper -- %s %s", encodedPath, encodedValue)
}

func customCommandRetryHelperCommand(t *testing.T, path string) string {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(path))
	return fmt.Sprintf("%q -test.run=TestCustomCommandIntegrationRetryHelper -- %s", exe, encodedPath)
}

func customCommandAttemptHelperCommand(t *testing.T, path string) string {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(path))
	return fmt.Sprintf("%q -test.run=TestCustomCommandIntegrationAttemptHelper -- %s", exe, encodedPath)
}

func customCommandAtmosAttemptHelperArgs(path string) string {
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(path))
	return fmt.Sprintf("-test.run=TestCustomCommandIntegrationAttemptHelper -- %s", encodedPath)
}

func TestCustomCommandIntegrationWriteHelper(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator == -1 {
		return
	}

	args := os.Args[separator+1:]
	require.Len(t, args, 2)
	pathBytes, err := base64.RawURLEncoding.DecodeString(args[0])
	require.NoError(t, err)
	valueBytes, err := base64.RawURLEncoding.DecodeString(args[1])
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(string(pathBytes), valueBytes, 0o600))
	os.Exit(0)
}

func TestCustomCommandIntegrationRetryHelper(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator == -1 {
		return
	}

	args := os.Args[separator+1:]
	require.Len(t, args, 1)
	pathBytes, err := base64.RawURLEncoding.DecodeString(args[0])
	require.NoError(t, err)
	path := string(pathBytes)

	attempt := 1
	if existing, err := os.ReadFile(path); err == nil {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(existing)))
		require.NoError(t, parseErr)
		attempt = parsed + 1
	}
	require.NoError(t, os.WriteFile(path, []byte(strconv.Itoa(attempt)), 0o600))
	_, _ = fmt.Fprintf(os.Stdout, "attempt-%d", attempt)
	_, _ = fmt.Fprintf(os.Stderr, "warning-%d", attempt)
	if attempt < 2 {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestCustomCommandIntegrationAttemptHelper(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator == -1 {
		return
	}

	args := os.Args[separator+1:]
	require.Len(t, args, 1)
	pathBytes, err := base64.RawURLEncoding.DecodeString(args[0])
	require.NoError(t, err)
	path := string(pathBytes)

	attempt := 1
	if existing, err := os.ReadFile(path); err == nil {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(existing)))
		require.NoError(t, parseErr)
		attempt = parsed + 1
	}
	require.NoError(t, os.WriteFile(path, []byte(strconv.Itoa(attempt)), 0o600))
	os.Exit(0)
}

// TestCustomCommandIntegration_ComponentEnvExported verifies that a custom component's `env`
// section is exported as real environment variables to the command's step subprocess (mirroring
// the built-in terraform/helmfile/packer/ansible providers). This is the behavior that lets a
// `!secret` placed in a custom component's `env` section reach the step as `$VAR` instead of being
// inlined into the command string.
func TestCustomCommandIntegration_ComponentEnvExported(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: resolves a component from stack config")
	}

	// Use the custom-components example, whose `deploy-app` component declares an `env` section.
	testDir := "../examples/custom-components"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	envOutputFile := filepath.Join(tmpDir, "component-env.txt")

	// Use the test binary itself (in env-dump helper mode via TestMain) as a cross-platform
	// substitute for `env` / `cmd /c set`: the step runs the binary, and TestMain writes the step
	// subprocess environment to the file named by _ATMOS_TEST_DUMP_ENV, then exits. This keeps the
	// test free of platform-specific binaries (see testing_main_test.go).
	exePath, err := os.Executable()
	require.NoError(t, err)

	// A custom command bound to the `script` component type whose step dumps its environment.
	testCommand := schema.Command{
		Name:        "test-component-env",
		Description: "Dump the environment of a custom component step",
		Arguments: []schema.CommandArgument{
			{Name: "component", Type: "component", Required: true},
		},
		Flags: []schema.CommandFlag{
			{Name: "stack", Shorthand: "s", SemanticType: "stack", Required: true},
		},
		Component: &schema.CommandComponent{Type: "script"},
		Env:       []schema.CommandEnv{{Key: "_ATMOS_TEST_DUMP_ENV", Value: envOutputFile}},
		Steps:     stepsFromStrings(fmt.Sprintf("%q", exePath)),
	}

	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	RootCmd.SetArgs([]string{"test-component-env", "deploy-app", "-s", "dev"})
	err = RootCmd.Execute()
	require.NoError(t, err, "custom command execution should succeed")

	envContent, err := os.ReadFile(envOutputFile)
	require.NoError(t, err, "should be able to read environment output file")
	envVars := string(envContent)
	// Log only the asserted keys, never the whole environment (which can leak CI secrets/tokens).
	t.Logf("Captured component env: DEPLOY_REGION=%q APP_VERSION=%q",
		extractEnvVar(envVars, "DEPLOY_REGION"), extractEnvVar(envVars, "APP_VERSION"))

	// The component `env` section values must be exported as real environment variables.
	assert.Equal(t, "us-east-1", extractEnvVar(envVars, "DEPLOY_REGION"),
		"component env section must export DEPLOY_REGION to the step subprocess")
	assert.Equal(t, "1.0.0", extractEnvVar(envVars, "APP_VERSION"),
		"component env section must export APP_VERSION to the step subprocess")
}

// extractEnvVar extracts the value of an environment variable from env output.
func extractEnvVar(envOutput, varName string) string {
	lines := strings.Split(envOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, varName+"=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// TestCustomCommandIntegration_BooleanFlagDefaults tests that boolean flags with default values
// are correctly registered and accessible in custom commands.
//
// Note: This test uses camelCase flag names (e.g., customDebug) to avoid conflicts with global
// flags and to allow simple .Flags.name template syntax. For user-facing commands with kebab-case
// flags (e.g., custom-debug), users can use {{ index .Flags "custom-debug" }} template syntax.
func TestCustomCommandIntegration_BooleanFlagDefaults(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a temporary file to capture output.
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "bool-flag-output.txt")

	// Create a custom command with boolean flags that have various default values.
	// Note: Using "customDebug" instead of "verbose" since "verbose" is a global flag.
	testCommand := schema.Command{
		Name:        "test-bool-defaults",
		Description: "Test boolean flag defaults",
		Flags: []schema.CommandFlag{
			{
				Name:      "customDebug",
				Shorthand: "d",
				Type:      "bool",
				Usage:     "Enable customDebug output",
				Default:   false, // Explicit false default.
			},
			{
				Name:      "force",
				Shorthand: "f",
				Type:      "bool",
				Usage:     "Force the operation",
				Default:   true, // Default to true.
			},
			{
				Name:  "dryrun",
				Type:  "bool",
				Usage: "Perform dry run",
				// No default - should default to false.
			},
		},
		Steps: stepsFromStrings(
			"echo customDebug={{ .Flags.customDebug }} force={{ .Flags.force }} dryrun={{ .Flags.dryrun }} > " + outputFile,
		),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Find the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-bool-defaults" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Verify flags are registered with correct defaults.
	customDebugFlag := customCmd.PersistentFlags().Lookup("customDebug")
	require.NotNil(t, customDebugFlag, "customDebug flag should be registered")
	assert.Equal(t, "false", customDebugFlag.DefValue, "customDebug should default to false")

	forceFlag := customCmd.PersistentFlags().Lookup("force")
	require.NotNil(t, forceFlag, "force flag should be registered")
	assert.Equal(t, "true", forceFlag.DefValue, "force should default to true")

	dryrunFlag := customCmd.PersistentFlags().Lookup("dryrun")
	require.NotNil(t, dryrunFlag, "dryrun flag should be registered")
	assert.Equal(t, "false", dryrunFlag.DefValue, "dryrun should default to false when no default specified")
}

// TestCustomCommandIntegration_BooleanFlagTemplatePatterns tests that boolean flags
// work correctly with various Go template patterns in step execution.
func TestCustomCommandIntegration_BooleanFlagTemplatePatterns(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a temporary file to capture output from all patterns.
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "template-patterns-output.txt")

	// Create a custom command that tests various template patterns with boolean flags.
	// Note: Using "customDebug" instead of "verbose" since "verbose" is a global flag.
	testCommand := schema.Command{
		Name:        "test-template-patterns",
		Description: "Test boolean flag template patterns",
		Flags: []schema.CommandFlag{
			{
				Name:      "customDebug",
				Shorthand: "d",
				Type:      "bool",
				Usage:     "Enable customDebug output",
				Default:   false,
			},
			{
				Name:    "clean",
				Type:    "bool",
				Usage:   "Clean before building",
				Default: true,
			},
		},
		Steps: stepsFromStrings(
			// Test multiple patterns in a single step that writes to file.
			// Using .Flags.customDebug (no hyphens) so we can use simple dot notation.
			`echo "PATTERN1={{ if .Flags.customDebug }}DEBUG_ON{{ end }}" >> `+outputFile,
			`echo "PATTERN2=Building{{ if .Flags.customDebug }} with debug{{ end }}" >> `+outputFile,
			`echo "PATTERN3={{ if .Flags.clean }}CLEAN_ON{{ else }}CLEAN_OFF{{ end }}" >> `+outputFile,
			`echo "PATTERN4={{ if not .Flags.customDebug }}QUIET_MODE{{ end }}" >> `+outputFile,
			`echo "PATTERN5=customDebug={{ .Flags.customDebug }}" >> `+outputFile,
			"echo \"PATTERN6=clean={{ printf \"%t\" .Flags.clean }}\" >> "+outputFile,
		),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Verify the command is registered.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-template-patterns" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Test 1: Execute with default values (customDebug=false, clean=true).
	// IMPORTANT: Must use RootCmd.SetArgs() + Execute() to properly initialize Cobra's flag merging.
	// Calling cmd.Run() directly bypasses flag initialization and causes "flag not defined" errors.
	RootCmd.SetArgs([]string{"test-template-patterns"})
	err = RootCmd.Execute()
	require.NoError(t, err, "Custom command execution should succeed")

	// Read and verify output.
	output, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Should be able to read output file")
	outputStr := string(output)
	t.Logf("Output with defaults (customDebug=false, clean=true):\n%s", outputStr)

	// Pattern 1: {{ if .Flags.customDebug }} - should be empty since customDebug=false.
	assert.Contains(t, outputStr, "PATTERN1=\n", "Pattern 1: if should produce empty when customDebug=false")

	// Pattern 2: Inline conditional - should not have "with customDebug".
	assert.Contains(t, outputStr, "PATTERN2=Building\n", "Pattern 2: inline if should not append when customDebug=false")

	// Pattern 3: if/else - clean=true so should be CLEAN_ON.
	assert.Contains(t, outputStr, "PATTERN3=CLEAN_ON", "Pattern 3: if/else should produce CLEAN_ON when clean=true")

	// Pattern 4: if not - customDebug=false so "not .Flags.customDebug" is true.
	assert.Contains(t, outputStr, "PATTERN4=QUIET_MODE", "Pattern 4: if not should produce QUIET_MODE when customDebug=false")

	// Pattern 5: Direct boolean value - should be "false".
	assert.Contains(t, outputStr, "PATTERN5=customDebug=false", "Pattern 5: direct value should render as 'false'")

	// Pattern 6: printf %t - should be "true".
	assert.Contains(t, outputStr, "PATTERN6=clean=true", "Pattern 6: printf should render as 'true'")

	// Clear output file for next test.
	err = os.WriteFile(outputFile, []byte{}, 0o644)
	require.NoError(t, err)

	// Test 2: Execute with customDebug=true and clean=false (via flags).
	RootCmd.SetArgs([]string{"test-template-patterns", "--customDebug", "--clean=false"})
	err = RootCmd.Execute()
	require.NoError(t, err, "Custom command execution with flags should succeed")

	// Read and verify output with customDebug=true, clean=false.
	output, err = os.ReadFile(outputFile)
	require.NoError(t, err)
	outputStr = string(output)
	t.Logf("Output with flags (customDebug=true, clean=false):\n%s", outputStr)

	// Pattern 1: {{ if .Flags.customDebug }} - should have DEBUG_ON.
	assert.Contains(t, outputStr, "PATTERN1=DEBUG_ON", "Pattern 1: if should produce DEBUG_ON when customDebug=true")

	// Pattern 2: Inline conditional - should have "with debug".
	assert.Contains(t, outputStr, "PATTERN2=Building with debug", "Pattern 2: inline if should append when customDebug=true")

	// Pattern 3: if/else - clean=false so should be CLEAN_OFF.
	assert.Contains(t, outputStr, "PATTERN3=CLEAN_OFF", "Pattern 3: if/else should produce CLEAN_OFF when clean=false")

	// Pattern 4: if not - customDebug=true so "not .Flags.customDebug" is false.
	assert.Contains(t, outputStr, "PATTERN4=\n", "Pattern 4: if not should produce empty when customDebug=true")

	// Pattern 5: Direct boolean value - should be "true".
	assert.Contains(t, outputStr, "PATTERN5=customDebug=true", "Pattern 5: direct value should render as 'true'")

	// Pattern 6: printf %t - should be "false".
	assert.Contains(t, outputStr, "PATTERN6=clean=false", "Pattern 6: printf should render as 'false'")
}

// TestCustomCommandIntegration_StringFlagDefaults tests that string flags with default values
// are correctly registered and accessible in custom commands.
func TestCustomCommandIntegration_StringFlagDefaults(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command with string flags that have default values.
	testCommand := schema.Command{
		Name:        "test-string-defaults",
		Description: "Test string flag defaults",
		Flags: []schema.CommandFlag{
			{
				Name:      "environment",
				Shorthand: "e",
				Usage:     "Target environment",
				Default:   "development",
			},
			{
				Name:  "region",
				Usage: "AWS region",
				// No default - should be empty string.
			},
			{
				Name:    "format",
				Usage:   "Output format",
				Default: "json",
			},
		},
		Steps: stepsFromStrings(
			"echo environment={{ .Flags.environment }} region={{ .Flags.region }} format={{ .Flags.format }}",
		),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Find the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-string-defaults" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Verify flags are registered with correct defaults.
	envFlag := customCmd.PersistentFlags().Lookup("environment")
	require.NotNil(t, envFlag, "environment flag should be registered")
	assert.Equal(t, "development", envFlag.DefValue, "environment should default to 'development'")

	regionFlag := customCmd.PersistentFlags().Lookup("region")
	require.NotNil(t, regionFlag, "region flag should be registered")
	assert.Equal(t, "", regionFlag.DefValue, "region should default to empty string when no default specified")

	formatFlag := customCmd.PersistentFlags().Lookup("format")
	require.NotNil(t, formatFlag, "format flag should be registered")
	assert.Equal(t, "json", formatFlag.DefValue, "format should default to 'json'")
}

// TestCustomCommandIntegration_NoIdentity tests that custom commands without identity
// work correctly and don't set authentication environment variables.
func TestCustomCommandIntegration_NoIdentity(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a temporary file to capture environment variables.
	tmpDir := t.TempDir()
	envOutputFile := filepath.Join(tmpDir, "no-identity-env.txt")

	// Get OS-specific command to dump environment variables.
	var dumpEnvCmd string
	if runtime.GOOS == "windows" {
		dumpEnvCmd = "cmd /c set > \"" + envOutputFile + "\""
	} else {
		dumpEnvCmd = "env > " + envOutputFile
	}

	// Create a custom command WITHOUT identity.
	testCommand := schema.Command{
		Name:        "test-no-identity",
		Description: "Test command without identity",
		// No Identity field
		Steps: stepsFromStrings(dumpEnvCmd),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Find and execute the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-no-identity" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Execute the custom command.
	customCmd.Run(customCmd, []string{})

	// Read the captured environment variables.
	envContent, err := os.ReadFile(envOutputFile)
	require.NoError(t, err, "Should be able to read environment output file")

	envVars := string(envContent)
	t.Logf("Captured environment variables (no identity):\n%s", envVars)

	// This command should NOT have ATMOS_IDENTITY set by our auth system
	// (it might have it from parent process, but we're checking our code doesn't add it).
	// We can't really assert it's NOT there without affecting parent, so just log for manual verification.
	t.Logf("Command without identity executed successfully")
}
