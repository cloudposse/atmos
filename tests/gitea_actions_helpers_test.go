package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// giteaAPI is a minimal client for the local Gitea emulator's REST API, authenticated
// as the throwaway atmos/atmos admin the emulator bootstrap creates.
type giteaAPI struct {
	base string
}

// raw sends one request and returns the status and the undecoded response body.
func (g giteaAPI) raw(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, g.base+"/api/v1"+path, reader)
	require.NoError(t, err)
	req.SetBasicAuth("atmos", "atmos")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoErrorf(t, err, "%s %s", method, path)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, data
}

// mustDo is do with an expected status.
func (g giteaAPI) mustDo(t *testing.T, want int, method, path string, body, out any) {
	t.Helper()
	status, data := g.raw(t, method, path, body)
	require.Equalf(t, want, status, "%s %s: %s", method, path, string(data))
	if out != nil {
		require.NoError(t, json.Unmarshal(data, out))
	}
}

type giteaRun struct {
	ID         int64  `json:"id"`
	Path       string `json:"path"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
}

type giteaStatus struct {
	Context     string `json:"context"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

// waitForRun polls until the workflow run for (file, event, sha) completes, then
// requires success. On failure it attaches every job's log to the test output.
func (g giteaAPI) waitForRun(t *testing.T, file, event, sha string) {
	t.Helper()
	var run giteaRun
	require.Eventuallyf(t, func() bool {
		var page struct {
			Runs []giteaRun `json:"workflow_runs"`
		}
		g.mustDo(t, http.StatusOK, http.MethodGet, "/repos/atmos/infra/actions/runs?limit=50", nil, &page)
		for _, r := range page.Runs {
			if strings.HasPrefix(r.Path, file+"@") && r.Event == event && r.HeadSHA == sha {
				run = r
				return r.Status == "completed"
			}
		}
		return false
	}, giteaActionsRunTimeout, 5*time.Second, "workflow %s (%s) did not complete for %s", file, event, sha)
	if run.Conclusion != "success" {
		g.logRunJobs(t, run.ID)
	}
	require.Equalf(t, "success", run.Conclusion, "workflow %s (%s) concluded %s", file, event, run.Conclusion)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// logRunJobs writes each job's log for a run into the test output for diagnosis.
func (g giteaAPI) logRunJobs(t *testing.T, runID int64) {
	t.Helper()
	var jobs struct {
		Jobs []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"jobs"`
	}
	g.mustDo(t, http.StatusOK, http.MethodGet, fmt.Sprintf("/repos/atmos/infra/actions/runs/%d/jobs", runID), nil, &jobs)
	for _, job := range jobs.Jobs {
		_, log := g.raw(t, http.MethodGet, fmt.Sprintf("/repos/atmos/infra/actions/jobs/%d/logs", job.ID), nil)
		t.Logf("===== job %s =====\n%s", job.Name, ansiEscape.ReplaceAllString(string(log), ""))
	}
}

// statuses returns the commit statuses on sha keyed by context, keeping the latest
// state of each (Gitea lists newest first).
func (g giteaAPI) statuses(t *testing.T, sha string) map[string]giteaStatus {
	t.Helper()
	var list []giteaStatus
	g.mustDo(t, http.StatusOK, http.MethodGet, "/repos/atmos/infra/commits/"+sha+"/statuses?limit=50", nil, &list)
	latest := map[string]giteaStatus{}
	for _, s := range list {
		if _, seen := latest[s.Context]; !seen {
			latest[s.Context] = s
		}
	}
	return latest
}

// buildLinuxAtmos cross-compiles this checkout's atmos for Linux on the host's
// architecture (what Docker Desktop and Linux CI runners execute), so workflow jobs
// run the code under test rather than the published image's release.
func buildLinuxAtmos(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs("..")
	require.NoError(t, err)
	out := filepath.Join(t.TempDir(), "atmos")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, ".")
	cmd.Dir = repoRoot
	cmd.Env = mergeCommandEnv(os.Environ(), map[string]string{
		"CGO_ENABLED": "0",
		"GOOS":        "linux",
		"GOARCH":      runtime.GOARCH,
	})
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "cross-compiling atmos for linux/%s: %s", runtime.GOARCH, string(output))
	return out
}

// mountAtmosIntoJobs appends runner options that bind-mount binary over the job
// image's atmos, so every workflow job runs the code under test.
func mountAtmosIntoJobs(t *testing.T, runnerConfig, binary string) {
	t.Helper()
	extra := fmt.Sprintf("  options: --volume %s:/usr/local/bin/atmos:ro\n  valid_volumes:\n    - %s\n", binary, binary)
	f, err := os.OpenFile(runnerConfig, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	defer f.Close()
	_, err = f.WriteString(extra)
	require.NoError(t, err)
}

// gitOutput runs git in dir and returns its trimmed stdout.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoErrorf(t, err, "git %s", strings.Join(args, " "))
	return strings.TrimSpace(string(out))
}
