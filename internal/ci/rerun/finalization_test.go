package rerun

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyStalledFinalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Job)
		want   Class
	}{
		{name: "only Complete job is still running", want: ClassRunnerStuckAfterComplete},
		{
			name: "gap exactly at threshold",
			mutate: func(j *Job) {
				j.CompletedAt = at(t9m.Add(DefaultStuckGap))
			},
			want: ClassRunnerStuckAfterComplete,
		},
		{
			name: "recent finalization despite an old last completed step",
			mutate: func(j *Job) {
				j.Steps[3].StartedAt = at(t43m.Add(-DefaultStuckGap + time.Second))
			},
			want: ClassSuperseded,
		},
		{
			name: "test still running",
			mutate: func(j *Job) {
				j.Steps[1] = step("Acceptance tests", "in_progress", "", nil)
			},
			want: ClassSuperseded,
		},
		{
			name:   "test failed",
			mutate: func(j *Job) { j.Steps[1].Conclusion = conclusionFailure },
			want:   ClassSuperseded,
		},
		{
			name: "post action still running",
			mutate: func(j *Job) {
				j.Steps[2] = step("Post Harden Runner", "in_progress", "", nil)
			},
			want: ClassSuperseded,
		},
		{
			name:   "another final step still running",
			mutate: func(j *Job) { j.Steps[3].Name = "Upload artifact" },
			want:   ClassSuperseded,
		},
		{
			name:   "finalization has a cancellation conclusion",
			mutate: func(j *Job) { j.Steps[3].Conclusion = conclusionCancelled },
			want:   ClassSuperseded,
		},
		{
			name:   "finalization has no start time",
			mutate: func(j *Job) { j.Steps[3].StartedAt = nil },
			want:   ClassSuperseded,
		},
		{
			name:   "job has no completion time",
			mutate: func(j *Job) { j.CompletedAt = nil },
			want:   ClassSuperseded,
		},
		{
			name:   "finalization has a completion time despite running status",
			mutate: func(j *Job) { j.Steps[3].CompletedAt = at(t9m) },
			want:   ClassSuperseded,
		},
		{
			name:   "no preceding work",
			mutate: func(j *Job) { j.Steps = j.Steps[3:] },
			want:   ClassSuperseded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			steps := greenSteps()
			steps[3] = step("Complete job", "in_progress", "", nil)
			steps[3].StartedAt = at(t9m)
			j := job("Acceptance Tests (macos, shard 2/10)", conclusionCancelled, at(t43m), steps)
			if tt.mutate != nil {
				tt.mutate(&j)
			}
			got := Classify([]Job{j}, Options{})
			require.Len(t, got, 1)
			assert.Equal(t, tt.want, got[0].Class)
		})
	}
}
