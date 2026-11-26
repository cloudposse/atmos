//go:build mage

package ci

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

const (
	setupGoCacheAction = "./.github/actions/setup-go-cache"
	cacheRestoreAction = "actions/cache/restore@"

	// maxSetupGoCacheMinutes caps the call-site bound. Healthy "Set up Go" steps took at most 8.3 minutes
	// (Build (macos-intel)) across 448 recent Tests and CodeQL steps; a hung one ran for 43 minutes (#3766).
	maxSetupGoCacheMinutes = 15

	// maxCacheSegmentMinutes caps how long one 128 MB cache segment may stall before @actions/cache
	// gives up and the restore becomes a cache miss. It must stay well below the call-site bound, so a
	// stalled restore recovers as a miss instead of failing the step.
	maxCacheSegmentMinutes = 5
)

type boundedStep struct {
	Name    string            `yaml:"name"`
	Uses    string            `yaml:"uses"`
	Timeout any               `yaml:"timeout-minutes"`
	With    map[string]string `yaml:"with"`
	Env     map[string]string `yaml:"env"`
}

type boundedWorkflow struct {
	Jobs map[string]struct {
		Steps []boundedStep `yaml:"steps"`
	} `yaml:"jobs"`
}

type boundedAction struct {
	Runs struct {
		Steps []boundedStep `yaml:"steps"`
	} `yaml:"runs"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

// A hung toolchain download or cache restore inside setup-go-cache used to consume the whole job
// timeout (#3766). Composite steps cannot set timeout-minutes, so every workflow step that calls the
// action must bound it, with a literal value no larger than the cap.
func TestEverySetupGoCacheCallBoundsItsDuration(t *testing.T) {
	root := repositoryRoot(t)
	var paths []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(root, ".github", "workflows", pattern))
		require.NoError(t, err)
		paths = append(paths, matches...)
	}
	require.Positive(t, len(paths), "no workflows found under %s", root)

	calls := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var workflow boundedWorkflow
		require.NoError(t, yaml.Unmarshal(data, &workflow), path)
		for jobID, job := range workflow.Jobs {
			for _, step := range job.Steps {
				if step.Uses != setupGoCacheAction {
					continue
				}
				calls++
				where := filepath.Base(path) + " job " + jobID + " step " + strconv.Quote(step.Name)
				if !assert.NotNil(t, step.Timeout, "%s calls setup-go-cache without timeout-minutes", where) {
					continue
				}
				minutes, isLiteral := step.Timeout.(int)
				if !assert.True(t, isLiteral, "%s: timeout-minutes must be a literal number of minutes, got %v", where, step.Timeout) {
					continue
				}
				assert.Positive(t, minutes, where)
				assert.LessOrEqual(t, minutes, maxSetupGoCacheMinutes, where)
			}
		}
	}
	// Guards against the scan silently matching nothing after a rename or move of the action.
	require.Positive(t, calls, "no workflow step uses %s", setupGoCacheAction)
}

// @actions/cache aborts a segment that exceeds SEGMENT_DOWNLOAD_TIMEOUT_MINS and restoreCacheV2 turns
// that into a "Failed to restore" warning and a cache miss. Each restore in setup-go-cache must set a
// short segment timeout and must not fail on a miss, so a stalled restore recovers instead of hanging.
func TestSetupGoCacheRestoresFallBackToAMissOnAStall(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "actions", "setup-go-cache", "action.yml"))
	require.NoError(t, err)
	var action boundedAction
	require.NoError(t, yaml.Unmarshal(data, &action))

	restores := 0
	for _, step := range action.Runs.Steps {
		if !strings.HasPrefix(step.Uses, cacheRestoreAction) {
			continue
		}
		restores++
		minutes, err := strconv.Atoi(step.Env["SEGMENT_DOWNLOAD_TIMEOUT_MINS"])
		if assert.NoError(t, err, "%q must set a numeric SEGMENT_DOWNLOAD_TIMEOUT_MINS", step.Name) {
			assert.Positive(t, minutes, step.Name)
			assert.LessOrEqual(t, minutes, maxCacheSegmentMinutes, step.Name)
		}
		assert.NotEqual(t, "true", step.With["fail-on-cache-miss"], "%q must treat a timed-out restore as a miss", step.Name)
	}
	require.Positive(t, restores, "setup-go-cache has no actions/cache/restore steps")
}
