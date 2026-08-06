package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommandIntegration_DependenciesCommandsDiamondDedup exercises the diamond-shaped
// dependency case: release depends on [test, lint], both of which depend on build, which must
// execute exactly once for the whole `atmos release` invocation, even though two different
// commands declare a dependency on it.
//
// Command names are prefixed per-test (dd-*) since RootCmd accumulates custom-command
// registrations across tests within one test binary run -- only flags/args are reset by
// NewTestKit -- so reusing a generic name like "build" across two test functions in this
// package would resolve to whichever test registered it first.
func TestCustomCommandIntegration_DependenciesCommandsDiamondDedup(t *testing.T) {
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
	buildLog := filepath.Join(tmpDir, "build.txt")
	releaseLog := filepath.Join(tmpDir, "release.txt")

	// filepath.ToSlash: see the comment on the parallel-needs test in
	// custom_command_control_test.go — an unquoted Windows backslash path reaching mvdan/sh
	// would have its backslashes consumed as shell escapes.
	buildLogArg := filepath.ToSlash(buildLog)
	releaseLogArg := filepath.ToSlash(releaseLog)
	atmosConfig.Commands = []schema.Command{
		{
			Name:  "dd-build",
			Steps: schema.Tasks{{Type: "shell", Command: "echo build >> " + buildLogArg}},
		},
		{
			Name:         "dd-test",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{{Name: "dd-build"}}},
			Steps:        schema.Tasks{{Type: "shell", Command: "echo test >> " + buildLogArg}},
		},
		{
			Name:         "dd-lint",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{{Name: "dd-build"}}},
			Steps:        schema.Tasks{{Type: "shell", Command: "echo lint >> " + buildLogArg}},
		},
		{
			Name:         "dd-release",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{{Name: "dd-test"}, {Name: "dd-lint"}}},
			Steps:        schema.Tasks{{Type: "shell", Command: "echo release >> " + releaseLogArg}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var releaseCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "dd-release" {
			releaseCmd = c
			break
		}
	}
	require.NotNil(t, releaseCmd, "dd-release command should be registered")

	releaseCmd.Run(releaseCmd, []string{})

	content, err := os.ReadFile(buildLog)
	require.NoError(t, err)
	lines := splitNonEmptyLines(string(content))
	buildCount := 0
	for _, line := range lines {
		if line == "build" {
			buildCount++
		}
	}
	assert.Equal(t, 1, buildCount, "build must execute exactly once despite two dependents (test and lint)")
	assert.Contains(t, lines, "test")
	assert.Contains(t, lines, "lint")
	assert.FileExists(t, releaseLog, "release's own step must still run after its dependencies complete")
}

