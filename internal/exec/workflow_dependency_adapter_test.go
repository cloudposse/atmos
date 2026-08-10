package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/taskgraph"
)

// TestExecuteWorkflow_DependenciesWorkflowsSameFile verifies a workflow's dependencies.workflows
// entry with no `file:` resolves against the SAME file the running workflow itself came from,
// and runs before the dependent's own steps.
func TestExecuteWorkflow_DependenciesWorkflowsSameFile(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	atmosConfig.BasePath = tmpDir
	atmosConfig.Workflows.BasePath = ""

	buildLog := filepath.Join(tmpDir, "build.txt")
	deployLog := filepath.Join(tmpDir, "deploy.txt")

	// Forward slashes when embedding a path into this raw YAML manifest string: a Windows
	// backslash path (e.g. C:\Users\...) parsed through a YAML double-quoted scalar would be
	// misread as C-style escapes (\U, \u, \x expect hex digits, failing with "did not find
	// expected hexdecimal number"), and the same backslashes would also be consumed as shell
	// escapes once the resulting command string reaches the mvdan/sh interpreter. Forward
	// slashes are valid path separators on Windows too, so os.OpenFile-based redirects still
	// resolve correctly.
	manifest := `
workflows:
  build:
    steps:
      - command: "echo build >> ` + filepath.ToSlash(buildLog) + `"
        type: shell
  deploy:
    dependencies:
      workflows:
        - build
    steps:
      - command: "echo deploy >> ` + filepath.ToSlash(deployLog) + `"
        type: shell
`
	workflowPath := filepath.Join(tmpDir, "same-file.yaml")
	require.NoError(t, os.WriteFile(workflowPath, []byte(manifest), 0o644))

	workflowConfig, err := LoadWorkflowConfig(workflowPath)
	require.NoError(t, err)
	deployDef := workflowConfig["deploy"]

	err = ExecuteWorkflow(atmosConfig, "deploy", workflowPath, &deployDef, false, "", "", "")
	require.NoError(t, err)

	assert.FileExists(t, buildLog, "same-file dependency 'build' must run")
	assert.FileExists(t, deployLog, "deploy's own step must still run after its dependency completes")
}

