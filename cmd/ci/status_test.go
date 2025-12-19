package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ci"
)

func TestTruncateSHA(t *testing.T) {
	tests := []struct {
		name     string
		sha      string
		expected string
	}{
		{
			name:     "full SHA",
			sha:      "abc1234567890def1234567890abcdef12345678",
			expected: "abc1234",
		},
		{
			name:     "short SHA",
			sha:      "abc",
			expected: "abc",
		},
		{
			name:     "exact 7 chars",
			sha:      "abc1234",
			expected: "abc1234",
		},
		{
			name:     "empty",
			sha:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateSHA(tt.sha)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCheckIcon(t *testing.T) {
	tests := []struct {
		name     string
		state    ci.CheckStatusState
		expected string
	}{
		{
			name:     "success",
			state:    ci.CheckStatusStateSuccess,
			expected: "\u2713",
		},
		{
			name:     "failure",
			state:    ci.CheckStatusStateFailure,
			expected: "\u2717",
		},
		{
			name:     "pending",
			state:    ci.CheckStatusStatePending,
			expected: "\u25CB",
		},
		{
			name:     "cancelled",
			state:    ci.CheckStatusStateCancelled,
			expected: "\u25CF",
		},
		{
			name:     "skipped",
			state:    ci.CheckStatusStateSkipped,
			expected: "\u2212",
		},
		{
			name:     "unknown",
			state:    ci.CheckStatusState("unknown"),
			expected: "?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCheckIcon(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Note: TestGetDefaultProvider was removed because it was non-deterministic.
// The function depends on the global CI registry state which can vary based on
// init() ordering and package imports. Testing would require mocking the registry,
// which isn't possible from this package as the registry is internal to pkg/ci.

func TestRepoContext(t *testing.T) {
	t.Run("struct fields", func(t *testing.T) {
		ctx := &repoContext{
			Owner:  "testowner",
			Repo:   "testrepo",
			Branch: "main",
			SHA:    "abc123",
		}

		assert.Equal(t, "testowner", ctx.Owner)
		assert.Equal(t, "testrepo", ctx.Repo)
		assert.Equal(t, "main", ctx.Branch)
		assert.Equal(t, "abc123", ctx.SHA)
	})
}

// Note: Testing runStatus, getRepoContext, and rendering functions requires
// extensive mocking of:
// - CI provider registry
// - Git repository operations
// - UI output capturing
//
// These are better suited for integration tests or tests with a full mock setup.
// The helper functions above provide coverage for the pure logic.

func TestRenderFunctions(t *testing.T) {
	// These tests verify the functions don't panic with various inputs.
	// Actual output verification would require UI mocking.

	t.Run("renderStatus with empty data", func(t *testing.T) {
		status := &ci.Status{
			Repository: "owner/repo",
		}
		// Should not panic.
		assert.NotPanics(t, func() {
			renderStatus(status)
		})
	})

	t.Run("renderStatus with current branch", func(t *testing.T) {
		status := &ci.Status{
			Repository: "owner/repo",
			CurrentBranch: &ci.BranchStatus{
				Branch:    "main",
				CommitSHA: "abc123",
				Checks:    []*ci.CheckStatus{},
			},
		}
		assert.NotPanics(t, func() {
			renderStatus(status)
		})
	})

	t.Run("renderStatus with PRs", func(t *testing.T) {
		status := &ci.Status{
			Repository: "owner/repo",
			CreatedByUser: []*ci.PRStatus{
				{
					Number: 1,
					Title:  "Test PR",
					Branch: "feature",
				},
			},
			ReviewRequests: []*ci.PRStatus{
				{
					Number: 2,
					Title:  "Review PR",
					Branch: "fix",
				},
			},
		}
		assert.NotPanics(t, func() {
			renderStatus(status)
		})
	})

	t.Run("renderBranchStatus with PR", func(t *testing.T) {
		bs := &ci.BranchStatus{
			Branch:    "feature",
			CommitSHA: "def456",
			PullRequest: &ci.PRStatus{
				Number:    42,
				Title:     "Feature PR",
				Branch:    "feature",
				AllPassed: true,
				Checks:    []*ci.CheckStatus{},
			},
		}
		assert.NotPanics(t, func() {
			renderBranchStatus(bs)
		})
	})

	t.Run("renderBranchStatus without PR", func(t *testing.T) {
		bs := &ci.BranchStatus{
			Branch:    "main",
			CommitSHA: "abc123",
			Checks: []*ci.CheckStatus{
				{Name: "build", Status: "completed", Conclusion: "success"},
			},
		}
		assert.NotPanics(t, func() {
			renderBranchStatus(bs)
		})
	})

	t.Run("renderPRStatus", func(t *testing.T) {
		pr := &ci.PRStatus{
			Number:    99,
			Title:     "Big Feature",
			Branch:    "big-feature",
			AllPassed: false,
			Checks: []*ci.CheckStatus{
				{Name: "test", Status: "completed", Conclusion: "failure"},
			},
		}
		assert.NotPanics(t, func() {
			renderPRStatus(pr)
		})
	})

	t.Run("renderChecks", func(t *testing.T) {
		checks := []*ci.CheckStatus{
			{Name: "build", Status: "completed", Conclusion: "success"},
			{Name: "test", Status: "in_progress", Conclusion: ""},
			{Name: "lint", Status: "completed", Conclusion: "failure"},
		}
		assert.NotPanics(t, func() {
			renderChecks(checks, "  ")
		})
	})
}
