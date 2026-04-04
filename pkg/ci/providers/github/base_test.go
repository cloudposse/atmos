package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var gitMergeBaseOriginal = mergeBaseResolver

func TestResolveBase_PullRequest_OpenSync(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"action": "synchronize",
		"pull_request": map[string]any{
			"head": map[string]any{
				"sha": "headsha123456789012345678901234567890ab",
			},
			"base": map[string]any{
				"ref": "main",
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
	assert.Equal(t, "pull_request", res.EventType)
	assert.Equal(t, "headsha123456789012345678901234567890ab", res.HeadSHA)
	assert.Equal(t, "abc123def456789012345678901234567890abcd", res.SHA)
	assert.Empty(t, res.Ref)
	assert.Contains(t, res.Source, "event.pull_request.base.sha")
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
	// For merged PRs: tries merge-base → HEAD~1 → GITHUB_BASE_REF.
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"head": map[string]any{
				"sha": "headsha123456789012345678901234567890ab",
			},
			"base": map[string]any{
				"ref": "main",
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
	assert.Equal(t, "pull_request", res.EventType)
	assert.Equal(t, "headsha123456789012345678901234567890ab", res.HeadSHA)
	assert.Equal(t, "abc123def456789012345678901234567890abcd", res.SHA)
	assert.Empty(t, res.Ref)
	assert.Contains(t, res.Source, "event.pull_request.base.sha")
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
	assert.Empty(t, res.SHA)
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

// TestReadEventPayload tests the readEventPayload helper function.
func TestReadEventPayload(t *testing.T) {
	t.Run("missing GITHUB_EVENT_PATH", func(t *testing.T) {
		t.Setenv("GITHUB_EVENT_PATH", "")
		_, err := readEventPayload()
		assert.ErrorIs(t, err, ErrEventPathNotSet)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Setenv("GITHUB_EVENT_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))
		_, err := readEventPayload()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reading")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		err := os.WriteFile(path, []byte("not json"), 0o644)
		require.NoError(t, err)
		t.Setenv("GITHUB_EVENT_PATH", path)

		_, err = readEventPayload()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parsing event payload")
	})

	t.Run("valid JSON", func(t *testing.T) {
		payload := map[string]any{"action": "opened", "number": float64(42)}
		path := writeEventPayload(t, payload)
		t.Setenv("GITHUB_EVENT_PATH", path)

		result, err := readEventPayload()
		require.NoError(t, err)
		assert.Equal(t, "opened", result["action"])
		assert.Equal(t, float64(42), result["number"])
	})
}

// TestResolveBase_Push_ForcePush tests force push scenarios.
func TestResolveBase_Push_ForcePush(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "push")

	eventPayload := map[string]any{
		"before": "abc123def456789012345678901234567890abcd",
		"forced": true,
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "push", res.EventType)
	assert.Contains(t, res.Source, "force-push")

	// In a real git repo, resolveParentCommit succeeds and returns HEAD~1 SHA.
	// In CI without a repo, it falls back to origin/HEAD ref.
	if res.SHA != "" {
		assert.Len(t, res.SHA, 40, "should be a full SHA")
		assert.Equal(t, "HEAD~1 (force-push)", res.Source)
	} else {
		assert.Equal(t, "refs/remotes/origin/HEAD", res.Ref)
		assert.Contains(t, res.Source, "HEAD~1 failed")
	}
}

// TestResolveBase_PullRequest_Closed_FallbackToBaseRef tests the fallback path.
func TestResolveBase_PullRequest_Closed_FallbackToBaseRef(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "develop")

	eventPayload := map[string]any{
		"action":       "closed",
		"pull_request": map[string]any{},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	if res.SHA != "" {
		assert.NotEmpty(t, res.Source)
	} else {
		assert.Equal(t, "refs/remotes/origin/develop", res.Ref)
	}
}

// TestResolveBase_Push_EmptyBefore tests push with empty before SHA.
func TestResolveBase_Push_EmptyBefore(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "push")

	eventPayload := map[string]any{
		"before": "",
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

// TestResolveFromBaseRef tests resolveFromBaseRef with various inputs.
func TestResolveFromBaseRef(t *testing.T) {
	t.Run("empty GITHUB_BASE_REF", func(t *testing.T) {
		t.Setenv("GITHUB_BASE_REF", "")
		res := resolveFromBaseRef("pull_request")
		assert.Equal(t, "refs/remotes/origin/HEAD", res.Ref)
		assert.Contains(t, res.Source, "GITHUB_BASE_REF empty")
	})

	t.Run("set GITHUB_BASE_REF", func(t *testing.T) {
		t.Setenv("GITHUB_BASE_REF", "feature-branch")
		res := resolveFromBaseRef("pull_request")
		assert.Equal(t, "refs/remotes/origin/feature-branch", res.Ref)
		assert.Equal(t, "GITHUB_BASE_REF", res.Source)
	})
}

// TestExtractPRHeadSHA tests the extractPRHeadSHA helper function.
func TestExtractPRHeadSHA(t *testing.T) {
	t.Run("valid head SHA", func(t *testing.T) {
		payload := map[string]any{
			"pull_request": map[string]any{
				"head": map[string]any{
					"sha": "abc123def456789012345678901234567890abcd",
				},
			},
		}
		sha := extractPRHeadSHA(payload)
		assert.Equal(t, "abc123def456789012345678901234567890abcd", sha)
	})

	t.Run("missing pull_request key", func(t *testing.T) {
		payload := map[string]any{"action": "opened"}
		sha := extractPRHeadSHA(payload)
		assert.Empty(t, sha)
	})

	t.Run("missing head key", func(t *testing.T) {
		payload := map[string]any{
			"pull_request": map[string]any{
				"base": map[string]any{"sha": "abc123"},
			},
		}
		sha := extractPRHeadSHA(payload)
		assert.Empty(t, sha)
	})

	t.Run("empty head SHA", func(t *testing.T) {
		payload := map[string]any{
			"pull_request": map[string]any{
				"head": map[string]any{"sha": ""},
			},
		}
		sha := extractPRHeadSHA(payload)
		assert.Empty(t, sha)
	})
}

// TestExtractTargetBranch tests the extractTargetBranch helper function.
func TestExtractTargetBranch(t *testing.T) {
	t.Run("from payload", func(t *testing.T) {
		payload := map[string]any{
			"pull_request": map[string]any{
				"base": map[string]any{
					"ref": "main",
				},
			},
		}
		branch := extractTargetBranch(payload)
		assert.Equal(t, "main", branch)
	})

	t.Run("missing pull_request falls back to env", func(t *testing.T) {
		t.Setenv("GITHUB_BASE_REF", "develop")
		payload := map[string]any{"action": "opened"}
		branch := extractTargetBranch(payload)
		assert.Equal(t, "develop", branch)
	})

	t.Run("missing base falls back to env", func(t *testing.T) {
		t.Setenv("GITHUB_BASE_REF", "staging")
		payload := map[string]any{
			"pull_request": map[string]any{
				"head": map[string]any{"sha": "abc123"},
			},
		}
		branch := extractTargetBranch(payload)
		assert.Equal(t, "staging", branch)
	})

	t.Run("empty ref falls back to env", func(t *testing.T) {
		t.Setenv("GITHUB_BASE_REF", "release")
		payload := map[string]any{
			"pull_request": map[string]any{
				"base": map[string]any{"ref": ""},
			},
		}
		branch := extractTargetBranch(payload)
		assert.Equal(t, "release", branch)
	})

	t.Run("no payload and no env", func(t *testing.T) {
		t.Setenv("GITHUB_BASE_REF", "")
		payload := map[string]any{"action": "opened"}
		branch := extractTargetBranch(payload)
		assert.Empty(t, branch)
	})
}

// TestResolveBase_Push_HeadSHA_Empty verifies that push events do not populate HeadSHA.
func TestResolveBase_Push_HeadSHA_Empty(t *testing.T) {
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
	assert.Empty(t, res.HeadSHA, "push events should not populate HeadSHA")
}

// TestResolveBase_PullRequest_OpenSync_NoHeadInPayload verifies fallback when head SHA is missing from PR payload.
func TestResolveBase_PullRequest_OpenSync_NoHeadInPayload(t *testing.T) {
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
	assert.Empty(t, res.HeadSHA, "should be empty when pull_request.head.sha is missing from payload")
}

func TestResolveBase_PullRequest_UsesPayloadBaseSHAWhenMergeBaseUnavailable(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")
	t.Cleanup(func() {
		mergeBaseResolver = gitMergeBaseOriginal
	})

	mergeBaseResolver = func(string) (string, error) {
		return "", assert.AnError
	}

	eventPayload := map[string]any{
		"action": "synchronize",
		"pull_request": map[string]any{
			"head": map[string]any{"sha": "headsha123456789012345678901234567890ab"},
			"base": map[string]any{
				"ref": "main",
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
	assert.Equal(t, "abc123def456789012345678901234567890abcd", res.SHA)
	assert.Contains(t, res.Source, "event.pull_request.base.sha")
	assert.Contains(t, res.Source, "merge-base unavailable")
}

func TestResolveBase_PullRequest_UsesMergeBaseWhenPayloadSHAMissing(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")
	t.Cleanup(func() {
		mergeBaseResolver = gitMergeBaseOriginal
	})

	mergeBaseResolver = func(string) (string, error) {
		return "feedfacefeedfacefeedfacefeedfacefeedface", nil
	}

	eventPayload := map[string]any{
		"action": "synchronize",
		"pull_request": map[string]any{
			"base": map[string]any{"ref": "main"},
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "feedfacefeedfacefeedfacefeedfacefeedface", res.SHA)
	assert.Equal(t, "merge-base(HEAD, origin/main)", res.Source)
}

func TestResolveBase_PullRequest_Closed_UsesParentWhenPayloadAndMergeBaseMissing(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")
	t.Cleanup(func() {
		mergeBaseResolver = gitMergeBaseOriginal
		parentCommitResolver = resolveParentCommit
	})

	mergeBaseResolver = func(string) (string, error) {
		return "", assert.AnError
	}
	parentCommitResolver = func() (string, error) {
		return "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", nil
	}

	eventPayload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"base": map[string]any{"ref": "main"},
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", res.SHA)
	assert.Equal(t, "HEAD~1 (merged PR, merge-base unavailable)", res.Source)
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
