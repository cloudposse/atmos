package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/ci/rerun"
)

// jobsPage builds one un-paginated jobs API response: conclusion "" is
// rendered as JSON null, the way the API reports a not-yet-settled job.
func jobsPage(t *testing.T, jobs ...[2]string) *http.Response {
	t.Helper()
	type job struct {
		Name       string  `json:"name"`
		Status     string  `json:"status"`
		Conclusion *string `json:"conclusion"`
	}
	page := struct {
		Jobs []job `json:"jobs"`
	}{}
	for _, j := range jobs {
		var c *string
		if j[1] != "" {
			v := j[1]
			c = &v
		}
		page.Jobs = append(page.Jobs, job{Name: j[0], Status: "completed", Conclusion: c})
	}
	raw, err := json.Marshal(page)
	require.NoError(t, err)
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(raw))}
}

func shard(target string, n int, conclusion string) [2]string {
	return [2]string{"Acceptance Tests (" + target + ", shard " + string(rune('0'+n)) + "/3)", conclusion}
}

// recordingWait records each requested wait instead of sleeping.
type recordingWait struct{ waits []time.Duration }

func (r *recordingWait) wait(_ context.Context, d time.Duration) error {
	r.waits = append(r.waits, d)
	return nil
}

func params(target string, polls int, wait func(context.Context, time.Duration) error) *ShardResultsParams {
	return &ShardResultsParams{
		Run:        rerun.RunRef{Repo: "cloudposse/atmos", RunID: "42", RunAttempt: "1"},
		Target:     target,
		ShardCount: 3,
		Polls:      polls,
		Interval:   time.Second,
		Wait:       wait,
	}
}

func TestTargetFromCheckName(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "Acceptance Tests (linux)", want: "linux"},
		{in: "Acceptance Tests (macos)", want: "macos"},
		{in: "Acceptance Tests (windows)", want: "windows"},
		{in: "Acceptance Tests ()", wantErr: true},
		{in: "Acceptance Tests (linux", wantErr: true},
		{in: "Build (linux)", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := TargetFromCheckName(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errInvalidCheckName)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckShardResults_AllSucceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := rerun.NewMockRESTClient(ctrl)
	client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/actions/runs/42/attempts/1/jobs?per_page=100", nil).
		Return(jobsPage(t, [2]string{"Build (linux)", "success"}, shard("linux", 1, "success"), shard("linux", 2, "success"), shard("linux", 3, "success"), //nolint:bodyclose // closed by the code under test, not this fixture.
			shard("macos", 1, "failure")), nil)

	rw := &recordingWait{}
	var out bytes.Buffer
	err := CheckShardResults(context.Background(), client, &out, params("linux", 5, rw.wait))
	require.NoError(t, err)
	assert.Contains(t, out.String(), `all 3 "linux" shard jobs succeeded`)
	assert.Empty(t, rw.waits, "no polling when everything settled on the first listing")
}

func TestCheckShardResults_WaitsForEmptyConclusion(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := rerun.NewMockRESTClient(ctrl)
	gomock.InOrder(
		// First listing: shard 3 finished but the API has not written its conclusion.
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, gomock.Any(), nil).
			Return(jobsPage(t, shard("macos", 1, "success"), shard("macos", 2, "success"), shard("macos", 3, "")), nil), //nolint:bodyclose // closed by the code under test, not this fixture.
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, gomock.Any(), nil).
			Return(jobsPage(t, shard("macos", 1, "success"), shard("macos", 2, "success"), shard("macos", 3, "")), nil), //nolint:bodyclose // closed by the code under test, not this fixture.
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, gomock.Any(), nil).
			Return(jobsPage(t, shard("macos", 1, "success"), shard("macos", 2, "success"), shard("macos", 3, "success")), nil), //nolint:bodyclose // closed by the code under test, not this fixture.
	)

	rw := &recordingWait{}
	var out bytes.Buffer
	err := CheckShardResults(context.Background(), client, &out, params("macos", 5, rw.wait))
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{time.Second, time.Second}, rw.waits)
	assert.Contains(t, out.String(), `poll 1/5: 1 of 3 "macos" shard jobs have no conclusion yet`)
	assert.Contains(t, out.String(), `all 3 "macos" shard jobs succeeded`)
}

func TestCheckShardResults_NeverSettles(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := rerun.NewMockRESTClient(ctrl)
	client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, gomock.Any(), nil).Times(2).
		DoAndReturn(func(context.Context, string, string, io.Reader) (*http.Response, error) {
			return jobsPage(t, shard("windows", 1, "success"), shard("windows", 2, ""), shard("windows", 3, "success")), nil
		})

	rw := &recordingWait{}
	var out bytes.Buffer
	err := CheckShardResults(context.Background(), client, &out, params("windows", 2, rw.wait))
	require.Error(t, err)
	assert.ErrorIs(t, err, errShardsNotSettled)
	assert.Len(t, rw.waits, 1, "no wait after the final poll")
	assert.Contains(t, out.String(), "never settled after 2 polls")
	assert.Contains(t, out.String(), "Acceptance Tests (windows, shard 2/3)\t(no conclusion yet)")
}

