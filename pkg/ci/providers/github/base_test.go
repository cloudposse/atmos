package github

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				"sha": "basesha123456789012345678901234567890ab",
			},
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	// merge-base may or may not succeed depending on git state in test env.
	// Either way, we MUST end up with a SHA — never the origin/<target> tip
	// ref, which is the path that produces false positives for out-of-date PRs.
	assert.Equal(t, "pull_request", res.EventType)
	assert.Equal(t, "headsha123456789012345678901234567890ab", res.HeadSHA)
	assert.Equal(t, "main", res.TargetBranch)
	assert.NotEmpty(t, res.SHA, "must populate a SHA — payload base.sha is the worst-case fallback")
	assert.Empty(t, res.Ref, "must not fall back to refs/remotes/origin/<target> (compares to current tip and causes false positives for out-of-date PRs)")
}

// TestResolveBase_PullRequest_OutOfDate_FallsBackToPayloadSHA covers the
// customer-reported scenario at the unit-test level: merge-base fails (no
// `origin/<target>` locally, shallow clone) and the PR is out of date with
// the target branch. The fix must NOT fall back to
// `refs/remotes/origin/<target>` (which compares to current tip and
// includes every commit on `<target>` since the fork point as a "diff").
// Instead we use `event.pull_request.base.sha`, which is at worst stale
// by however many `<target>` commits have landed since the last PR sync.
//
// We use a deliberately non-existent branch name so the test is
// deterministic regardless of the host repo's branch state. The bug
// pattern is identical for any target branch.
func TestResolveBase_PullRequest_OutOfDate_FallsBackToPayloadSHA(t *testing.T) {
	const target = "nonexistent-target-for-pr2380"

	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", target)

	eventPayload := map[string]any{
		"action": "synchronize",
		"pull_request": map[string]any{
			"head": map[string]any{
				"sha": "headsha123456789012345678901234567890ab",
			},
			"base": map[string]any{
				"ref": target,
				"sha": "stalebasesha789012345678901234567890abcd",
			},
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotEmpty(t, res.SHA)
	assert.Empty(t, res.Ref, "must not return a ref to current tip of target branch — that path causes false positives")
	assert.Equal(t, "stalebasesha789012345678901234567890abcd", res.SHA)
	assert.Equal(t, "event.pull_request.base.sha", res.Source)
	assert.Equal(t, target, res.TargetBranch)
}

// TestResolveBase_PullRequest_NoPayloadBaseSHA_LastResortRef confirms the
// last-resort branch still works when the event payload has no base.sha
// (degenerate or hand-crafted payloads). We do still log a warning, but
// the resolution is non-empty so callers can proceed.
//
// Uses a non-existent target branch so merge-base cannot succeed and we
// reach the last-resort path deterministically.
func TestResolveBase_PullRequest_NoPayloadBaseSHA_LastResortRef(t *testing.T) {
	const target = "nonexistent-target-for-pr2380"

	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", target)

	eventPayload := map[string]any{
		"action": "synchronize",
		"pull_request": map[string]any{
			"head": map[string]any{
				"sha": "headsha123456789012345678901234567890ab",
			},
			"base": map[string]any{
				"ref": target,
				// NOTE: no "sha" field — degenerate payload.
			},
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	// Last-resort path: ref to current tip. Caller has been warned.
	assert.Equal(t, "refs/remotes/origin/"+target, res.Ref)
	assert.Empty(t, res.SHA)
	assert.Equal(t, target, res.TargetBranch)
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
	// merge-base may fail (no origin/develop in test), falls back to ref.
	if res.SHA == "" {
		assert.Equal(t, "refs/remotes/origin/develop", res.Ref)
	}
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
	// In test env, merge-base and HEAD~1 may or may not work; any of the
	// three documented fallbacks produces a valid resolution. Whichever
	// branch fires depends on the CI runner's checkout depth and whether
	// origin/<base> has been fetched: PR runs typically reach merge-base
	// or HEAD~1, but post-merge runs on main commonly hit the payload-SHA
	// fallback, where Source = "event.pull_request.base.sha".
	//
	// Bug history: the original assertion was
	//   assert.Contains(t, res.Source, "merge-base", "HEAD~1")
	// which testify treats as a single Contains check with "HEAD~1" as
	// the failure-message argument -- it never matched the payload-SHA
	// path. Symptom: green on the PR (HEAD~1 reachable), red on main
	// (only the payload SHA is available).
	if res.SHA != "" {
		validSources := []string{
			"merge-base",
			"HEAD~1",
			"event.pull_request.base.sha",
		}
		matched := false
		for _, want := range validSources {
			if strings.Contains(res.Source, want) {
				matched = true
				break
			}
		}
		assert.True(t, matched,
			"Source %q must contain one of %v", res.Source, validSources)
	} else {
		assert.Equal(t, "refs/remotes/origin/main", res.Ref)
	}
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
	// Falls back to GITHUB_BASE_REF in test env.
	if res.SHA == "" {
		assert.Equal(t, "refs/remotes/origin/main", res.Ref)
	}
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

// TestResolveBase_MergeGroup verifies that a merge_group event with a full
// payload resolves to event.merge_group.base_sha (the target-branch commit
// the synthetic merge commit was built on top of), populates HeadSHA from
// merge_group.head_sha, and strips refs/heads/ from base_ref to derive the
// target branch.
func TestResolveBase_MergeGroup(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "merge_group")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"action": "checks_requested",
		"merge_group": map[string]any{
			"base_sha": "basesha123456789012345678901234567890ab",
			"head_sha": "synthsha123456789012345678901234567890ab",
			"base_ref": "refs/heads/main",
			"head_ref": "refs/heads/gh-readonly-queue/main/pr-42-headsha",
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "merge_group", res.EventType)
	assert.Equal(t, "basesha123456789012345678901234567890ab", res.SHA)
	assert.Equal(t, "synthsha123456789012345678901234567890ab", res.HeadSHA)
	assert.Equal(t, "main", res.TargetBranch)
	assert.Equal(t, "event.merge_group.base_sha", res.Source)
	assert.Empty(t, res.Ref, "should resolve to a SHA, not fall back to a ref")
}

// TestResolveBase_MergeGroup_PayloadHeadSHAOnly verifies that when the payload
// has head_sha but no base_sha (an unlikely-but-possible truncated payload),
// we fall back to env-based ref resolution while still populating HeadSHA.
func TestResolveBase_MergeGroup_PayloadHeadSHAOnly(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "merge_group")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"merge_group": map[string]any{
			"head_sha": "synthsha123456789012345678901234567890ab",
			"base_ref": "refs/heads/main",
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "merge_group", res.EventType)
	assert.Equal(t, "refs/remotes/origin/main", res.Ref, "no base_sha → fall back to ref")
	assert.Equal(t, "synthsha123456789012345678901234567890ab", res.HeadSHA)
	assert.Equal(t, "main", res.TargetBranch)
}

// TestResolveBase_MergeGroup_NoEventPath verifies graceful degradation when
// GITHUB_EVENT_PATH is unset (e.g., test environments without a real event
// file). We must not error out — fall back to env-only resolution.
func TestResolveBase_MergeGroup_NoEventPath(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "merge_group")
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_BASE_REF", "main")

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "merge_group", res.EventType)
	assert.Equal(t, "refs/remotes/origin/main", res.Ref)
	assert.Equal(t, "GITHUB_BASE_REF", res.Source)
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

// TestResolveBase_MergeGroup_NoBaseRef verifies that with no event payload
// and no GITHUB_BASE_REF, we still gracefully degrade to the default ref
// rather than erroring out.
func TestResolveBase_MergeGroup_NoBaseRef(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "merge_group")
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_EVENT_PATH", "")

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/HEAD", res.Ref)
}

// TestResolveBase_MergeGroup_PayloadBaseRefNoEnv verifies that when the payload
// supplies merge_group.base_ref but GITHUB_BASE_REF is empty (and base_sha is
// absent), the payload base_ref is promoted to res.Ref instead of defaulting
// to refs/remotes/origin/HEAD.
func TestResolveBase_MergeGroup_PayloadBaseRefNoEnv(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "merge_group")
	t.Setenv("GITHUB_BASE_REF", "")

	eventPayload := map[string]any{
		"merge_group": map[string]any{
			"head_sha": "synthsha123456789012345678901234567890ab",
			"base_ref": "refs/heads/main",
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "merge_group", res.EventType)
	assert.Equal(t, "refs/remotes/origin/main", res.Ref, "payload base_ref should be promoted when env is empty")
	assert.Equal(t, "event.merge_group.base_ref", res.Source)
	assert.Equal(t, "synthsha123456789012345678901234567890ab", res.HeadSHA)
	assert.Equal(t, "main", res.TargetBranch)
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
	// merge-base fails (no target in payload, falls back to GITHUB_BASE_REF for target,
	// but origin/develop doesn't exist in test env). HEAD~1 may or may not work.
	if res.SHA != "" {
		// HEAD~1 succeeded or merge-base succeeded.
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

// runGit runs a git command in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}

// runGitOutput runs a git command in dir and returns trimmed stdout.
func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.Output()
	require.NoError(t, err, "git %v failed", args)
	return strings.TrimSpace(string(out))
}

// TestResolveBase_PullRequest_Merged_UsesMergeCommitParent reproduces the
// production incident at the unit-test level: the workflow's checkout was
// not pinned to pull_request.head.sha, so HEAD ends up on the target
// branch's post-merge tip (the merge commit itself) instead of the PR's
// original branch. Without the fix, this degenerates tier 1's
// merge-base(HEAD, origin/<target>) into a self-referential, meaningless
// base. The fix must instead resolve merge_commit_sha^1 -- the merge
// commit's first parent, which is always the target branch's pre-merge
// tip (verified: `git merge --no-ff` always parents a merge commit as
// [previous-tip-of-current-branch, merged-branch-tip], never the fork
// point) -- giving downstream merge-base-aware diffing a real commit on
// the target branch's mainline to work from, instead of HEAD itself.
func TestResolveBase_PullRequest_Merged_UsesMergeCommitParent(t *testing.T) {
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-q", "-b", "main")
	runGit(t, repoDir, "config", "user.name", "Test")
	runGit(t, repoDir, "config", "user.email", "test@test.com")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "a.txt"), []byte("a"), 0o644))
	runGit(t, repoDir, "add", "a.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "A: initial commit")

	runGit(t, repoDir, "checkout", "-q", "-b", "pr-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "f1.txt"), []byte("f1"), 0o644))
	runGit(t, repoDir, "add", "f1.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "F1: the PR's own change")
	prHeadSHA := runGitOutput(t, repoDir, "rev-parse", "HEAD")

	runGit(t, repoDir, "checkout", "-q", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "b.txt"), []byte("b"), 0o644))
	runGit(t, repoDir, "add", "b.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "B: unrelated commit landed on main after the PR branch was cut")
	mainTipBeforeMergeSHA := runGitOutput(t, repoDir, "rev-parse", "HEAD")

	runGit(t, repoDir, "merge", "-q", "--no-ff", "pr-branch", "-m", "Merge pull request from pr-branch")
	mergeCommitSHA := runGitOutput(t, repoDir, "rev-parse", "HEAD")

	// HEAD is now the merge commit on main -- reproduces "checkout already
	// at post-merge HEAD."
	t.Chdir(repoDir)

	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"merged": true,
			"head": map[string]any{
				"sha": prHeadSHA,
			},
			"base": map[string]any{
				"ref": "main",
			},
			"merge_commit_sha": mergeCommitSHA,
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, mainTipBeforeMergeSHA, res.SHA, "must resolve to the merge commit's first parent")
	assert.NotEqual(t, mergeCommitSHA, res.SHA, "must not be the degenerate merge commit itself (== checked-out HEAD)")
	assert.Equal(t, sourceMergeCommitParent, res.Source)
	assert.Equal(t, prHeadSHA, res.HeadSHA)
	assert.Equal(t, "main", res.TargetBranch)
	assert.Equal(t, "pull_request", res.EventType)
}

