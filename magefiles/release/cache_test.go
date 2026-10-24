package release

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseCacheWarmupUsesProductionBuildConfiguration(t *testing.T) {
	var workflow pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-cache-warmup.yml", &workflow)
	job := workflow.Jobs["warmup"]
	require.Equal(t, "github.ref == 'refs/heads/main'", job.If)
	require.Equal(t, map[string]string{"contents": "read"}, workflow.Permissions)
	require.NotContains(t, workflow.On, "pull_request")
	require.NotContains(t, workflow.On, "pull_request_target")

	warm := pipelineUses(t, job.Steps, "goreleaser/goreleaser-action")
	release := pipelineUses(t, pipelineAction(t, "release-build"), "goreleaser/goreleaser-action")
	require.Equal(t, release.Uses, warm.Uses)
	require.Equal(t, release.With["version"], warm.With["version"])
	require.Equal(t, release.With["distribution"], warm.With["distribution"])
	require.Equal(t, "build --snapshot --clean --skip=before --timeout 150m --config .goreleaser.yml", warm.With["args"],
		"warm all production targets and flags without packaging, publishing or running release hooks")

	cache := pipelineUses(t, job.Steps, "./.github/actions/setup-release-cache")
	save := pipelineUses(t, job.Steps, "actions/cache/save")
	restore := pipelineUses(t, pipelineAction(t, "setup-release-cache"), "actions/cache/restore")
	require.Equal(t, restore.With["path"], save.With["path"], "cache versions must match between writer and reader")
	require.Equal(t, "${{ steps."+cache.ID+".outputs.cache-primary-key }}-${{ github.run_id }}-${{ github.run_attempt }}", save.With["key"])
	require.Empty(t, save.If, "save only after the entire matrix succeeds, never on failure")
	require.Equal(t, save.Uses, job.Steps[len(job.Steps)-1].Uses)
	for _, step := range job.Steps {
		require.False(t, step.ContinueOnError)
	}
}

func TestReleaseCacheIsRestoreOnlyAndSeparateFromNativeCaches(t *testing.T) {
	steps := pipelineAction(t, "setup-release-cache")
	setup := pipelineUses(t, steps, "./.github/actions/setup-go-cache")
	require.Equal(t, "false", setup.With["cache"], "do not download the native/race archive as well")
	restore := pipelineUses(t, steps, "actions/cache/restore")
	require.Contains(t, restore.With["key"], "steps.go.outputs.go-version")
	require.Contains(t, restore.With["key"], "hashFiles('.goreleaser.yml')")
	require.Contains(t, restore.With["key"], "hashFiles(inputs.cache-dependency-path)")
	for _, prefix := range strings.Split(strings.TrimSpace(restore.With["restore-keys"]), "\n") {
		require.True(t, strings.HasPrefix(prefix, "release-go-cache-v1-"))
		require.Contains(t, prefix, "steps.go.outputs.go-version", "never restore another Go toolchain's cache")
	}
	for _, step := range steps {
		require.False(t, strings.HasPrefix(step.Uses, "actions/cache/save@"), "source build jobs must never write release caches")
	}
}
