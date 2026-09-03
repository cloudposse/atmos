package rerun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	t0   = time.Date(2026, 9, 3, 16, 0, 0, 0, time.UTC)
	t9m  = t0.Add(9*time.Minute + 36*time.Second)
	t43m = t0.Add(43*time.Minute + 17*time.Second)
)

func at(t time.Time) *time.Time { return &t }

func okStep(name string, done time.Time) Step {
	return Step{Name: name, Status: statusCompleted, Conclusion: conclusionSuccess, CompletedAt: at(done)}
}

func step(name, status, conclusion string, done *time.Time) Step {
	return Step{Name: name, Status: status, Conclusion: conclusion, CompletedAt: done}
}

// greenSteps mirrors a real stuck Windows build: every step, including
// `Complete job`, succeeded, with one fractional-second timestamp.
func greenSteps() []Step {
	return []Step{
		okStep("Set up job", t0),
		okStep("Build", t0.Add(9*time.Minute+30*time.Second+123*time.Millisecond)),
		step("Post Harden Runner", statusCompleted, conclusionSkipped, at(t0.Add(9*time.Minute+35*time.Second))),
		okStep("Complete job", t9m),
	}
}

func job(name, conclusion string, done *time.Time, steps []Step) Job {
	return Job{Name: name, Status: statusCompleted, Conclusion: conclusion, CompletedAt: done, Steps: steps}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		job  Job
		opts Options
		want Class
	}{
		{
			name: "cancelled with all steps green and reaped 34 min later is runner-stuck-after-complete",
			job:  job("Build (windows)", conclusionCancelled, at(t43m), greenSteps()),
			want: ClassRunnerStuckAfterComplete,
		},
		{
			name: "gap exactly at the threshold is runner-stuck-after-complete",
			job:  job("Build (windows)", conclusionCancelled, at(t9m.Add(DefaultStuckGap)), greenSteps()),
			want: ClassRunnerStuckAfterComplete,
		},
		{
			name: "gap one second under the threshold is superseded",
			job:  job("Build (windows)", conclusionCancelled, at(t9m.Add(DefaultStuckGap-time.Second)), greenSteps()),
			want: ClassSuperseded,
		},
		{
			name: "custom StuckGap lowers the threshold",
			job:  job("Build (windows)", conclusionCancelled, at(t9m.Add(4*time.Second)), greenSteps()),
			opts: Options{StuckGap: 3 * time.Second},
			want: ClassRunnerStuckAfterComplete,
		},
		{
			name: "custom StuckGap raises the threshold",
			job:  job("Build (windows)", conclusionCancelled, at(t9m.Add(4*time.Second)), greenSteps()),
			opts: Options{StuckGap: 10 * time.Second},
			want: ClassSuperseded,
		},
		{
			name: "cancelled mid-step is superseded",
			job: job("Acceptance Tests (linux, shard 2/10)", conclusionCancelled, at(t0.Add(5*time.Minute)), []Step{
				okStep("Set up job", t0),
				step("Acceptance tests", statusCompleted, conclusionCancelled, at(t0.Add(5*time.Minute))),
				step("Complete job", statusCompleted, conclusionSkipped, nil),
			}),
			want: ClassSuperseded,
		},
		{
			name: "cancelled with a step still in progress is superseded",
			job: job("Acceptance Tests (linux, shard 2/10)", conclusionCancelled, at(t43m), []Step{
				okStep("Set up job", t0),
				step("Acceptance tests", "in_progress", "", nil),
			}),
			want: ClassSuperseded,
		},
		{
			name: "cancelled before any step ran is superseded",
			job:  job("Acceptance Tests (linux, shard 3/10)", conclusionCancelled, at(t0), nil),
			want: ClassSuperseded,
		},
		{
			name: "cancelled with a missing job completion time is superseded",
			job:  job("Build (windows)", conclusionCancelled, nil, greenSteps()),
			want: ClassSuperseded,
		},
		{
			name: "cancelled with a missing last-step completion time is superseded",
			job: job("Build (windows)", conclusionCancelled, at(t43m), []Step{
				okStep("Set up job", t0),
				step("Complete job", statusCompleted, conclusionSuccess, nil),
			}),
			want: ClassSuperseded,
		},
		{
			name: "failure with no failed step is runner-lost",
			job:  job("Acceptance Tests (macos, shard 4/10)", conclusionFailure, at(t43m), []Step{okStep("Set up job", t0)}),
			want: ClassRunnerLost,
		},
		{
			name: "failure whose only failed step is an aggregator check is check-cascade",
			job: job("Acceptance Tests (windows)", conclusionFailure, at(t43m), []Step{
				okStep("Harden Runner", t43m),
				step("Check per-OS test matrix result", statusCompleted, conclusionFailure, at(t43m)),
				step("Check terraform-registry-cache result", statusCompleted, conclusionSkipped, at(t43m)),
				okStep("Complete job", t43m),
			}),
			want: ClassCheckCascade,
		},
		{
			name: "failure with two aggregator checks failed is check-cascade",
			job: job("[k3s] demo-helmfile", conclusionFailure, at(t43m), []Step{
				step("Check k3s matrix result", statusCompleted, conclusionFailure, at(t43m)),
				step("Check terraform-registry-cache result", statusCompleted, conclusionFailure, at(t43m)),
			}),
			want: ClassCheckCascade,
		},
		{
			name: "failure with a real failed step is real-failure",
			job: job("Acceptance Tests (linux, shard 4/10)", conclusionFailure, at(t0.Add(20*time.Minute)), []Step{
				okStep("Set up job", t0),
				step("Acceptance tests", statusCompleted, conclusionFailure, at(t0.Add(19*time.Minute))),
				okStep("Complete job", t0.Add(20*time.Minute)),
			}),
			want: ClassRealFailure,
		},
		{
			name: "aggregator check failed alongside a real step is real-failure",
			job: job("[k3s] demo-helmfile", conclusionFailure, at(t0.Add(20*time.Minute)), []Step{
				step("Run demo", statusCompleted, conclusionFailure, at(t0.Add(19*time.Minute))),
				step("Check k3s matrix result", statusCompleted, conclusionFailure, at(t0.Add(19*time.Minute+30*time.Second))),
			}),
			want: ClassRealFailure,
		},
		{
			name: "any other conclusion is real-failure",
			job:  job("Build (linux)", "timed_out", at(t43m), greenSteps()),
			want: ClassRealFailure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Classify([]Job{tt.job}, tt.opts)
			require.Len(t, got, 1)
			assert.Equal(t, Classified{Job: tt.job.Name, Conclusion: tt.job.Conclusion, Class: tt.want}, got[0])
		})
	}
}