// TestExecuteWorkflow_DependenciesWorkflowsCrossFile verifies a dependencies.workflows entry
// with an explicit `file:` resolves against that OTHER file, the same way the CLI's
// `-f`/`--file` flag resolves a workflow file.
func TestExecuteWorkflow_DependenciesWorkflowsCrossFile(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	atmosConfig.BasePath = tmpDir
	atmosConfig.Workflows.BasePath = ""
	// InitCliConfig already precomputed WorkflowsDirAbsolutePath against the original
	// stacksPath-derived config; getWorkflowsDirToUse prefers that precomputed field over the
	// live BasePath/Workflows.BasePath above, so it must be updated too or cross-file
	// resolution below still looks in the stale stacksPath directory instead of tmpDir.
	atmosConfig.WorkflowsDirAbsolutePath = tmpDir

	buildLog := filepath.Join(tmpDir, "build.txt")
	deployLog := filepath.Join(tmpDir, "deploy.txt")

	// See the same-file test's comment: forward slashes avoid both a YAML double-quoted-scalar
	// hex-escape misparse and mvdan/sh consuming backslashes as shell escapes on Windows.
	buildManifest := `
workflows:
  build:
    steps:
      - command: "echo build >> ` + filepath.ToSlash(buildLog) + `"
        type: shell
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "build.yaml"), []byte(buildManifest), 0o644))

	deployManifest := `
workflows:
  deploy:
    dependencies:
      workflows:
        - name: build
          file: build.yaml
    steps:
      - command: "echo deploy >> ` + filepath.ToSlash(deployLog) + `"
        type: shell
`
	deployPath := filepath.Join(tmpDir, "deploy.yaml")
	require.NoError(t, os.WriteFile(deployPath, []byte(deployManifest), 0o644))

	workflowConfig, err := LoadWorkflowConfig(deployPath)
	require.NoError(t, err)
	deployDef := workflowConfig["deploy"]

	err = ExecuteWorkflow(atmosConfig, "deploy", deployPath, &deployDef, false, "", "", "")
	require.NoError(t, err)

	assert.FileExists(t, buildLog, "cross-file dependency 'build' (resolved via file: build.yaml) must run")
	assert.FileExists(t, deployLog, "deploy's own step must still run after its cross-file dependency completes")
}

// TestExecuteWorkflow_DependenciesWorkflowsDiamondDedup verifies a diamond-shaped
// workflow-depends-on-workflow graph -- release depends on [test, lint], both of which depend on
// build -- executes build exactly once for the whole `release` invocation. Before the fix,
// WorkflowRunner dispatched a workflow dependency by calling the general-purpose ExecuteWorkflow
// entry point, which unconditionally re-resolved and re-ran ITS OWN dependencies.workflows again
// -- on top of the shared dependency already correctly deduped and run once by the parent's own
// taskgraph.Run graph. A shared dependency reached through two different parents would then run
// three times (once correctly, twice redundantly) instead of once.
func TestExecuteWorkflow_DependenciesWorkflowsDiamondDedup(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	atmosConfig.BasePath = tmpDir
	atmosConfig.Workflows.BasePath = ""

	buildLog := filepath.Join(tmpDir, "build.txt")
	releaseLog := filepath.Join(tmpDir, "release.txt")

	// See TestExecuteWorkflow_DependenciesWorkflowsSameFile's comment on forward slashes.
	manifest := `
workflows:
  build:
    steps:
      - command: "echo build >> ` + filepath.ToSlash(buildLog) + `"
        type: shell
  test:
    dependencies:
      workflows: [build]
    steps:
      - command: "echo test"
        type: shell
  lint:
    dependencies:
      workflows: [build]
    steps:
      - command: "echo lint"
        type: shell
  release:
    dependencies:
      workflows: [test, lint]
    steps:
      - command: "echo release >> ` + filepath.ToSlash(releaseLog) + `"
        type: shell
`
	workflowPath := filepath.Join(tmpDir, "diamond.yaml")
	require.NoError(t, os.WriteFile(workflowPath, []byte(manifest), 0o644))

	workflowConfig, err := LoadWorkflowConfig(workflowPath)
	require.NoError(t, err)
	releaseDef := workflowConfig["release"]

	err = ExecuteWorkflow(atmosConfig, "release", workflowPath, &releaseDef, false, "", "", "")
	require.NoError(t, err)

	content, err := os.ReadFile(buildLog)
	require.NoError(t, err)
	lines := splitNonEmptyTrimmedLines(string(content))
	assert.Len(t, lines, 1, "build must execute exactly once despite two dependents (test and lint), got: %v", lines)
	assert.FileExists(t, releaseLog, "release's own step must still run after its dependencies complete")
}

// splitNonEmptyTrimmedLines splits shell-command-captured output into trimmed, non-empty lines.
// Trimmed (not just \n-split) because mvdan/sh's echo+>> redirect on Windows CI has been observed
// producing a trailing space and \r before the \n, which bare \n-splitting would preserve as
// content differences irrelevant to what these tests assert.
func splitNonEmptyTrimmedLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// TestResolveAtmosBinary_UsesOwnExecutablePath verifies commandRunnerViaSubprocess resolves the
// currently-running binary's own absolute path (via os.Executable(), mirroring the established
// pkg/runner/step/atmos.go pattern for `type: atmos` steps) rather than the bare string "atmos",
// which exec.Command would resolve via a PATH lookup -- silently running whatever atmos version
// happens to be first on PATH instead of the actively-running one. A bare-name regression here is
// exactly what let a workflow's dependencies.commands subprocess dispatch run a stale/different
// atmos version in a dev environment where a released version also lives on PATH.
func TestResolveAtmosBinary_UsesOwnExecutablePath(t *testing.T) {
	resolved, err := resolveAtmosBinary()
	require.NoError(t, err)

	wantExecutable, err := os.Executable()
	require.NoError(t, err)

	assert.Equal(t, wantExecutable, resolved, "must resolve the currently-running binary's own path")
	assert.NotEqual(t, "atmos", resolved, "must never be the bare command name (PATH-resolved at exec time)")
	assert.True(t, filepath.IsAbs(resolved), "resolved binary path must be absolute")
}

// TestCommandLookup_AmbiguousNameErrors guards against dependencies.commands: [build] silently
// resolving to whichever of two unrelated "build" commands schema.FindCommandByName's tree-walk
// visits first when the config declares duplicate nested command names -- CommandLookup must
// surface a clear error instead, since graph-building (taskgraph.Run) fails on this before any
// dispatch happens, closing the gap for both the lookup and the actual cobra-tree dispatch.
func TestCommandLookup_AmbiguousNameErrors(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Commands: []schema.Command{
			{Name: "terraform", Commands: []schema.Command{{Name: "build"}}},
			{Name: "docker", Commands: []schema.Command{{Name: "build"}}},
		},
	}

	lookup := CommandLookup(atmosConfig)
	_, found, err := lookup(taskgraph.Ref{Kind: taskgraph.KindCommand, Name: "build"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAmbiguousCommandName)
	assert.False(t, found)
}

// TestCommandLookup_UniqueNestedNameResolves confirms the unambiguous case still works: a nested
// command referenced by its own name resolves correctly regardless of nesting depth.
func TestCommandLookup_UniqueNestedNameResolves(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Commands: []schema.Command{
			{Name: "terraform", Commands: []schema.Command{
				{Name: "build", Dependencies: &schema.Dependencies{
					Commands: schema.UnitDependencies{{Name: "lint"}},
				}},
			}},
		},
	}

	lookup := CommandLookup(atmosConfig)
	refs, found, err := lookup(taskgraph.Ref{Kind: taskgraph.KindCommand, Name: "build"})

	require.NoError(t, err)
	assert.True(t, found)
	require.Len(t, refs, 1)
	assert.Equal(t, "lint", refs[0].Name)
}

// TestCommandLookup_UnknownNameReturnsNotFound confirms an unregistered command name resolves
// to found=false with no error, distinguishing "this command doesn't exist" (a graph-build
// error surfaced by taskgraph.buildGraph as ErrUnknownDependency) from an actual lookup failure.
func TestCommandLookup_UnknownNameReturnsNotFound(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Commands: []schema.Command{
			{Name: "terraform"},
		},
	}

	lookup := CommandLookup(atmosConfig)
	refs, found, err := lookup(taskgraph.Ref{Kind: taskgraph.KindCommand, Name: "does-not-exist"})

	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, refs)
}

// TestCommandLookup_NoDependenciesReturnsEmptyFound confirms a resolved command with no
// dependencies: block at all (Dependencies == nil, not merely an empty one) still reports
// found=true with a nil/empty ref slice -- a leaf command in the graph, not a lookup failure.
func TestCommandLookup_NoDependenciesReturnsEmptyFound(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Commands: []schema.Command{
			{Name: "lint"},
		},
	}

	lookup := CommandLookup(atmosConfig)
	refs, found, err := lookup(taskgraph.Ref{Kind: taskgraph.KindCommand, Name: "lint"})

	require.NoError(t, err)
	assert.True(t, found)
	assert.Empty(t, refs)
}

// TestWorkflowLookup_NoFileErrors confirms a command-depends-on-workflow reference (defaultFile
// == "", since a custom command has no "current workflow file" to fall back to) with no
// explicit `file:` of its own returns the documented ErrWorkflowNoWorkflow guidance, rather than
// resolving against an empty/garbage path.
func TestWorkflowLookup_NoFileErrors(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	lookup := WorkflowLookup(atmosConfig, "")

	refs, found, err := lookup(taskgraph.Ref{Kind: taskgraph.KindWorkflow, Name: "deploy"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowNoWorkflow)
	assert.False(t, found)
	assert.Nil(t, refs)
}

// TestWorkflowLookup_LoadErrorPropagates confirms a defaultFile that doesn't resolve to an
// existing workflow manifest surfaces LoadWorkflowConfig's own error (ErrWorkflowFileNotFound)
// through WorkflowLookup, rather than being swallowed as a plain not-found.
func TestWorkflowLookup_LoadErrorPropagates(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	lookup := WorkflowLookup(atmosConfig, "does-not-exist.yaml")

	refs, found, err := lookup(taskgraph.Ref{Kind: taskgraph.KindWorkflow, Name: "deploy"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowFileNotFound)
	assert.False(t, found)
	assert.Nil(t, refs)
}

// TestWorkflowLookup_UnknownWorkflowNameReturnsNotFound confirms a workflow file that loads
// successfully but doesn't define the requested workflow name resolves to found=false with no
// error -- the graph builder is the one that turns this into ErrUnknownDependency, not the
// lookup itself.
func TestWorkflowLookup_UnknownWorkflowNameReturnsNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, "workflows.yaml")
	require.NoError(t, os.WriteFile(workflowPath, []byte("workflows:\n  build:\n    steps: []\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{}
	lookup := WorkflowLookup(atmosConfig, workflowPath)

	refs, found, err := lookup(taskgraph.Ref{Kind: taskgraph.KindWorkflow, Name: "deploy"})

	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, refs)
}

// TestWorkflowRunner_LoadErrorPropagates mirrors TestWorkflowLookup_LoadErrorPropagates for the
// execution side: a defaultFile that doesn't resolve to an existing manifest must fail the
// dispatch with LoadWorkflowConfig's own error, not silently no-op.
func TestWorkflowRunner_LoadErrorPropagates(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	runner := WorkflowRunner(atmosConfig, "does-not-exist.yaml", false, "")

	err := runner(context.Background(), taskgraph.Ref{Kind: taskgraph.KindWorkflow, Name: "deploy"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowFileNotFound)
}

// TestWorkflowRunner_UnknownWorkflowNameErrors confirms WorkflowRunner returns a clear
// ErrWorkflowNoWorkflow (rather than a nil-pointer panic on a zero-value WorkflowDefinition) when
// dispatched against a ref whose name isn't defined in the resolved file -- this can only happen
// if a lookup and a runner ever disagree about what "found" means, so the runner must fail
// loudly on its own instead of assuming the lookup already guaranteed existence.
func TestWorkflowRunner_UnknownWorkflowNameErrors(t *testing.T) {
	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, "workflows.yaml")
	require.NoError(t, os.WriteFile(workflowPath, []byte("workflows:\n  build:\n    steps: []\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{}
	runner := WorkflowRunner(atmosConfig, workflowPath, false, "")

	err := runner(context.Background(), taskgraph.Ref{Kind: taskgraph.KindWorkflow, Name: "deploy"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowNoWorkflow)
}

// TestCommandRunnerViaSubprocess_InvokesRunningBinaryWithFormattedArgs verifies the actual
// subprocess dispatch: commandRunnerViaSubprocess must shell out to the CURRENTLY-RUNNING
// binary (via resolveAtmosBinary/os.Executable, not a bare "atmos" PATH lookup) with the
// dependency's name, its flags formatted as `--flag=value`, then its positional args. Uses the
// _ATMOS_TEST_ARGS_FILE test-binary-as-subprocess convention (see testmain_test.go) so this
// runs the real ExecuteShellCommand/os/exec path end to end instead of only asserting on
// constructed strings.
func TestCommandRunnerViaSubprocess_InvokesRunningBinaryWithFormattedArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("_ATMOS_TEST_ARGS_FILE", argsFile)

	atmosConfig := &schema.AtmosConfiguration{}
	runner := commandRunnerViaSubprocess(atmosConfig)

	err := runner(context.Background(), taskgraph.Ref{
		Kind:  taskgraph.KindCommand,
		Name:  "mycommand",
		Flags: map[string]string{"env": "dev"},
		Args:  []string{"--foo"},
	})
	require.NoError(t, err, "the subprocess (this test binary, intercepted by TestMain) must exit 0")

	content, readErr := os.ReadFile(argsFile)
	require.NoError(t, readErr, "TestMain must have written the subprocess's argv to _ATMOS_TEST_ARGS_FILE")
	got := strings.Split(string(content), "\n")
	require.Len(t, got, 3)
	assert.Equal(t, "mycommand", got[0], "first arg must be the dependency's command name")
	assert.Equal(t, "--env=dev", got[1], "flags must be formatted as --name=value")
	assert.Equal(t, "--foo", got[2], "positional args must be appended after flags")
}