// TestCustomCommandIntegration_DependenciesCommandsParameterizedBothRun verifies that several
// dependency entries referencing the same command with DIFFERENT flag overrides are distinct
// graph nodes, run CONCURRENTLY (taskgraph's default), and each observes its OWN flag value --
// not a value raced-and-overwritten by a sibling dispatch. Before the fix, CustomCommandRunner
// resolved every differently-flagged Ref to the SAME already-registered *cobra.Command and
// mutated its shared PersistentFlags with no synchronization; a dispatch could read whatever a
// concurrently-racing sibling had just Set(). Six variants (not two) deliberately maximizes the
// number of concurrent pairwise races within a single run -- go test -race's own instrumentation
// overhead widens each race's window further, but isn't required for this test to be meaningful.
func TestCustomCommandIntegration_DependenciesCommandsParameterizedBothRun(t *testing.T) {
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
	buildLog := filepath.Join(tmpDir, "build.txt")

	envValues := []string{"dev", "staging", "prod", "qa", "uat", "sandbox"}
	deps := make(schema.UnitDependencies, 0, len(envValues))
	want := make([]string, 0, len(envValues))
	for _, env := range envValues {
		deps = append(deps, schema.UnitDependency{Name: "pp-build", Flags: map[string]string{"env": env}})
		want = append(want, "env="+env)
	}

	atmosConfig.Commands = []schema.Command{
		{
			Name: "pp-build",
			Flags: []schema.CommandFlag{
				{Name: "env", Type: "string", Default: "dev"},
			},
			Steps: schema.Tasks{{Type: "shell", Command: "echo env={{ .Flags.env }} >> " + filepath.ToSlash(buildLog)}},
		},
		{
			Name:         "pp-release",
			Dependencies: &schema.Dependencies{Commands: deps},
			Steps:        schema.Tasks{{Type: "shell", Command: "true"}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var releaseCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "pp-release" {
			releaseCmd = c
			break
		}
	}
	require.NotNil(t, releaseCmd, "pp-release command should be registered")

	releaseCmd.Run(releaseCmd, []string{})

	content, err := os.ReadFile(buildLog)
	require.NoError(t, err)
	lines := splitNonEmptyLines(string(content))
	assert.ElementsMatch(t, want, lines,
		"differently-parameterized invocations of build must each run exactly once with its OWN flag value")
}

// TestCustomCommandIntegration_DependenciesCommandsMixedBareAndFlaggedBothCorrect verifies a bare
// (no flag override) dependency reference to a command, sharing a graph with a differently-
// flagged reference to the SAME command, observes the command's DECLARED DEFAULT -- not whatever
// value a sibling dispatch happened to Set() on the shared *cobra.Command. This is a distinct bug
// from the concurrency race TestCustomCommandIntegration_DependenciesCommandsParameterizedBothRun
// covers: even fully serialized (no race possible), a bare reference dispatched after a flagged
// one previously inherited the flagged dispatch's leftover value, because CustomCommandRunner
// only ever called Set() for flags present in a given Ref -- it never reset flags absent from
// that Ref back to their declared default before running. Deterministic, not timing-dependent:
// reproduced 100% of the time pre-fix, in either dispatch order.
func TestCustomCommandIntegration_DependenciesCommandsMixedBareAndFlaggedBothCorrect(t *testing.T) {
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
	buildLog := filepath.Join(tmpDir, "build.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name: "mb-compile",
			Flags: []schema.CommandFlag{
				{Name: "target", Type: "string", Default: "default"},
			},
			Steps: schema.Tasks{{Type: "shell", Command: "echo target={{ .Flags.target }} >> " + filepath.ToSlash(buildLog)}},
		},
		{
			Name: "mb-verify",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{
				{Name: "mb-compile"}, // bare: must observe the declared default, "default".
				{Name: "mb-compile", Flags: map[string]string{"target": "test"}},
			}},
			Steps: schema.Tasks{{Type: "shell", Command: "true"}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var verifyCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "mb-verify" {
			verifyCmd = c
			break
		}
	}
	require.NotNil(t, verifyCmd, "mb-verify command should be registered")

	verifyCmd.Run(verifyCmd, []string{})

	content, err := os.ReadFile(buildLog)
	require.NoError(t, err)
	lines := splitNonEmptyLines(string(content))
	assert.ElementsMatch(t, []string{"target=default", "target=test"}, lines,
		"the bare reference must observe the declared default, not a sibling dispatch's leftover flag value")
}

// TestCustomCommandIntegration_DependenciesFailBestEffortSwallowsFailure verifies `fail:
// best_effort` on a dependencies.commands entry actually swallows that dependency's failure: the
// parent command's own steps still run, and the process does not hard-exit. Before the fix,
// CustomCommandRunner invoked the dependency's cobra Run in-process, and executeCustomCommand
// unconditionally called errUtils.CheckErrorPrintAndExit (-> os.Exit) on any step failure --
// killing the whole process synchronously, before control ever returned to taskgraph.Run for its
// already-correct effectiveFailMode/best_effort handling to have any effect.
//
// The overridden errUtils.OsExit records into a mutex-guarded flag rather than panicking: a
// dependency's exit happens inside a pkg/scheduler WORKER GOROUTINE (see the stack trace this
// test originally caught pre-fix), not the test's own goroutine, and an unrecovered panic in any
// goroutine terminates the whole process regardless of a recover() set up elsewhere -- so
// assert.Panics-style catching is not reliable here.
func TestCustomCommandIntegration_DependenciesFailBestEffortSwallowsFailure(t *testing.T) {
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
	ownStepLog := filepath.Join(tmpDir, "own-step.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name:  "be-always-fails",
			Steps: schema.Tasks{{Type: "shell", Command: "exit 1"}},
		},
		{
			Name: "be-parent",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{
				{Name: "be-always-fails", Fail: "best_effort"},
			}},
			Steps: schema.Tasks{{Type: "shell", Command: "echo ran >> " + filepath.ToSlash(ownStepLog)}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var parentCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "be-parent" {
			parentCmd = c
			break
		}
	}
	require.NotNil(t, parentCmd, "be-parent command should be registered")

	var mu sync.Mutex
	exited := false
	originalOsExit := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = originalOsExit })
	errUtils.OsExit = func(code int) {
		mu.Lock()
		exited = true
		mu.Unlock()
	}

	parentCmd.Run(parentCmd, []string{})

	mu.Lock()
	defer mu.Unlock()
	assert.False(t, exited, "fail: best_effort must never reach errUtils.OsExit")
	assert.FileExists(t, ownStepLog, "parent's own step must still run after a best_effort dependency fails")
}

