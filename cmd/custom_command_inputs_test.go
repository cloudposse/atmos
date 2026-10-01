package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommandIntegration_InputsSkipsWhenSourcesUnchanged exercises the full `inputs:`/
// `artifacts:` feature end-to-end: a step declares inputs.sources/artifacts.paths with no
// explicit `when:`, so it implicitly means `when: checksum.changed`. The first run has no
// recorded state and must execute; the second run (sources unchanged) must skip; touching the
// source file must cause a third run to execute again.
func TestCustomCommandIntegration_InputsSkipsWhenSourcesUnchanged(t *testing.T) {
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
	// Freshness state persists under {BasePath}/.atmos/cache/freshness -- route it into tmpDir
	// so this test doesn't pollute (or get polluted by) the fixture's real base path.
	atmosConfig.BasePath = tmpDir

	srcFile := filepath.Join(tmpDir, "main.go")
	artifactFile := filepath.Join(tmpDir, "bin", "app")
	runLog := filepath.Join(tmpDir, "run.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactFile), 0o755))
	require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))

	atmosConfig.Commands = []schema.Command{
		{
			Name:             "ii-build",
			WorkingDirectory: tmpDir,
			Steps: schema.Tasks{
				{
					Type:      "shell",
					Command:   customCommandAppendHelperCommand(t, runLog, "ran"),
					Inputs:    &schema.Inputs{Sources: []string{"main.go"}},
					Artifacts: &schema.Artifacts{Paths: []string{"bin/app"}},
				},
			},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var buildCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "ii-build" {
			buildCmd = c
			break
		}
	}
	require.NotNil(t, buildCmd, "ii-build command should be registered")

	countRuns := func() int {
		content, readErr := os.ReadFile(runLog)
		if os.IsNotExist(readErr) {
			return 0
		}
		require.NoError(t, readErr)
		return len(splitNonEmptyLines(string(content)))
	}

	// First run: no recorded state -> must execute.
	buildCmd.Run(buildCmd, []string{})
	assert.Equal(t, 1, countRuns(), "first run must execute (no prior recorded state)")

	// Second run: sources unchanged -> must skip.
	buildCmd.Run(buildCmd, []string{})
	assert.Equal(t, 1, countRuns(), "second run must skip since sources are unchanged")

	// Touch the source file's content -> must execute again.
	require.NoError(t, os.WriteFile(srcFile, []byte("package main // changed"), 0o644))
	buildCmd.Run(buildCmd, []string{})
	assert.Equal(t, 2, countRuns(), "third run must execute since sources changed")
}

// TestCustomCommandIntegration_ExplicitFreshnessWhenRunsOnFreshState verifies steps with an
// EXPLICIT freshness-referencing `when:` (overriding the implicit checksum.changed default) run
// when the source is newer than the artifact, for both the timestamp.changed convenience fact and
// the structured sources/artifacts comparison form. Both are stateless -- pure current-mtime
// comparison, no "first run" concept -- so this test sets explicit mtimes (source newer) rather
// than relying on "no prior state". Before the fix, the cheap hasRunnableStep pre-check in
// executeCustomCommand evaluated every step's raw `when:` against a Context with no freshness
// facts populated (TimestampChanged/Sources/Artifacts always zero-value), so an explicit
// freshness-referencing `when:` always evaluated false there -- regardless of actual file
// mtimes -- and the whole command was skipped before the real, correctly-wired per-step loop
// (which DOES pass real freshness facts) ever ran.
func TestCustomCommandIntegration_ExplicitFreshnessWhenRunsOnFreshState(t *testing.T) {
	cases := []struct {
		name    string
		cmdName string
		when    string
	}{
		{name: "timestamp.changed", cmdName: "ii-build-ts", when: "timestamp.changed"},
		{name: "sources/artifacts comparison", cmdName: "ii-build-cmp", when: "sources.exists(s, artifacts.all(a, s.mtime > a.mtime))"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
			atmosConfig.BasePath = tmpDir

			srcFile := filepath.Join(tmpDir, "main.go")
			artifactFile := filepath.Join(tmpDir, "bin", "app")
			runLog := filepath.Join(tmpDir, "run.txt")
			require.NoError(t, os.WriteFile(srcFile, []byte("package main"), 0o644))
			require.NoError(t, os.MkdirAll(filepath.Dir(artifactFile), 0o755))
			require.NoError(t, os.WriteFile(artifactFile, []byte("binary"), 0o644))
			// Explicit, deterministic mtimes (rather than relying on wall-clock write ordering,
			// which can collide on filesystems with coarse mtime resolution) so the source is
			// unambiguously newer than the artifact -- what both `when:` forms must observe true.
			older := time.Now().Add(-time.Hour)
			newer := time.Now()
			require.NoError(t, os.Chtimes(artifactFile, older, older))
			require.NoError(t, os.Chtimes(srcFile, newer, newer))

			atmosConfig.Commands = []schema.Command{
				{
					Name:             tc.cmdName,
					WorkingDirectory: tmpDir,
					Steps: schema.Tasks{
						{
							Type:      "shell",
							Command:   customCommandWriteHelperCommand(t, runLog, "ran"),
							Inputs:    &schema.Inputs{Sources: []string{"main.go"}},
							Artifacts: &schema.Artifacts{Paths: []string{"bin/app"}},
							When:      schema.MustCondition(tc.when),
						},
					},
				},
			}

			err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
			require.NoError(t, err)

			var buildCmd *cobra.Command
			for _, c := range RootCmd.Commands() {
				if c.Name() == tc.cmdName {
					buildCmd = c
					break
				}
			}
			require.NotNil(t, buildCmd, "%s command should be registered", tc.cmdName)

			buildCmd.Run(buildCmd, []string{})

			assert.FileExists(t, runLog, "step with explicit when: %q must run on a fresh checkout", tc.when)
		})
	}
}

