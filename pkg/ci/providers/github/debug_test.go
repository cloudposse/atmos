package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDebugMode(t *testing.T) {
	tests := []struct {
		name      string
		gha       string
		runnerDbg string
		stepDbg   string
		expected  bool
	}{
		{
			name:      "GHA + runner debug -> true",
			gha:       "true",
			runnerDbg: "true",
			expected:  true,
		},
		{
			name:     "GHA + step debug -> true",
			gha:      "true",
			stepDbg:  "true",
			expected: true,
		},
		{
			name:      "GHA + both debug vars -> true",
			gha:       "true",
			runnerDbg: "true",
			stepDbg:   "true",
			expected:  true,
		},
		{
			name:     "GHA + neither debug var -> false",
			gha:      "true",
			expected: false,
		},
		{
			name:      "GHA=false + runner debug -> false (not in GHA)",
			gha:       "false",
			runnerDbg: "true",
			expected:  false,
		},
		{
			name:     "GHA=false + step debug -> false (not in GHA)",
			gha:      "false",
			stepDbg:  "true",
			expected: false,
		},
		{
			name:      "GHA unset + runner debug -> false",
			runnerDbg: "true",
			expected:  false,
		},
		{
			name:     "GHA unset + step debug -> false",
			stepDbg:  "true",
			expected: false,
		},
		{
			name:      "GHA=true + ACTIONS_RUNNER_DEBUG=1 (not literal 'true') -> false",
			gha:       "true",
			runnerDbg: "1",
			expected:  false,
		},
		{
			name:     "GHA=true + ACTIONS_STEP_DEBUG=TRUE (uppercase) -> false (strict match)",
			gha:      "true",
			stepDbg:  "TRUE",
			expected: false,
		},
		{
			name:      "GHA=true + ACTIONS_RUNNER_DEBUG=false explicit -> false",
			gha:       "true",
			runnerDbg: "false",
			stepDbg:   "false",
			expected:  false,
		},
		{
			name:     "all unset -> false",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", tt.gha)
			t.Setenv("ACTIONS_RUNNER_DEBUG", tt.runnerDbg)
			t.Setenv("ACTIONS_STEP_DEBUG", tt.stepDbg)

			assert.Equal(t, tt.expected, (&Provider{}).IsDebugMode())
		})
	}
}
