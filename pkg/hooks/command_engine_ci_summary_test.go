package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ciEnabledCtx returns a minimal ExecContext with CI integration enabled — the
// master switch the reporting steps (summary/annotations/results) now require.
func ciEnabledCtx() *ExecContext {
	return &ExecContext{
		Hook:        &Hook{Kind: "checkov"},
		AtmosConfig: &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}},
		Info:        &schema.ConfigAndStacksInfo{Stack: "test", ComponentFromArg: "bucket"},
	}
}

// renderCISummary should append a hook's markdown summary to the GitHub Actions
// job step summary when running in CI, so scanner findings surface in the
// pipeline run rather than only the terminal log stream.
func TestRenderCISummary_WritesToGitHubStepSummary(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(github.NewProvider())

	summaryPath := filepath.Join(t.TempDir(), "step-summary.md")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	const body = "## checkov\n\n✅ no findings\n"
	renderCISummary(ciEnabledCtx(), &Output{Summary: &Summary{Kind: "checkov", Body: body}})

	got, err := os.ReadFile(summaryPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), "## checkov")
	assert.Contains(t, string(got), "no findings")
}

// Without a step-summary destination, renderCISummary must be a safe no-op: the
// findings already rendered to the terminal, so a missing/disabled CI summary
// must never error or panic.
func TestRenderCISummary_NoStepSummaryIsNoop(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(github.NewProvider())

	// Detected as GitHub Actions, but no GITHUB_STEP_SUMMARY path is set.
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", "")

	assert.NotPanics(t, func() {
		renderCISummary(ciEnabledCtx(), &Output{Summary: &Summary{Kind: "checkov", Body: "## checkov\n\nfindings\n"}})
	})
}

// renderCISummary must skip writing when there is nothing to report (nil output,
// nil summary, or an empty body) so it doesn't emit blank step-summary blocks.
func TestRenderCISummary_NothingToReport(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(github.NewProvider())

	summaryPath := filepath.Join(t.TempDir(), "step-summary.md")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	renderCISummary(ciEnabledCtx(), nil)
	renderCISummary(ciEnabledCtx(), &Output{})
	renderCISummary(ciEnabledCtx(), &Output{Summary: &Summary{Kind: "checkov", Body: ""}})

	_, err := os.Stat(summaryPath)
	assert.True(t, os.IsNotExist(err), "no summary file should be created when there is nothing to report")
}

// Two scanners running in the same job must each land as their own chapter:
// renderCISummary appends (never overwrites), preserves any pre-existing step
// summary content, and prefixes a blank line so each `## <tool>` heading
// renders as a heading rather than being glued onto the previous block.
func TestRenderCISummary_AppendsSeparatedChaptersPreservingExisting(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(github.NewProvider())

	summaryPath := filepath.Join(t.TempDir(), "step-summary.md")
	// Simulate content a prior step already wrote, with no trailing blank line.
	require.NoError(t, os.WriteFile(summaryPath, []byte("Existing content from a prior step."), 0o600))
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	renderCISummary(ciEnabledCtx(), &Output{Summary: &Summary{Kind: "checkov", Body: "## checkov\n\n✅ no findings\n"}})
	renderCISummary(ciEnabledCtx(), &Output{Summary: &Summary{Kind: "trivy", Body: "## trivy\n\n✅ no findings\n"}})

	got, err := os.ReadFile(summaryPath)
	require.NoError(t, err)
	s := string(got)

	// Pre-existing content survived (append, not overwrite).
	assert.True(t, strings.HasPrefix(s, "Existing content from a prior step."), "must not overwrite prior content")
	// Both chapters present and ordered.
	assert.Contains(t, s, "## checkov")
	assert.Contains(t, s, "## trivy")
	assert.Less(t, strings.Index(s, "## checkov"), strings.Index(s, "## trivy"), "summaries append in order")
	// Each heading starts its own line, so GFM renders it as a heading even
	// when the preceding (non-hook) content had no trailing newline.
	assert.Contains(t, s, "\n## checkov")
	// Consecutive hook chapters are separated by a full blank line (each body
	// ends in "\n" and renderCISummary prefixes another "\n").
	assert.Contains(t, s, "\n\n## trivy")
}

// End-to-end through CommandEngine.Run: the four built-in scanner/cost kinds
// (checkov, trivy, kics, infracost) all share this engine and surface their
// findings via a ResultHandler that returns a Summary. This test exercises that
// shared path — a ResultHandler-produced summary must reach the CI step summary
// after a real Run — so it covers every CommandEngine-based built-in kind, not
// just the renderCISummary helper in isolation.
func TestCommandEngine_Run_WritesSummaryToCIStepSummary(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(github.NewProvider())

	summaryPath := filepath.Join(t.TempDir(), "step-summary.md")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	exe := testExePath(t)
	const body = "## checkov\n\n| Severity | Rule |\n|---|---|\n| high | CKV_AWS_19 |\n"
	kind := &Kind{
		Name:   "scanner-like",
		Engine: &CommandEngine{},
		ResultHandler: func(ctx *ExecContext) (*Summary, error) {
			return &Summary{Kind: ctx.Hook.Kind, Status: StatusWarning, Title: "findings", Body: body}, nil
		},
	}
	hook := &Hook{
		Kind:    "scanner-like",
		Command: exe,
		Args:    []string{"-test.run", "^$"},
		Env:     map[string]string{"_ATMOS_TEST_WRITE_OUTPUT": "1", "_ATMOS_TEST_OUTPUT_BODY": "raw"},
	}
	ctx := &ExecContext{
		Hook:        kind.ResolveDefaults(hook),
		Kind:        kind,
		AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: t.TempDir(), CI: schema.CIConfig{Enabled: true}},
		Info:        &schema.ConfigAndStacksInfo{Stack: "s", ComponentFromArg: "c"},
	}

	_, err := kind.Engine.Run(ctx)
	require.NoError(t, err)

	got, err := os.ReadFile(summaryPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), "## checkov")
	assert.Contains(t, string(got), "CKV_AWS_19")
}
