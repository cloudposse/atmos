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

	// The call-site bound is capped at maxSetupGoCacheMinutes. Healthy "Set up Go" steps took at most
	// 8.3 minutes (Build (macos-intel)) across 448 recent Tests and CodeQL steps; a hung one ran for
	// 43 minutes (#3766).
	maxSetupGoCacheMinutes = 15

	// A literal job timeout must exceed the call-site bound by at least minJobHeadroomMinutes, so the
	// step bound expires first and the hang fails as "Set up Go" instead of a cancelled job.
	minJobHeadroomMinutes = 3

	// One 128 MB cache segment may stall for at most maxCacheSegmentMinutes before @actions/cache gives
	// up and the restore becomes a cache miss. It stays well below the call-site bound, so a stalled
	// restore recovers as a miss instead of failing the step.
	maxCacheSegmentMinutes = 5
)

// Workflows whose setup-go-cache calls are deliberately left unbounded, with the reason. Each entry
// must still call the action, so a stale exemption fails the test.
var setupGoCacheExemptWorkflows = map[string]string{
	// plugins/releases.yaml lists this workflow as a build input of every plugin package, so any edit
	// to it requires bumping every plugin version and republishing unchanged plugin bytes.
	"plugin-packages.yaml": "plugin release build input",
}

type boundedStep struct {
	Name    string            `yaml:"name"`
	Uses    string            `yaml:"uses"`
	Timeout any               `yaml:"timeout-minutes"`
	With    map[string]string `yaml:"with"`
	Env     map[string]string `yaml:"env"`
}

type boundedJob struct {
	Timeout any           `yaml:"timeout-minutes"`
	Steps   []boundedStep `yaml:"steps"`
}

type boundedWorkflow struct {
	Jobs map[string]boundedJob `yaml:"jobs"`
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

func workflowPaths(t *testing.T) []string {
	t.Helper()
	root := repositoryRoot(t)
	var paths []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(root, ".github", "workflows", pattern))
		require.NoError(t, err)
		paths = append(paths, matches...)
	}
	require.Positive(t, len(paths), "no workflows found under %s", root)
	return paths
}

// checkSetupGoCacheBound asserts one call site's literal step bound fits the cap and, when the job has a
// literal timeout, expires at least minJobHeadroomMinutes before it.
func checkSetupGoCacheBound(t assert.TestingT, where string, job *boundedJob, step *boundedStep) {
	if helper, ok := t.(interface{ Helper() }); ok {
		helper.Helper()
	}
	if !assert.NotNil(t, step.Timeout, "%s calls setup-go-cache without timeout-minutes", where) {
		return
	}
	minutes, isLiteral := step.Timeout.(int)
	if !assert.True(t, isLiteral, "%s: timeout-minutes must be a literal number of minutes, got %v", where, step.Timeout) {
		return
	}
	assert.Positive(t, minutes, where)
	assert.LessOrEqual(t, minutes, maxSetupGoCacheMinutes, where)
	// Expression job timeouts (matrix values) can't be evaluated here; the default 360 always fits.
	if jobMinutes, ok := job.Timeout.(int); ok {
		assert.LessOrEqual(t, minutes, jobMinutes-minJobHeadroomMinutes,
			"%s: the step bound must expire at least %d minutes before the %d-minute job timeout", where, minJobHeadroomMinutes, jobMinutes)
	}
}

// A hung toolchain download or cache restore inside setup-go-cache used to consume the whole job
// timeout (#3766). Composite steps cannot set timeout-minutes, so every workflow step that calls the
// action must bound it with a literal value that can take effect before the job timeout.
func TestEverySetupGoCacheCallBoundsItsDuration(t *testing.T) {
	calls := 0
	exemptCalls := map[string]int{}
	for _, path := range workflowPaths(t) {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var workflow boundedWorkflow
		require.NoError(t, yaml.Unmarshal(data, &workflow), path)
		file := filepath.Base(path)
		for jobID, job := range workflow.Jobs {
			for i := range job.Steps {
				step := &job.Steps[i]
				if step.Uses != setupGoCacheAction {
					continue
				}
				if _, exempt := setupGoCacheExemptWorkflows[file]; exempt {
					exemptCalls[file]++
					continue
				}
				calls++
				checkSetupGoCacheBound(t, file+" job "+jobID+" step "+strconv.Quote(step.Name), &job, step)
			}
		}
	}
	// Guards against the scan silently matching nothing after a rename or move of the action.
	require.Positive(t, calls, "no workflow step uses %s", setupGoCacheAction)
	for file, reason := range setupGoCacheExemptWorkflows {
		assert.Positive(t, exemptCalls[file], "stale exemption: %s (%s) no longer calls setup-go-cache", file, reason)
	}
}

// failureRecorder records assertion failures without failing the enclosing test.
type failureRecorder struct{ failures int }

func (r *failureRecorder) Errorf(string, ...any) { r.failures++ }

// A step bound that equals or exceeds its job timeout can never fire, so the check must reject it, and
// it must accept a bound that leaves the required headroom.
func TestSetupGoCacheBoundMustExpireBeforeTheJobTimeout(t *testing.T) {
	recorder := &failureRecorder{}
	checkSetupGoCacheBound(recorder, "short job", &boundedJob{Timeout: 10}, &boundedStep{Timeout: 15})
	assert.Positive(t, recorder.failures, "a 15-minute bound in a 10-minute job must be rejected")

	recorder = &failureRecorder{}
	checkSetupGoCacheBound(recorder, "tight job", &boundedJob{Timeout: 10}, &boundedStep{Timeout: 8})
	assert.Positive(t, recorder.failures, "a bound leaving under %d minutes of headroom must be rejected", minJobHeadroomMinutes)

	recorder = &failureRecorder{}
	checkSetupGoCacheBound(recorder, "fitting job", &boundedJob{Timeout: 10}, &boundedStep{Timeout: 7})
	assert.Zero(t, recorder.failures, "a 7-minute bound in a 10-minute job leaves the required headroom")

	recorder = &failureRecorder{}
	checkSetupGoCacheBound(recorder, "matrix job", &boundedJob{Timeout: "${{ matrix.jobTimeout }}"}, &boundedStep{Timeout: 15})
	assert.Zero(t, recorder.failures, "an expression job timeout can't be evaluated and must not fail the check")
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