func TestClassifyOmitsSuccessfulSkippedAndRunningJobs(t *testing.T) {
	t.Parallel()

	jobs := []Job{
		job("Build (linux)", conclusionSuccess, at(t9m), greenSteps()),
		job("release", conclusionSkipped, at(t9m), nil),
		{Name: "Acceptance Tests (linux, shard 1/10)", Status: "in_progress"},
		job("Build (windows)", conclusionCancelled, at(t43m), greenSteps()),
		job("Acceptance Tests (linux, shard 4/10)", conclusionFailure, at(t9m), []Step{
			step("Acceptance tests", statusCompleted, conclusionFailure, at(t9m)),
		}),
	}
	got := Classify(jobs, Options{})
	assert.Equal(t, []Classified{
		{Job: "Build (windows)", Conclusion: conclusionCancelled, Class: ClassRunnerStuckAfterComplete},
		{Job: "Acceptance Tests (linux, shard 4/10)", Conclusion: conclusionFailure, Class: ClassRealFailure},
	}, got)
	assert.Empty(t, Classify(nil, Options{}))
}

func TestDecodeJobs(t *testing.T) {
	t.Parallel()

	const page1 = `{"jobs":[{"name":"a","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:09:36.123456Z","steps":[]}]}`
	const page2 = `{"jobs":[{"name":"b","status":"completed","conclusion":"cancelled","completed_at":"2026-09-03T16:43:17Z","steps":[{"name":"s","status":"completed","conclusion":"success","completed_at":null}]}]}`

	tests := []struct {
		name      string
		input     string
		wantNames []string
		wantErr   bool
	}{
		{name: "single page", input: page1, wantNames: []string{"a"}},
		{name: "concatenated pages as emitted by gh api --paginate", input: page1 + "\n" + page2 + "\n", wantNames: []string{"a", "b"}},
		{name: "bare array", input: `[{"name":"c","conclusion":"failure"},{"name":"d","conclusion":"success"}]`, wantNames: []string{"c", "d"}},
		{name: "empty input yields no jobs", input: "", wantNames: []string{}},
		{name: "page without jobs key yields no jobs", input: `{"total_count":0}`, wantNames: []string{}},
		{name: "truncated JSON is an error", input: `{"jobs":[`, wantErr: true},
		{name: "scalar document is an error", input: `42`, wantErr: true},
		{name: "wrong shape is an error", input: `{"jobs":{"name":"x"}}`, wantErr: true},
		{name: "array of scalars is an error", input: `[1,2]`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			jobs, err := DecodeJobs(strings.NewReader(tt.input))
			if tt.wantErr {
				require.ErrorIs(t, err, errInvalidJobsJSON)
				return
			}
			require.NoError(t, err)
			names := make([]string, 0, len(jobs))
			for _, j := range jobs {
				names = append(names, j.Name)
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}

	t.Run("fractional seconds and null timestamps decode", func(t *testing.T) {
		t.Parallel()
		jobs, err := DecodeJobs(strings.NewReader(page1 + page2))
		require.NoError(t, err)
		require.Len(t, jobs, 2)
		require.NotNil(t, jobs[0].CompletedAt)
		assert.Equal(t, time.Date(2026, 9, 3, 16, 9, 36, 123456000, time.UTC), jobs[0].CompletedAt.UTC())
		require.Len(t, jobs[1].Steps, 1)
		assert.Nil(t, jobs[1].Steps[0].CompletedAt)
	})
}

func TestVerdict(t *testing.T) {
	t.Parallel()

	entry := func(class Class) Classified { return Classified{Job: string(class), Conclusion: "x", Class: class} }
	tests := []struct {
		name       string
		classified []Classified
		want       Outcome
		reason     string
	}{
		{name: "nothing failed", want: OutcomeNoJobs, reason: "no non-success jobs"},
		{name: "stuck runner alone", classified: []Classified{entry(ClassRunnerStuckAfterComplete)}, want: OutcomeRerun, reason: "all 1 non-success job(s) are infrastructure zombies or cascades of one"},
		{name: "lost runner alone", classified: []Classified{entry(ClassRunnerLost)}, want: OutcomeRerun, reason: "all 1 non-success job(s) are infrastructure zombies or cascades of one"},
		{name: "stuck runner plus cascades", classified: []Classified{entry(ClassRunnerStuckAfterComplete), entry(ClassCheckCascade), entry(ClassCheckCascade)}, want: OutcomeRerun, reason: "all 3 non-success job(s) are infrastructure zombies or cascades of one"},
		{name: "cascade alone never reruns", classified: []Classified{entry(ClassCheckCascade)}, want: OutcomeNoRerun, reason: "no runner-stuck-after-complete or runner-lost jobs"},
		{name: "real failure alone", classified: []Classified{entry(ClassRealFailure)}, want: OutcomeNoRerun, reason: "no runner-stuck-after-complete or runner-lost jobs"},
		{name: "real failure vetoes a stuck runner", classified: []Classified{entry(ClassRunnerStuckAfterComplete), entry(ClassRealFailure)}, want: OutcomeNoRerun, reason: "1 job(s) are real-failure or superseded"},
		{name: "superseded vetoes a lost runner", classified: []Classified{entry(ClassRunnerLost), entry(ClassSuperseded), entry(ClassSuperseded)}, want: OutcomeNoRerun, reason: "2 job(s) are real-failure or superseded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, reason := Verdict(tt.classified)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.reason, reason)
		})
	}
}

func TestWriteTSV(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	require.NoError(t, WriteTSV(&buf, []Classified{
		{Job: "Build (windows)", Conclusion: conclusionCancelled, Class: ClassRunnerStuckAfterComplete},
		{Job: "Acceptance Tests (windows)", Conclusion: conclusionFailure, Class: ClassCheckCascade},
	}))
	assert.Equal(t, "Build (windows)\tcancelled\trunner-stuck-after-complete\nAcceptance Tests (windows)\tfailure\tcheck-cascade\n", buf.String())

	buf.Reset()
	require.NoError(t, WriteTSV(&buf, nil))
	assert.Empty(t, buf.String())
}

func TestWriteMarkdownSummary(t *testing.T) {
	t.Parallel()

	t.Run("zombie run", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		require.NoError(t, WriteMarkdownSummary(&buf, RunRef{Repo: "cloudposse/atmos", RunID: "33774798945", RunAttempt: "2"}, []Classified{
			{Job: "Build (windows)", Conclusion: conclusionCancelled, Class: ClassRunnerStuckAfterComplete},
			{Job: "Acceptance Tests (windows)", Conclusion: conclusionFailure, Class: ClassCheckCascade},
		}, OutcomeRerun, "all 2 non-success job(s) are infrastructure zombies or cascades of one"))

		want := "### Infra-failure classification: run [33774798945 attempt 2](https://github.com/cloudposse/atmos/actions/runs/33774798945/attempts/2)\n\n" +
			"| Job | Conclusion | Class |\n" +
			"|---|---|---|\n" +
			"| Build (windows) | cancelled | runner-stuck-after-complete |\n" +
			"| Acceptance Tests (windows) | failure | check-cascade |\n" +
			"\nVerdict: **rerun** (all 2 non-success job(s) are infrastructure zombies or cascades of one)\n"
		assert.Equal(t, want, buf.String())
	})

	t.Run("no non-success jobs", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		require.NoError(t, WriteMarkdownSummary(&buf, RunRef{Repo: "cloudposse/atmos", RunID: "1", RunAttempt: "1"}, nil, OutcomeNoJobs, "no non-success jobs"))

		want := "### Infra-failure classification: run [1 attempt 1](https://github.com/cloudposse/atmos/actions/runs/1/attempts/1)\n\n" +
			"| Job | Conclusion | Class |\n" +
			"|---|---|---|\n" +
			"\nVerdict: **no-jobs** (no non-success jobs)\n"
		assert.Equal(t, want, buf.String())
	})
}

