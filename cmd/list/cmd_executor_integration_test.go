package list

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/tests"
)

// completeFixturePath is the relative path from cmd/list to the `complete`
// scenario fixture. All cmd-layer executor integration tests use it — it
// has a full atmos.yaml with components and stacks, enough for
// InitCliConfig / ExecuteDescribeStacks to run without erroring out.
// Built with filepath.Join for Windows compatibility.
var completeFixturePath = filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete")

var dependenciesFixturePath = filepath.Join("..", "..", "tests", "fixtures", "scenarios", "dependencies-components-inheritance")

// initExecutorTestIO initializes the I/O, UI, and data contexts expected
// by the executor functions. Safe to call multiple times (init guards
// against double registration).
func initExecutorTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "failed to initialize I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
}

// chdirToCompleteFixture cd's into the `complete` scenario fixture for the
// duration of the test. The cmd-layer executor functions read atmos.yaml via
// discovery from CWD, so we can't simply pass a BasePath — the CWD must be
// inside the fixture so `cfg.InitCliConfig` finds the config.
func chdirToCompleteFixture(t *testing.T) {
	t.Helper()
	tests.RequireFilePath(t, completeFixturePath, "test fixture directory")
	t.Chdir(completeFixturePath)
}

func chdirToDependenciesFixture(t *testing.T) {
	t.Helper()
	tests.RequireFilePath(t, dependenciesFixturePath, "test fixture directory")
	t.Chdir(dependenciesFixturePath)
}

// newCmdWithListParser returns a fresh cobra command with the given list
// parser's flags registered alongside the global flags
// (`base-path`, `config`, `config-path`, `profile`, …). The executor
// functions in cmd/list/*.go call `ProcessCommandLineArgs` which reads
// those global flags, so they must be present on the command for the
// executor to run at all.
func newCmdWithListParser(use string, register func(cmd *cobra.Command)) *cobra.Command {
	cmd := &cobra.Command{Use: use}
	// Register global flags first — matches how RootCmd is assembled.
	flags.NewGlobalOptionsBuilder().Build().RegisterFlags(cmd)
	register(cmd)
	return cmd
}

// TestExecuteListInstancesCmd_CoverageIntegration exercises the full
// cmd-layer `executeListInstancesCmd` against the `complete` fixture and
// asserts it completes without error. Covers the whole glue path —
// ProcessCommandLineArgs → InitCliConfig → createAuthManagerForList →
// list.ExecuteListInstancesCmd — including the PR's new
// `ProcessTemplates` / `ProcessFunctions` pass-throughs.
func TestExecuteListInstancesCmd_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("instances", instancesParser.RegisterFlags)
	opts := &InstancesOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: false, // Avoid YAML-function evaluation in test env.
	}

	require.NoError(t, executeListInstancesCmd(cmd, []string{}, opts),
		"complete fixture should list instances cleanly with ProcessTemplates=true, ProcessFunctions=false")
}

// TestExecuteListInstancesCmd_InvalidProvenance exercises the early-return
// validation path when `--provenance` is used without `--format=tree`.
// Does not need the fixture since the validation runs before any config
// loading.
func TestExecuteListInstancesCmd_InvalidProvenance(t *testing.T) {
	initExecutorTestIO(t)
	cmd := newCmdWithListParser("instances", instancesParser.RegisterFlags)
	opts := &InstancesOptions{
		Format:     "json",
		Provenance: true, // Invalid: provenance requires --format=tree.
	}

	err := executeListInstancesCmd(cmd, []string{}, opts)

	require.Error(t, err, "provenance without tree format should fail validation")
	assert.Contains(t, err.Error(), "--provenance")
}

// TestListComponentsWithOptions_CoverageIntegration exercises the cmd-layer
// `listComponentsWithOptions` + `initAndExtractComponents` against the
// `complete` fixture and asserts a clean run.
func TestListComponentsWithOptions_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("components", componentsParser.RegisterFlags)
	opts := &ComponentsOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	require.NoError(t, listComponentsWithOptions(cmd, []string{}, opts),
		"complete fixture should list components cleanly")
}

// TestExecuteListMetadataCmd_CoverageIntegration exercises the cmd-layer
// `executeListMetadataCmd` against the `complete` fixture and asserts a
// clean run.
func TestExecuteListMetadataCmd_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("metadata", metadataParser.RegisterFlags)
	opts := &MetadataOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	require.NoError(t, executeListMetadataCmd(cmd, []string{}, opts),
		"complete fixture should list metadata cleanly")
}

// TestExecuteListSources_CoverageIntegration exercises the cmd-layer
// `executeListSources` against the `complete` fixture. Using the
// positional arg form also covers the `len(args) > 0` branch in
// `parseSourcesOptions`. The `complete` fixture does not define a `vpc`
// source component — the informational "No components with source
// configured matching component 'vpc'" output that the command prints
// is expected, and the command still returns nil.
func TestExecuteListSources_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("sources", sourcesParser.RegisterFlags)
	require.NoError(t, executeListSources(cmd, []string{"vpc"}),
		"executeListSources should return nil when no matching sources — prints an info message")
}

