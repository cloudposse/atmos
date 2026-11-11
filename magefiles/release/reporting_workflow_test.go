package release

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTimingCancellationCannotCancelOrSkipQuality(t *testing.T) {
	var wf pipelineWorkflow
	pipelineYAML(t, ".github/workflows/ci-timing-summary.yml", &wf)
	require.Empty(t, wf.Concurrency.Group, "workflow cancellation would kill both siblings")
	timing, quality := wf.Jobs["summarize"], wf.Jobs["quality-report"]
	require.True(t, timing.Concurrency.Cancel)
	require.NotEmpty(t, timing.Concurrency.Group)
	require.Nil(t, quality.Needs, "quality must run even when timing is cancelled or fails")
	require.Equal(t, timing.If, quality.If)
	require.Contains(t, quality.If, "github.event.workflow_run.name == 'Quality Gate Signal'", "fork review signals can have no PR association")
	require.Equal(t, "./.github/workflows/quality-report.yml", quality.Uses)
	var report pipelineWorkflow
	pipelineYAML(t, ".github/workflows/quality-report.yml", &report)
	require.False(t, report.Jobs["report"].Concurrency.Cancel, "active reporting must complete")
	require.NotEqual(t, timing.Concurrency.Group, report.Jobs["report"].Concurrency.Group)
	var milestones pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-milestones.yml", &milestones)
	require.Equal(t, "release-milestone-policy", milestones.Concurrency.Group)
	require.False(t, milestones.Concurrency.Cancel)
}

func TestExplicitQualityRefreshExportsSnapshot(t *testing.T) {
	steps := pipelineAction(t, "quality-gate")
	operation := steps[pipelineRunIndex(t, steps, "quality:reportPR")]
	require.Contains(t, operation.Run, `go tool mage quality:status "$GITHUB_REPOSITORY" "$QUALITY_PR_NUMBER" > "$RUNNER_TEMP/quality-result.json"`)
	artifact := pipelineUses(t, steps, "actions/upload-artifact")
	require.Equal(t, "success() && inputs.operation == 'reportPR'", artifact.If)
	require.Equal(t, "${{ runner.temp }}/quality-result.json", artifact.With["path"])
	require.Equal(t, "error", artifact.With["if-no-files-found"])
}
