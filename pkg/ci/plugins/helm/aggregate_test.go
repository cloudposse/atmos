package helm

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestOnAfterAggregateRendersPlanSummary(t *testing.T) {
	writer := &fakeWriter{}
	ctx := &plugin.HookContext{
		Provider: fakeProvider{writer: writer},
		Aggregate: schema.HelmCIResultSet{
			Command: "plan",
			Results: []schema.HelmCIResult{
				{
					NodeID: "changed", Stack: "prod", Component: "api", Processed: true, DurationMS: 25,
					Summary: map[string]any{
						"chart": "oci://registry/api", "release_name": "api", "namespace": "apps",
						"target": "kubernetes", "diff": "+ kind: Deployment",
					},
				},
				{
					NodeID: "unchanged", Stack: "dev", Component: "web", Processed: true,
					Summary: map[string]any{"chart": "web", "release_name": "web", "namespace": "apps", "target": "git"},
				},
				{
					NodeID: "failed", Stack: "dev", Component: "db", ExitCode: 1, Error: "render failed",
					Summary: map[string]any{"chart": "db"},
				},
				{NodeID: "skipped", Stack: "prod", Component: "worker"},
			},
		},
	}

	err := (&Plugin{}).onAfterAggregate(ctx)
	require.NoError(t, err)
	assert.Contains(t, writer.summary, "## Helm Plan Summary")
	assert.Contains(t, writer.summary, "Processed 4 component(s): 1 changed, 1 unchanged, 1 failed, 1 skipped.")
	assert.Contains(t, writer.summary, "| dev | db | failed | db |")
	assert.Contains(t, writer.summary, "| dev | web | no changes | web | web | apps | git | - |")
	assert.Contains(t, writer.summary, "| prod | api | changed | oci://registry/api | api | apps | kubernetes | 25ms |")
	assert.Contains(t, writer.summary, "+ kind: Deployment")
	assert.Contains(t, writer.summary, "render failed")
	assert.Less(t, strings.Index(writer.summary, "| dev | db |"), strings.Index(writer.summary, "| prod | api |"))
}

func TestOnAfterAggregateRendersApplySummary(t *testing.T) {
	writer := &fakeWriter{}
	err := (&Plugin{}).onAfterAggregate(&plugin.HookContext{
		Provider: fakeProvider{writer: writer},
		Aggregate: &schema.HelmCIResultSet{
			Command: "deploy",
			Results: []schema.HelmCIResult{{
				Stack: "dev", Component: "api", Processed: true,
				Summary: map[string]any{
					"chart": "api", "release_name": "api", "namespace": "apps", "target": "kubernetes",
				},
			}},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, writer.summary, "## Helm Apply Summary")
	assert.Contains(t, writer.summary, "Processed 1 component(s): 1 succeeded, 0 failed, 0 skipped.")
	assert.Contains(t, writer.summary, "| dev | api | succeeded | api | api | apps | kubernetes | - |")
}

func TestOnAfterAggregateUsesSafeDetailFences(t *testing.T) {
	writer := &fakeWriter{}
	err := (&Plugin{}).onAfterAggregate(&plugin.HookContext{
		Provider: fakeProvider{writer: writer},
		Aggregate: schema.HelmCIResultSet{
			Command: "plan",
			Results: []schema.HelmCIResult{
				{
					Stack: "dev", Component: "api", Processed: true,
					Summary: map[string]any{"diff": "+ change\n```\nnot summary Markdown"},
				},
				{
					Stack: "dev", Component: "worker", Status: "failed",
					Error: "render failed\n```\nnot summary Markdown",
				},
			},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, writer.summary, "````diff\n+ change\n```\nnot summary Markdown\n````")
	assert.Contains(t, writer.summary, "````text\nrender failed\n```\nnot summary Markdown\n````")
}

func TestOnAfterAggregateSkipsInvalidOrDisabledAndReturnsWriterError(t *testing.T) {
	pluginUnderTest := &Plugin{}
	require.NoError(t, pluginUnderTest.onAfterAggregate(&plugin.HookContext{Provider: fakeProvider{}, Aggregate: "invalid"}))
	require.NoError(t, pluginUnderTest.onAfterAggregate(&plugin.HookContext{
		Provider:  fakeProvider{},
		Aggregate: schema.HelmCIResultSet{},
	}))

	disabled := false
	writer := &fakeWriter{}
	require.NoError(t, pluginUnderTest.onAfterAggregate(&plugin.HookContext{
		Config:    &schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &disabled}}},
		Provider:  fakeProvider{writer: writer},
		Aggregate: schema.HelmCIResultSet{Results: []schema.HelmCIResult{{Processed: true}}},
	}))
	assert.Empty(t, writer.summary)

	sentinel := errors.New("write failed")
	err := pluginUnderTest.onAfterAggregate(&plugin.HookContext{
		Provider:  fakeProvider{writer: &fakeWriter{err: sentinel}},
		Aggregate: schema.HelmCIResultSet{Results: []schema.HelmCIResult{{Processed: true}}},
	})
	require.ErrorIs(t, err, sentinel)
}

func TestOnAfterAggregateRendersFailureWithoutResults(t *testing.T) {
	writer := &fakeWriter{}
	err := (&Plugin{}).onAfterAggregate(&plugin.HookContext{
		Provider:     fakeProvider{writer: writer},
		Aggregate:    schema.HelmCIResultSet{Command: "apply"},
		CommandError: errors.New("dependency graph contains a cycle"),
		ExitCode:     1,
	})
	require.NoError(t, err)
	assert.Contains(t, writer.summary, "## Helm Apply Summary")
	assert.Contains(t, writer.summary, "Command failed before any components were processed.")
	assert.Contains(t, writer.summary, "dependency graph contains a cycle")
}

func TestHelmAggregateHelpers(t *testing.T) {
	resultSet := schema.HelmCIResultSet{Command: "diff"}
	assert.Equal(t, resultSet, mustNormalizeHelmAggregate(t, resultSet))
	assert.Equal(t, resultSet, mustNormalizeHelmAggregate(t, &resultSet))
	_, ok := normalizeHelmAggregate((*schema.HelmCIResultSet)(nil))
	assert.False(t, ok)
	_, ok = normalizeHelmAggregate("invalid")
	assert.False(t, ok)

	assert.Equal(t, "plan", normalizeHelmAggregateCommand("diff"))
	assert.Equal(t, "apply", normalizeHelmAggregateCommand("deploy"))
	assert.Equal(t, "-", formatHelmAggregateDuration(0))
	assert.Equal(t, "15ms", formatHelmAggregateDuration(15))
	assert.Equal(t, `a\|b c`, helmMarkdownCell("a|b\nc"))

	oversized := strings.Repeat("x", helmAggregateMarkdownMaxBytes+100)
	truncated := enforceHelmAggregateMarkdownLimit(oversized)
	assert.LessOrEqual(t, len(truncated), helmAggregateMarkdownMaxBytes)
	assert.Contains(t, truncated, "Summary truncated")

	unicodeOversized := strings.Repeat("界", helmAggregateMarkdownMaxBytes)
	unicodeTruncated := enforceHelmAggregateMarkdownLimit(unicodeOversized)
	assert.True(t, utf8.ValidString(unicodeTruncated))
}

func mustNormalizeHelmAggregate(t *testing.T, value any) schema.HelmCIResultSet {
	t.Helper()
	result, ok := normalizeHelmAggregate(value)
	require.True(t, ok)
	return result
}
