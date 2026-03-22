package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBase_PullRequest_OpenSync(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"action": "synchronize",
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/main", res.Ref)
	assert.Empty(t, res.SHA)
	assert.Equal(t, "GITHUB_BASE_REF", res.Source)
	assert.Equal(t, "pull_request", res.EventType)
}

func TestResolveBase_PullRequest_Opened(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "develop")

	eventPayload := map[string]any{
		"action": "opened",
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/develop", res.Ref)
	assert.Empty(t, res.SHA)
}

func TestResolveBase_PullRequest_Closed(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"base": map[string]any{
				"sha": "abc123def456789012345678901234567890abcd",
			},
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Ref)
	assert.Equal(t, "abc123def456789012345678901234567890abcd", res.SHA)
	assert.Equal(t, "event.pull_request.base.sha", res.Source)
	assert.Equal(t, "pull_request", res.EventType)
}

func TestResolveBase_PullRequestTarget(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request_target")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"action": "opened",
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/main", res.Ref)
}

func TestResolveBase_Push_Normal(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "push")

	eventPayload := map[string]any{
		"before": "abc123def456789012345678901234567890abcd",
		"forced": false,
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Ref)
	assert.Equal(t, "abc123def456789012345678901234567890abcd", res.SHA)
	assert.Equal(t, "event.before", res.Source)
	assert.Equal(t, "push", res.EventType)
}

func TestResolveBase_Push_NewBranch(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "push")

	eventPayload := map[string]any{
		"before": "0000000000000000000000000000000000000000",
		"forced": false,
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/HEAD", res.Ref)
	assert.Contains(t, res.Source, "no before SHA")
}

func TestResolveBase_MergeGroup(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "merge_group")
	t.Setenv("GITHUB_BASE_REF", "main")

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/main", res.Ref)
	assert.Equal(t, "GITHUB_BASE_REF", res.Source)
	assert.Equal(t, "merge_group", res.EventType)
}

func TestResolveBase_UnknownEvent(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/HEAD", res.Ref)
	assert.Equal(t, "default", res.Source)
}

func TestResolveBase_MissingEventPath(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_EVENT_PATH", "")

	p := NewProvider()
	_, err := p.ResolveBase()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GITHUB_EVENT_PATH")
}

func TestResolveBase_MergeGroup_NoBaseRef(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "merge_group")
	t.Setenv("GITHUB_BASE_REF", "")

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/HEAD", res.Ref)
}

// writeEventPayload writes a JSON event payload to a temp file and returns the path.
func writeEventPayload(t *testing.T, payload map[string]any) string {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "event.json")
	err = os.WriteFile(path, data, 0o644)
	require.NoError(t, err)

	return path
}