// TestResolveBase_PullRequest_Merged_NoMergeCommitSHA_FallsThroughUnchanged
// is a regression guard: when the payload has no merge_commit_sha (e.g. an
// older GitHub Actions runner, or a hand-crafted payload), tier 0 must be a
// no-op and resolution must fall through to the pre-existing chain exactly
// as it did before this fix -- mirrors TestResolveBase_PullRequest_Closed.
func TestResolveBase_PullRequest_Merged_NoMergeCommitSHA_FallsThroughUnchanged(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")

	eventPayload := map[string]any{
		"action": "closed",
		"pull_request": map[string]any{
			"merged": true,
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
	assert.NotContains(t, res.Source, "merge_commit_sha", "tier 0 must not fire when the payload has no merge_commit_sha")
	// Same nondeterministic-but-bounded assertion as TestResolveBase_PullRequest_Closed:
	// in the test environment, merge-base and HEAD~1 may or may not work.
	if res.SHA != "" {
		validSources := []string{
			"merge-base",
			"HEAD~1",
			"event.pull_request.base.sha",
		}
		matched := false
		for _, want := range validSources {
			if strings.Contains(res.Source, want) {
				matched = true
				break
			}
		}
		assert.True(t, matched,
			"Source %q must contain one of %v", res.Source, validSources)
	} else {
		assert.Equal(t, "refs/remotes/origin/main", res.Ref)
	}
}

// TestResolveBase_PullRequest_OpenedOrSynchronize_Unaffected is a direct
// regression guard: tier 0 must only ever run for action == "closed". Even
// if a payload somehow carries "merged": true and a merge_commit_sha on a
// "synchronize" action (which shouldn't happen in real GitHub payloads, but
// nothing stops a malformed/replayed event from having it), it must be
// ignored. Uses a nonexistent target branch so tier 1 (merge-base) is
// guaranteed to fail and tier 3 (payload base.sha) fires deterministically.
func TestResolveBase_PullRequest_OpenedOrSynchronize_Unaffected(t *testing.T) {
	const target = "nonexistent-target-for-merged-pr-tier0-guard"

	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", target)

	eventPayload := map[string]any{
		"action": "synchronize",
		"pull_request": map[string]any{
			"merged": true,
			"head": map[string]any{
				"sha": "headsha123456789012345678901234567890ab",
			},
			"base": map[string]any{
				"ref": target,
				"sha": "stalebasesha789012345678901234567890abcd",
			},
			"merge_commit_sha": "shouldneverbeusedsha123456789012345678ab",
		},
	}
	eventPath := writeEventPayload(t, eventPayload)
	t.Setenv("GITHUB_EVENT_PATH", eventPath)

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "stalebasesha789012345678901234567890abcd", res.SHA)
	assert.Equal(t, "event.pull_request.base.sha", res.Source, "tier 0 must never fire for a non-closed action")
	assert.NotContains(t, res.Source, "merge_commit_sha")
}
