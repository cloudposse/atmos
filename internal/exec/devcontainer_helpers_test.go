package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsContainerRunning validates the isContainerRunning status checking logic.
// This is a critical decision point used throughout devcontainer lifecycle management
// to determine whether containers need to be started, stopped, or are already in the
// desired state. The function must match exact status strings ("running", "Running", "Up")
// to avoid false positives with similar-looking statuses like "Up 5 minutes".
//
// The duplication is by design: each test suite validates different domain-specific status logic
// with its own set of expected values and edge cases. Consolidating these would reduce readability
// and make it harder to maintain domain-specific test scenarios independently.
//
//nolint:dupl // This table-driven test intentionally shares structure with other status validation tests.
func TestIsContainerRunning(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{
			name:     "lowercase running",
			status:   "running",
			expected: true,
		},
		{
			name:     "capitalized Running",
			status:   "Running",
			expected: true,
		},
		{
			name:     "status Up",
			status:   "Up",
			expected: true,
		},
		{
			name:     "status exited",
			status:   "exited",
			expected: false,
		},
		{
			name:     "status stopped",
			status:   "stopped",
			expected: false,
		},
		{
			name:     "status paused",
			status:   "paused",
			expected: false,
		},
		{
			name:     "status created",
			status:   "created",
			expected: false,
		},
		{
			name:     "empty status",
			status:   "",
			expected: false,
		},
		{
			name:     "uppercase RUNNING",
			status:   "RUNNING",
			expected: false,
		},
		{
			name:     "status with spaces",
			status:   " running ",
			expected: false,
		},
		{
			name:     "Docker status Up 5 minutes",
			status:   "Up 5 minutes",
			expected: false,
		},
		{
			name:     "exact match Up only",
			status:   "Up",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isContainerRunning(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}