// TestCustomCommandIntegration_PreconditionsSkipsWhenToolAlreadyOnPath verifies the independent
// `preconditions:` mechanism: a step with `preconditions.tools` and no explicit inputs/artifacts
// skips when every declared tool already resolves via exec.LookPath. Uses "go" as the tool name
// (guaranteed present in this repo's own test environment, since these tests run under `go
// test`) rather than a shell command like `which`/`true`, both to exercise the real
// exec.LookPath-based mechanism end-to-end and to stay cross-platform (no Unix-only binaries).
func TestCustomCommandIntegration_PreconditionsSkipsWhenToolAlreadyOnPath(t *testing.T) {
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
	atmosConfig.BasePath = tmpDir
	runLog := filepath.Join(tmpDir, "run.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name:             "ii-install",
			WorkingDirectory: tmpDir,
			Steps: schema.Tasks{
				{
					Type:          "shell",
					Command:       customCommandWriteHelperCommand(t, runLog, "ran"),
					Preconditions: &schema.Preconditions{Tools: []string{"go"}},
					// Explicit here for clarity; this is also the implicit default for a
					// preconditions-only step with no `when:` at all.
					When: schema.MustCondition("!cel !preconditions.success"),
				},
			},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var installCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "ii-install" {
			installCmd = c
			break
		}
	}
	require.NotNil(t, installCmd, "ii-install command should be registered")

	installCmd.Run(installCmd, []string{})

	assert.NoFileExists(t, runLog, "step must be skipped when the preconditions tool is already on PATH")
}

// TestCustomCommandIntegration_PreconditionsRunsWhenToolMissing is the negative-path counterpart:
// a tool that is definitely not on PATH must leave the preconditions unmet, so the step runs.
func TestCustomCommandIntegration_PreconditionsRunsWhenToolMissing(t *testing.T) {
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
	atmosConfig.BasePath = tmpDir
	runLog := filepath.Join(tmpDir, "run.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name:             "ii-install-missing",
			WorkingDirectory: tmpDir,
			Steps: schema.Tasks{
				{
					Type:          "shell",
					Command:       customCommandWriteHelperCommand(t, runLog, "ran"),
					Preconditions: &schema.Preconditions{Tools: []string{"atmos-definitely-does-not-exist-tool"}},
				},
			},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var installCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "ii-install-missing" {
			installCmd = c
			break
		}
	}
	require.NotNil(t, installCmd, "ii-install-missing command should be registered")

	installCmd.Run(installCmd, []string{})

	assert.FileExists(t, runLog, "step must run when the preconditions tool is not on PATH")
}
