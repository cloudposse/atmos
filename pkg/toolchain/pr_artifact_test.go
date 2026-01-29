package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPRVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		wantPR   int
		wantIsPR bool
	}{
		{
			name:     "valid PR version",
			version:  "pr:2038",
			wantPR:   2038,
			wantIsPR: true,
		},
		{
			name:     "valid PR version with large number",
			version:  "pr:12345",
			wantPR:   12345,
			wantIsPR: true,
		},
		{
			name:     "valid PR version with single digit",
			version:  "pr:1",
			wantPR:   1,
			wantIsPR: true,
		},
		{
			name:     "regular semver",
			version:  "1.2.3",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "latest keyword",
			version:  "latest",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "version with v prefix",
			version:  "v1.2.3",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr prefix without colon",
			version:  "pr2038",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "PR with uppercase (not supported)",
			version:  "PR:2038",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr with slash (not supported)",
			version:  "pr/2038",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr with invalid number",
			version:  "pr:abc",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr with zero",
			version:  "pr:0",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr with negative number",
			version:  "pr:-1",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "empty string",
			version:  "",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr colon only",
			version:  "pr:",
			wantPR:   0,
			wantIsPR: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPR, gotIsPR := IsPRVersion(tt.version)
			assert.Equal(t, tt.wantPR, gotPR, "PR number mismatch")
			assert.Equal(t, tt.wantIsPR, gotIsPR, "isPR mismatch")
		})
	}
}

func TestBuildTokenRequiredError(t *testing.T) {
	err := buildTokenRequiredError()
	assert.Error(t, err)
	// The error uses ErrAuthenticationFailed sentinel.
	assert.Contains(t, err.Error(), "authentication")
}

func TestHandlePRArtifactError(t *testing.T) {
	t.Run("generic error returns tool installation error", func(t *testing.T) {
		err := handlePRArtifactError(assert.AnError, 2038)
		assert.Error(t, err)
		// Generic errors are wrapped with ErrToolInstall.
		assert.Contains(t, err.Error(), "tool installation")
	})
}

// Note: Full integration tests for InstallFromPR require:
// - A valid GitHub token
// - Network access
// - A real PR with artifacts
// Those tests should be in a separate integration test file with appropriate
// skip conditions.