func TestCheckShardResults_FailedShard(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := rerun.NewMockRESTClient(ctrl)
	client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, gomock.Any(), nil).
		Return(jobsPage(t, shard("linux", 1, "success"), shard("linux", 2, "failure"), shard("linux", 3, "cancelled")), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

	var out bytes.Buffer
	err := CheckShardResults(context.Background(), client, &out, params("linux", 5, (&recordingWait{}).wait))
	require.Error(t, err)
	assert.ErrorIs(t, err, errShardsFailed)
	assert.Contains(t, err.Error(), `2 of 3 "linux" shards`)
	assert.Contains(t, out.String(), "Acceptance Tests (linux, shard 2/3)\tfailure")
	assert.Contains(t, out.String(), "Acceptance Tests (linux, shard 3/3)\tcancelled")
	assert.NotContains(t, out.String(), "shard 1/3")
}

func TestCheckShardResults_ShardCountMismatchFailsAtOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := rerun.NewMockRESTClient(ctrl)
	client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, gomock.Any(), nil).
		Return(jobsPage(t, shard("linux", 1, "success"), shard("linux", 2, "")), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

	rw := &recordingWait{}
	var out bytes.Buffer
	err := CheckShardResults(context.Background(), client, &out, params("linux", 5, rw.wait))
	require.Error(t, err)
	assert.ErrorIs(t, err, errShardCount)
	assert.Empty(t, rw.waits, "a shape mismatch is not something waiting fixes")
	assert.Contains(t, out.String(), `expected 3 "linux" shard jobs, found 2`)
}

func TestCheckShardResults_Errors(t *testing.T) {
	t.Run("fetch error propagates", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, gomock.Any(), nil).Return(nil, assert.AnError)

		err := CheckShardResults(context.Background(), client, io.Discard, params("linux", 5, (&recordingWait{}).wait))
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
	t.Run("invalid shard count", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		p := params("linux", 5, nil)
		p.ShardCount = 0
		err := CheckShardResults(context.Background(), rerun.NewMockRESTClient(ctrl), io.Discard, p)
		require.Error(t, err)
		assert.ErrorIs(t, err, errShardCountValue)
	})
	t.Run("cancelled context stops the wait", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, gomock.Any(), nil).
			Return(jobsPage(t, shard("linux", 1, ""), shard("linux", 2, "success"), shard("linux", 3, "success")), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p := params("linux", 5, nil) // Real sleepWithContext, which must honor ctx.
		p.Interval = time.Hour
		err := CheckShardResults(ctx, client, io.Discard, p)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestSleepWithContext_ReturnsAfterDuration(t *testing.T) {
	start := time.Now()
	require.NoError(t, sleepWithContext(context.Background(), 5*time.Millisecond))
	assert.GreaterOrEqual(t, time.Since(start), 5*time.Millisecond)
}

func TestShardResultsParams_Polling(t *testing.T) {
	customWaitCalled := false
	customWait := func(context.Context, time.Duration) error {
		customWaitCalled = true
		return nil
	}

	tests := []struct {
		name         string
		params       ShardResultsParams
		wantPolls    int
		wantInterval time.Duration
		wantCustom   bool
	}{
		{
			name:         "zero values fall back to defaults",
			params:       ShardResultsParams{},
			wantPolls:    DefaultShardPolls,
			wantInterval: DefaultShardPollInterval,
		},
		{
			name:         "negative values fall back to defaults",
			params:       ShardResultsParams{Polls: -1, Interval: -1},
			wantPolls:    DefaultShardPolls,
			wantInterval: DefaultShardPollInterval,
		},
		{
			// Only Polls is invalid: proves the two defaults apply
			// independently rather than as an all-or-nothing pair.
			name:         "invalid polls preserve explicit interval",
			params:       ShardResultsParams{Polls: 0, Interval: time.Second},
			wantPolls:    DefaultShardPolls,
			wantInterval: time.Second,
		},
		{
			name:         "invalid interval preserves explicit polls",
			params:       ShardResultsParams{Polls: 3, Interval: 0},
			wantPolls:    3,
			wantInterval: DefaultShardPollInterval,
		},
		{
			name:         "explicit values are kept as-is",
			params:       ShardResultsParams{Polls: 3, Interval: time.Millisecond, Wait: customWait},
			wantPolls:    3,
			wantInterval: time.Millisecond,
			wantCustom:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			polls, interval, wait := tt.params.polling()
			assert.Equal(t, tt.wantPolls, polls)
			assert.Equal(t, tt.wantInterval, interval)
			require.NotNil(t, wait)
			if tt.wantCustom {
				customWaitCalled = false
				assert.NoError(t, wait(context.Background(), 0))
				assert.True(t, customWaitCalled, "polling() must return the injected Wait function, not sleepWithContext")
			}
		})
	}
}

func TestWriteShardListing_TabSeparated(t *testing.T) {
	var out bytes.Buffer
	writeShardListing(&out, []ShardResult{{Name: "a", Conclusion: "success"}, {Name: "b"}})
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "  a\tsuccess", lines[0])
	assert.Equal(t, "  b\t(no conclusion yet)", lines[1])
}
