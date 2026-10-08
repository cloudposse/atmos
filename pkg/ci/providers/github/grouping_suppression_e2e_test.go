package github

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// End-to-end regression coverage for #3309: drive the REAL GitHub provider
// through both ci grouping entry points with GITHUB_ACTION_REPOSITORY set, and
// assert that detected legacy actions keep structured stdout free of
// `::group::`/`::endgroup::` metadata while non-legacy runs still group. This
// exercises the full path (env → provider.SuppressLogGrouping → ci grouper →
// data stdout) that the mock-based unit tests only approximate.

// sentinelEnvKey derives the log-group sentinel env var name from the exported
// "KEY=VALUE" helper, so the test does not hardcode the internal constant.
func sentinelEnvKey(t *testing.T) string {
	t.Helper()
	k, _, ok := strings.Cut(ci.LogGroupSentinelEnv(), "=")
	require.True(t, ok)
	return k
}

// setupGroupingE2E registers the real GitHub provider as the detected CI
// provider, points it at actionRepo, and captures the data channel (stdout).
func setupGroupingE2E(t *testing.T, actionRepo string) *bytes.Buffer {
	t.Helper()
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)

	t.Setenv("GITHUB_ACTIONS", "true") // Provider.Detect() returns true.
	t.Setenv("GITHUB_ACTION_REPOSITORY", actionRepo)
	t.Setenv(sentinelEnvKey(t), "") // Not already inside a parent group.
	ci.Register(NewProvider())

	stdout := &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	return stdout
}

// ciEnabledConfig is a CI-enabled config in the default (auto) grouping mode,
// under which DimensionStep is active.
func ciEnabledConfig() *schema.AtmosConfiguration {
	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Enabled = true
	return cfg
}

const e2eJSONPayload = `{"settings":{"key":"value"}}`

func TestGrouping_E2E_StdoutStaysCleanInLegacyActions(t *testing.T) {
	legacyRepos := []string{
		"cloudposse/github-action-atmos-get-setting",
		"cloudposse/github-action-terraform-plan-storage", // Does not match the old atmos-* prefix.
	}

	for _, repo := range legacyRepos {
		t.Run("ci.Group/"+repo, func(t *testing.T) {
			stdout := setupGroupingE2E(t, repo)

			err := ci.Group(ciEnabledConfig(), ci.DimensionStep, "describe component", func() error {
				return data.Writeln(e2eJSONPayload)
			})
			require.NoError(t, err)

			out := stdout.String()
			assert.Contains(t, out, e2eJSONPayload, "the JSON payload must reach stdout")
			assert.NotContains(t, out, "::group::", "no group marker may pollute stdout in a legacy action")
			assert.NotContains(t, out, "::endgroup::")
		})

		t.Run("ci.StartLogGroup/"+repo, func(t *testing.T) {
			stdout := setupGroupingE2E(t, repo)

			end := ci.StartLogGroup("describe component")
			require.NoError(t, data.Writeln(e2eJSONPayload))
			end()

			out := stdout.String()
			assert.Contains(t, out, e2eJSONPayload)
			assert.NotContains(t, out, "::group::")
			assert.NotContains(t, out, "::endgroup::")
		})
	}
}

// Control: outside the legacy-action set, grouping still brackets stdout output
// with markers on stdout, proving suppression is specific to legacy actions.
func TestGrouping_E2E_NonLegacyActionStillGroups(t *testing.T) {
	stdout := setupGroupingE2E(t, "cloudposse/github-action-setup-atmos")

	err := ci.Group(ciEnabledConfig(), ci.DimensionStep, "describe component", func() error {
		return data.Writeln(e2eJSONPayload)
	})
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "::group::describe component")
	assert.Contains(t, out, e2eJSONPayload)
	assert.Contains(t, out, "::endgroup::")
	assert.True(t, ci.GroupingEnabled(ciEnabledConfig()))
}

// The fn error must propagate even when grouping is suppressed, and the sentinel
// must not be reported as propagatable in a legacy action.
func TestGrouping_E2E_SuppressedGroupPropagatesCallbackError(t *testing.T) {
	setupGroupingE2E(t, "cloudposse/github-action-atmos-get-setting")

	sentinel := errors.New("boom")
	err := ci.Group(ciEnabledConfig(), ci.DimensionStep, "describe component", func() error {
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	assert.False(t, ci.GroupingEnabled(ciEnabledConfig()),
		"grouping must be reported unavailable in a legacy action")
	assert.False(t, ci.ShouldPropagateLogGroupSentinel(ciEnabledConfig(), ci.DimensionStep),
		"the sentinel must not propagate when grouping is suppressed")
}

// Depth must not leak: a suppressed Group runs fn without touching the nesting
// state, so a subsequent non-suppressed run still emits markers. If suppression
// had incremented the depth counter without decrementing, this second run would
// silently no-op.
func TestGrouping_E2E_SuppressionDoesNotLeakNestingDepth(t *testing.T) {
	// First, a suppressed run in a legacy action.
	setupGroupingE2E(t, "cloudposse/github-action-atmos-get-setting")
	require.NoError(t, ci.Group(ciEnabledConfig(), ci.DimensionStep, "suppressed", func() error { return nil }))

	// Then a fresh non-legacy run must still group.
	stdout := setupGroupingE2E(t, "cloudposse/github-action-setup-atmos")
	require.NoError(t, ci.Group(ciEnabledConfig(), ci.DimensionStep, "active", func() error {
		return data.Writeln(e2eJSONPayload)
	}))
	assert.Contains(t, stdout.String(), "::group::active", "nesting depth must not leak across suppressed runs")
}