// TestCustomCommandIntegration_DependenciesFailFastStillPropagates is the negative-path
// counterpart: the default (wait_all, no fail: override) must still surface a dependency's
// failure -- the parent's own steps must NOT run, and errUtils.OsExit must still be reached
// (observed via the same mutex-guarded flag as the best_effort test above, not panic/recover: pre-
// fix this exit fires from inside a pkg/scheduler worker goroutine, where an unrecovered panic
// would crash the whole test binary rather than being catchable by this goroutine's recover()).
// Confirms the best_effort fix above didn't accidentally swallow failures it shouldn't.
func TestCustomCommandIntegration_DependenciesFailFastStillPropagates(t *testing.T) {
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
	ownStepLog := filepath.Join(tmpDir, "own-step.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name:  "ff-always-fails",
			Steps: schema.Tasks{{Type: "shell", Command: "exit 1"}},
		},
		{
			Name: "ff-parent",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{
				{Name: "ff-always-fails"}, // no fail: override -> default wait_all, must still fail.
			}},
			Steps: schema.Tasks{{Type: "shell", Command: "echo ran >> " + filepath.ToSlash(ownStepLog)}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var parentCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "ff-parent" {
			parentCmd = c
			break
		}
	}
	require.NotNil(t, parentCmd, "ff-parent command should be registered")

	var mu sync.Mutex
	exited := false
	originalOsExit := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = originalOsExit })
	errUtils.OsExit = func(code int) {
		mu.Lock()
		exited = true
		mu.Unlock()
	}

	parentCmd.Run(parentCmd, []string{})

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, exited, "the default (wait_all) fail mode must still surface a dependency's failure via errUtils.OsExit")
	assert.NoFileExists(t, ownStepLog, "parent's own step must not run when a required dependency fails")
}

// splitNonEmptyLines splits shell-command-captured output into trimmed, non-empty lines. Each
// line is trimmed of surrounding whitespace (not just the \n split character) because mvdan/sh's
// `echo`+`>>` redirect on Windows CI has been observed producing a trailing space and \r before
// the \n (e.g. "first \r\n") where the same command on Unix produces a bare "first\n" -- trimming
// keeps line-content assertions ("first", not "first \r") portable across platforms without
// asserting on that shell/OS-level formatting difference, which Atmos doesn't control.
func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