// TestExecuteListDependenciesCmd_CoverageIntegration exercises the cmd-layer
// `executeListDependenciesCmd` against a dependency-focused fixture. This covers
// the command glue path — ProcessCommandLineArgs → InitCliConfig →
// createAuthManagerForList → ExecuteDescribeStacksWithAuthDisabled →
// dependencies.BuildGraph/Render.
func TestExecuteListDependenciesCmd_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToDependenciesFixture(t)

	cmd := newCmdWithListParser("dependencies", dependenciesParser.RegisterFlags)
	opts := &DependenciesOptions{
		Format:           "json",
		Direction:        "both",
		Stack:            "dev",
		Component:        "vpc",
		ProcessTemplates: true,
		ProcessFunctions: false,
		AuthDisabled:     true,
	}

	require.NoError(t, executeListDependenciesCmd(cmd, []string{"vpc"}, opts),
		"dependencies fixture should list vpc dependencies cleanly")
}

// TestListStacksWithOptions_CoverageIntegration exercises the cmd-layer
// `listStacksWithOptions` + `executeAndExtractStacks` against the
// `complete` fixture for the non-tree format path and asserts a clean
// run.
func TestListStacksWithOptions_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	opts := &StacksOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	require.NoError(t, listStacksWithOptions(cmd, []string{}, opts),
		"complete fixture should list stacks cleanly")
}

// TestListStacksWithOptions_NoStacksYet exercises `listStacksWithOptions` against a
// brand-new project with no stack manifests at all (no stacks/ directory). It must
// report a friendly "No stacks found" message instead of the raw config error
// ("failed to find import") that `cfg.InitCliConfig` returns for zero glob matches.
func TestListStacksWithOptions_NoStacksYet(t *testing.T) {
	initExecutorTestIO(t)

	tmpDir := t.TempDir()
	componentsDir := filepath.Join(tmpDir, "components", "terraform")
	require.NoError(t, os.MkdirAll(componentsDir, 0o755))

	atmosYaml := `
base_path: "` + filepath.ToSlash(tmpDir) + `"
stacks:
  base_path: stacks
  included_paths:
    - "**/*"
components:
  terraform:
    base_path: components/terraform
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYaml), 0o644))

	t.Chdir(tmpDir)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	opts := &StacksOptions{Format: "json"}

	require.NoError(t, listStacksWithOptions(cmd, []string{}, opts),
		"a project with no stacks yet should report a friendly message, not an error")
}

// TestListStacksWithOptions_TreeFormat exercises the tree-format branch
// (`renderStacksTreeFormat` + `resolveAndFilterImportTrees` +
// `buildAllowedStacksSet`) against the `complete` fixture. These are
// provenance-aware paths that don't run for the default `json`/`table`
// formats.
func TestListStacksWithOptions_TreeFormat(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	opts := &StacksOptions{
		Format:           "tree",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	require.NoError(t, listStacksWithOptions(cmd, []string{}, opts),
		"complete fixture should render the tree format cleanly")
}

// TestListStacksWithOptions_TreeFormatWithProvenance exercises the tree
// format with `--provenance` enabled, which activates the import-chain
// annotation path inside `format.RenderStacksTree`.
func TestListStacksWithOptions_TreeFormatWithProvenance(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	opts := &StacksOptions{
		Format:           "tree",
		Provenance:       true,
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	require.NoError(t, listStacksWithOptions(cmd, []string{}, opts),
		"complete fixture should render the tree-with-provenance format cleanly")
}

// TestExecuteListInstancesCmd_TreeFormat exercises the tree-format branch
// of `list.ExecuteListInstancesCmd` through the cmd-layer executor —
// covers the `opts.Format == tree` branch that bypasses the normal
// render pipeline and calls `importresolver.ResolveImportTreeFromProvenance`
// directly.
func TestExecuteListInstancesCmd_TreeFormat(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("instances", instancesParser.RegisterFlags)
	opts := &InstancesOptions{
		Format:           "tree",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	require.NoError(t, executeListInstancesCmd(cmd, []string{}, opts),
		"complete fixture should render instances in tree format cleanly")
}

// TestExecuteListInstancesCmd_MatrixFormat exercises the matrix-format
// branch (`executeMatrixFormat`) which produces GitHub-Actions-compatible
// JSON for driving parallel CI jobs.
func TestExecuteListInstancesCmd_MatrixFormat(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("instances", instancesParser.RegisterFlags)
	opts := &InstancesOptions{
		Format:           "matrix",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	require.NoError(t, executeListInstancesCmd(cmd, []string{}, opts),
		"complete fixture should render instances in matrix format cleanly")
}
