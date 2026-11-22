package gitea_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform" // Register the terraform CI plugin.
	"github.com/cloudposse/atmos/pkg/ci/providers/gitea"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests set.
var (
	_ = schema.CIConfig{Enabled: true}
	_ = schema.CIChecksConfig{Enabled: ptr(true)}
)

func ptr[T any](v T) *T { return &v }

const planOutput = "Plan: 1 to add, 0 to change, 0 to destroy."

// giteaRun simulates an act_runner job: it exports both GITHUB_ACTIONS and GITEA_ACTIONS,
// clears every Gitea API setting, and returns the step summary and output file paths.
func giteaRun(t *testing.T, serverURL string) (summary, output string) {
	t.Helper()

	t.Cleanup(ci.SwapRegistryForTest())
	ci.Register(gitea.NewProvider())
	// Resolve the SHA from GITHUB_SHA rather than the surrounding git checkout.
	t.Chdir(t.TempDir())

	dir := t.TempDir()
	summary = filepath.Join(dir, "summary.md")
	output = filepath.Join(dir, "output.txt")
	require.NoError(t, os.WriteFile(summary, nil, 0o600))
	require.NoError(t, os.WriteFile(output, nil, 0o600))

	for name, value := range map[string]string{
		"CI": "true", "GITHUB_ACTIONS": "true", "GITEA_ACTIONS": "true",
		"GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "acme/infra",
		"GITHUB_SHA": "0123456789abcdef0123456789abcdef01234567", "GITHUB_REF": "refs/heads/main",
		"GITHUB_REF_NAME": "main", "GITHUB_RUN_ID": "1",
		"GITHUB_SERVER_URL": serverURL, "GITHUB_STEP_SUMMARY": summary, "GITHUB_OUTPUT": output,
		"GITEA_SERVER_URL": "", "GITEA_API_URL": "", "GITHUB_API_URL": "", "GITEA_STEP_SUMMARY": "", "GITEA_OUTPUT": "",
		"ATMOS_CI_GITEA_API_URL": "", "ATMOS_CI_GITEA_ALLOW_INSECURE": "",
		"ATMOS_CI_GITEA_TOKEN": "", "GITEA_TOKEN": "",
		"GITHUB_EVENT_PATH": "",
	} {
		t.Setenv(name, value)
	}
	return summary, output
}

// runPlan fires the before and after plan hooks the way `atmos terraform plan` does.
func runPlan(t *testing.T, checks bool) []error {
	t.Helper()

	config := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
	if checks {
		config.CI.Checks.Enabled = ptr(true)
	}
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "plan"}
	var errs []error
	for _, event := range []string{"before.terraform.plan", "after.terraform.plan"} {
		errs = append(errs, ci.Execute(ci.ExecuteOptions{Event: event, AtmosConfig: config, Info: info, Output: planOutput}))
	}
	return errs
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// An unusable Gitea API URL disables only the API-backed features; the run itself,
// the job summary, and the step outputs continue through the runner's files.
func TestExecuteWithUnusableAPIURLKeepsSummaryAndOutputs(t *testing.T) {
	for name, serverURL := range map[string]string{
		"http server URL without opt-in": "http://gitea.internal:3000",
		"missing server URL":             "",
	} {
		t.Run(name, func(t *testing.T) {
			for _, checks := range []bool{false, true} {
				summary, output := giteaRun(t, serverURL)

				for _, err := range runPlan(t, checks) {
					require.NoError(t, err)
				}
				assert.Contains(t, readFile(t, summary), "plan")
				assert.Contains(t, readFile(t, output), "has_changes=true")
			}
		})
	}
}

// A token never reaches an HTTP endpoint unless ATMOS_CI_GITEA_ALLOW_INSECURE is set.
func TestExecuteNeverSendsTokenOverHTTPWithoutOptIn(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	t.Cleanup(server.Close)

	summary, _ := giteaRun(t, server.URL)
	t.Setenv("ATMOS_CI_GITEA_TOKEN", "secret-token")

	for _, err := range runPlan(t, true) {
		require.NoError(t, err)
	}
	assert.NotEmpty(t, readFile(t, summary))
	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, requests, "no API request may be sent over HTTP without the opt-in")
}

// With the opt-in, the same HTTP endpoint receives authenticated commit statuses.
func TestExecuteSendsStatusesOverHTTPWithOptIn(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	t.Cleanup(server.Close)

	summary, output := giteaRun(t, server.URL)
	t.Setenv("ATMOS_CI_GITEA_TOKEN", "secret-token")
	t.Setenv("ATMOS_CI_GITEA_ALLOW_INSECURE", "true")

	for _, err := range runPlan(t, true) {
		require.NoError(t, err)
	}
	assert.NotEmpty(t, readFile(t, summary))
	assert.Contains(t, readFile(t, output), "has_changes=true")

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, requests, "commit statuses reach the opted-in HTTP endpoint")
	for _, request := range requests {
		assert.True(t, strings.HasPrefix(request, "POST /api/v1/repos/acme/infra/statuses/0123456789abcdef0123456789abcdef01234567 token secret-token"), request)
	}
}
