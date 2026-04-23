package reexec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDepth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"zero", "0", 0},
		{"one", "1", 1},
		{"two", "2", 2},
		{"ten", "10", 10},
		{"negative treated as 0", "-1", 0},
		{"non-numeric treated as 0", "abc", 0},
		{"whitespace treated as 0", " 1 ", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Depth(tt.input))
		})
	}
}

func TestCurrentDepth(t *testing.T) {
	t.Run("unset defaults to 0", func(t *testing.T) {
		t.Setenv(DepthEnvVar, "")
		assert.Equal(t, 0, CurrentDepth())
	})

	t.Run("reads set value", func(t *testing.T) {
		t.Setenv(DepthEnvVar, "3")
		assert.Equal(t, 3, CurrentDepth())
	})
}

func TestNextEnv(t *testing.T) {
	tests := []struct {
		name              string
		input             []string
		wantContainsFinal string // Expected final DepthEnvVar= entry.
		wantDepthCount    int    // Expected number of DepthEnvVar= entries.
	}{
		{
			name:              "no existing depth adds =1",
			input:             []string{"PATH=/usr/bin", "HOME=/home/user"},
			wantContainsFinal: "ATMOS_REEXEC_DEPTH=1",
			wantDepthCount:    1,
		},
		{
			name:              "existing depth=1 becomes 2",
			input:             []string{"PATH=/usr/bin", "ATMOS_REEXEC_DEPTH=1"},
			wantContainsFinal: "ATMOS_REEXEC_DEPTH=2",
			wantDepthCount:    1,
		},
		{
			name:              "existing depth=5 becomes 6",
			input:             []string{"ATMOS_REEXEC_DEPTH=5", "PATH=/usr/bin"},
			wantContainsFinal: "ATMOS_REEXEC_DEPTH=6",
			wantDepthCount:    1,
		},
		{
			name:              "duplicate depth entries collapsed, last wins",
			input:             []string{"ATMOS_REEXEC_DEPTH=1", "PATH=/usr/bin", "ATMOS_REEXEC_DEPTH=3"},
			wantContainsFinal: "ATMOS_REEXEC_DEPTH=4",
			wantDepthCount:    1,
		},
		{
			name:              "invalid value treated as 0",
			input:             []string{"ATMOS_REEXEC_DEPTH=notanumber", "PATH=/usr/bin"},
			wantContainsFinal: "ATMOS_REEXEC_DEPTH=1",
			wantDepthCount:    1,
		},
		{
			name:              "empty env produces single entry",
			input:             []string{},
			wantContainsFinal: "ATMOS_REEXEC_DEPTH=1",
			wantDepthCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NextEnv(tt.input)

			count := 0
			var lastDepth string
			for _, kv := range result {
				if len(kv) >= len(DepthEnvVar)+1 && kv[:len(DepthEnvVar)+1] == DepthEnvVar+"=" {
					count++
					lastDepth = kv
				}
			}
			assert.Equal(t, tt.wantDepthCount, count, "unexpected number of depth entries")
			assert.Equal(t, tt.wantContainsFinal, lastDepth, "unexpected final depth value")

			// Non-depth entries must be preserved in order.
			for _, kv := range tt.input {
				if len(kv) >= len(DepthEnvVar)+1 && kv[:len(DepthEnvVar)+1] == DepthEnvVar+"=" {
					continue
				}
				assert.Contains(t, result, kv, "non-depth entry must be preserved")
			}
		})
	}
}

func TestNextEnv_DoesNotMutateInput(t *testing.T) {
	input := []string{"PATH=/usr/bin", "ATMOS_REEXEC_DEPTH=1", "HOME=/home/user"}
	original := make([]string, len(input))
	copy(original, input)

	_ = NextEnv(input)

	assert.Equal(t, original, input, "NextEnv must not mutate its input")
}
