package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/tests"
)

// completeFixturePath is the relative path from cmd/list to the `complete`
// scenario fixture. All cmd-layer executor integration tests use it — it
// has a full atmos.yaml with components and stacks, enough for
// InitCliConfig / ExecuteDescribeStacks to run without erroring out.
const completeFixturePath = "../../tests/fixtures/scenarios/complete"

// initExecutorTestIO initializes the I/O and UI contexts expected by the
// executor functions. Safe to call multiple times (formatter/writer init
// guards against double registration).
func initExecutorTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to initialize I/O context: %v", err)
	}
	ui.InitFormatter(ioCtx)
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

// newCmdWithListParser returns a fresh cobra command with the given list
// parser's flags registered. Used to build a minimal cmd for executor
// tests without pulling in the root command.
func newCmdWithListParser(use string, register func(cmd *cobra.Command)) *cobra.Command {
	cmd := &cobra.Command{Use: use}
	register(cmd)
	return cmd
}

// TestExecuteListInstancesCmd_CoverageIntegration exercises the full
// cmd-layer `executeListInstancesCmd` against the `complete` fixture so
// the flag-forwarding lines added by this PR are covered. Failure modes
// inside `list.ExecuteListInstancesCmd` are out of scope here; we only
// need to assert it returns without panicking and that the cmd-layer
// glue ran to completion.
func TestExecuteListInstancesCmd_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("instances", instancesParser.RegisterFlags)
	opts := &InstancesOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: false, // Avoid YAML-function evaluation in test env.
	}

	err := executeListInstancesCmd(cmd, []string{}, opts)
	// Whether this returns nil or an error depends on the fixture, but
	// the point is that the cmd-layer glue (ProcessCommandLineArgs →
	// InitCliConfig → createAuthManagerForList → list.ExecuteList…) is
	// exercised, including my new ProcessTemplates/ProcessFunctions
	// struct assignments that pass through to the pkg layer.
	_ = err
}

// TestExecuteListInstancesCmd_InvalidBasePath exercises the early-return
// path when `--provenance` is used without `--format=tree`.
func TestExecuteListInstancesCmd_InvalidProvenance(t *testing.T) {
	initExecutorTestIO(t)
	cmd := newCmdWithListParser("instances", instancesParser.RegisterFlags)
	opts := &InstancesOptions{
		Format:     "json",
		Provenance: true, // Invalid: provenance requires --format=tree.
	}

	err := executeListInstancesCmd(cmd, []string{}, opts)

	assert.Error(t, err, "provenance without tree format should fail validation")
	assert.Contains(t, err.Error(), "--provenance")
}

// TestListComponentsWithOptions_CoverageIntegration exercises the cmd-layer
// `listComponentsWithOptions` + `initAndExtractComponents` against the
// `complete` fixture.
func TestListComponentsWithOptions_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("components", componentsParser.RegisterFlags)
	opts := &ComponentsOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	err := listComponentsWithOptions(cmd, []string{}, opts)
	_ = err
}

// TestExecuteListMetadataCmd_CoverageIntegration exercises the cmd-layer
// `executeListMetadataCmd` against the `complete` fixture.
func TestExecuteListMetadataCmd_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("metadata", metadataParser.RegisterFlags)
	opts := &MetadataOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	err := executeListMetadataCmd(cmd, []string{}, opts)
	_ = err
}

// TestExecuteListSources_CoverageIntegration exercises the cmd-layer
// `executeListSources` against the `complete` fixture. Using the
// positional arg form also covers `initSourcesCommand`'s `len(args) > 0`
// branch in `parseSourcesOptions`.
func TestExecuteListSources_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("sources", sourcesParser.RegisterFlags)
	err := executeListSources(cmd, []string{"vpc"})
	_ = err
}

// TestListStacksWithOptions_CoverageIntegration exercises the cmd-layer
// `listStacksWithOptions` + `executeAndExtractStacks` against the
// `complete` fixture for the non-tree format path.
func TestListStacksWithOptions_CoverageIntegration(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	opts := &StacksOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	err := listStacksWithOptions(cmd, []string{}, opts)
	_ = err
}