// TestRealRuns replays the job listings of four real `Tests` runs from
// 2026-09-03 (trimmed to their non-success jobs plus two successful ones; the
// full listings classify identically). Two are zombie runs that a human
// re-ran with "Re-run failed jobs" and that went green; two have genuine
// failures and must never be rerun.
func TestRealRuns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run     string
		want    []Classified
		outcome Outcome
	}{
		{
			// Build (windows) reaped 34 min after `Complete job`; everything
			// downstream is an aggregator that saw no shards.
			run: "33774798945",
			want: []Classified{
				{Job: "Build (windows)", Conclusion: conclusionCancelled, Class: ClassRunnerStuckAfterComplete},
				{Job: "[k3s] demo-helmfile", Conclusion: conclusionFailure, Class: ClassCheckCascade},
				{Job: "Acceptance Tests (windows)", Conclusion: conclusionFailure, Class: ClassCheckCascade},
				{Job: "Acceptance Tests (macos)", Conclusion: conclusionFailure, Class: ClassCheckCascade},
				{Job: "Acceptance Tests (linux)", Conclusion: conclusionFailure, Class: ClassCheckCascade},
			},
			outcome: OutcomeRerun,
		},
		{
			// Two Windows shards reaped 50 min after their last step.
			run: "33769749251",
			want: []Classified{
				{Job: "Acceptance Tests (windows, shard 6/10)", Conclusion: conclusionCancelled, Class: ClassRunnerStuckAfterComplete},
				{Job: "Acceptance Tests (windows, shard 2/10)", Conclusion: conclusionCancelled, Class: ClassRunnerStuckAfterComplete},
				{Job: "Acceptance Tests (windows)", Conclusion: conclusionFailure, Class: ClassCheckCascade},
			},
			outcome: OutcomeRerun,
		},
		{
			// Genuine unit-test and demo failures.
			run: "33775397630",
			want: []Classified{
				{Job: "[magefiles] unit tests", Conclusion: conclusionFailure, Class: ClassRealFailure},
				{Job: "[mock-linux] examples/demo-atlantis", Conclusion: conclusionFailure, Class: ClassRealFailure},
			},
			outcome: OutcomeNoRerun,
		},
		{
			// A real shard failure plus the aggregator that reported it.
			run: "33771957449",
			want: []Classified{
				{Job: "Acceptance Tests (linux, shard 4/10)", Conclusion: conclusionFailure, Class: ClassRealFailure},
				{Job: "Acceptance Tests (linux)", Conclusion: conclusionFailure, Class: ClassCheckCascade},
			},
			outcome: OutcomeNoRerun,
		},
	}
	for _, tt := range tests {
		t.Run(tt.run, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(filepath.Join("testdata", "run-"+tt.run+".json"))
			require.NoError(t, err)
			defer f.Close()
			jobs, err := DecodeJobs(f)
			require.NoError(t, err)
			require.NotEmpty(t, jobs)

			got := Classify(jobs, Options{})
			assert.Equal(t, tt.want, got)
			outcome, _ := Verdict(got)
			assert.Equal(t, tt.outcome, outcome)
		})
	}
}
