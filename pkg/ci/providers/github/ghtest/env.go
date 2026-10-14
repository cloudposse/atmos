package ghtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Defaults used by SetEnv when an option does not override them.
const (
	defaultRepository = "owner/repo"
	defaultSHA        = "0123456789abcdef0123456789abcdef01234567"
	defaultBaseSHA    = "fedcba9876543210fedcba9876543210fedcba98"
	defaultRef        = "refs/heads/main"
	defaultServerURL  = "https://github.com"
	defaultToken      = "ghtest-token"
	defaultRunID      = "1234567890"
	defaultEventName  = "push"
	eventPullRequest  = "pull_request"

	// FilePerm is the mode of files SetEnv creates.
	filePerm = 0o600
)

// Env holds the paths of the files SetEnv created for the GitHub Actions runner contract.
type Env struct {
	// Output is the $GITHUB_OUTPUT file.
	Output string
	// EnvFile is the $GITHUB_ENV file.
	EnvFile string
	// Path is the $GITHUB_PATH file.
	Path string
	// Summary is the $GITHUB_STEP_SUMMARY file.
	Summary string
	// EventPath is the $GITHUB_EVENT_PATH event payload JSON file.
	EventPath string
}

type pullRequestFixture struct {
	number  int
	headRef string
	baseRef string
}

type envConfig struct {
	repository string
	sha        string
	ref        string
	serverURL  string
	token      string
	runID      string
	eventName  string
	payload    map[string]any
	pr         *pullRequestFixture

	// Derived from the options by SetEnv.
	refName string
	headRef string
	baseRef string
}

// EnvOption customizes SetEnv.
type EnvOption func(*envConfig)

// WithRepository sets GITHUB_REPOSITORY ("owner/repo").
func WithRepository(repository string) EnvOption {
	defer perf.Track(nil, "ghtest.WithRepository")()

	return func(c *envConfig) { c.repository = repository }
}

// WithSHA sets GITHUB_SHA.
func WithSHA(sha string) EnvOption {
	defer perf.Track(nil, "ghtest.WithSHA")()

	return func(c *envConfig) { c.sha = sha }
}

// WithRef sets GITHUB_REF (for example "refs/heads/feature"). GITHUB_REF_NAME is derived from it.
func WithRef(ref string) EnvOption {
	defer perf.Track(nil, "ghtest.WithRef")()

	return func(c *envConfig) { c.ref = ref }
}

// WithEvent sets GITHUB_EVENT_NAME and the top-level keys of the event payload
// file. When combined with WithPullRequest the payload keys are overlaid on the
// generated pull_request payload, and the event name (for example
// "pull_request_target") replaces the default "pull_request".
func WithEvent(name string, payload map[string]any) EnvOption {
	defer perf.Track(nil, "ghtest.WithEvent")()

	return func(c *envConfig) {
		c.eventName = name
		c.payload = payload
	}
}

// WithRunID sets GITHUB_RUN_ID.
func WithRunID(id string) EnvOption {
	defer perf.Track(nil, "ghtest.WithRunID")()

	return func(c *envConfig) { c.runID = id }
}

// WithServerURL sets GITHUB_SERVER_URL.
func WithServerURL(serverURL string) EnvOption {
	defer perf.Track(nil, "ghtest.WithServerURL")()

	return func(c *envConfig) { c.serverURL = serverURL }
}

// WithToken sets GITHUB_TOKEN.
func WithToken(token string) EnvOption {
	defer perf.Track(nil, "ghtest.WithToken")()

	return func(c *envConfig) { c.token = token }
}

// WithPullRequest makes the run a pull request: it writes a pull_request event
// payload, sets GITHUB_REF=refs/pull/N/merge, GITHUB_REF_NAME=N/merge,
// GITHUB_HEAD_REF and GITHUB_BASE_REF, and defaults the event name to
// "pull_request" unless WithEvent chose another (such as pull_request_target).
func WithPullRequest(number int, headRef, baseRef string) EnvOption {
	defer perf.Track(nil, "ghtest.WithPullRequest")()

	return func(c *envConfig) {
		c.pr = &pullRequestFixture{number: number, headRef: headRef, baseRef: baseRef}
	}
}

// SetEnv points the process at s as a GitHub Actions run, using t.Setenv so the
// environment is restored when the test ends. It returns the paths of the
// runner files it created. Tests using SetEnv must not be parallel.
//
// Variables set: GITHUB_ACTIONS, GITHUB_API_URL, GITHUB_TOKEN, GITHUB_REPOSITORY,
// GITHUB_SHA, GITHUB_REF, GITHUB_REF_NAME, GITHUB_HEAD_REF, GITHUB_BASE_REF,
// GITHUB_EVENT_NAME, GITHUB_EVENT_PATH, GITHUB_RUN_ID, GITHUB_RUN_NUMBER,
// GITHUB_SERVER_URL, GITHUB_ACTOR, GITHUB_WORKFLOW, GITHUB_JOB, GITHUB_OUTPUT,
// GITHUB_ENV, GITHUB_PATH and GITHUB_STEP_SUMMARY. Ambient overrides that could
// redirect the client (ATMOS_CI_GITHUB_API_URL, ATMOS_CI_GITHUB_TOKEN,
// ATMOS_PRO_GITHUB_TOKEN, GH_TOKEN) are blanked.
//
// The provider resolves Context.SHA from git HEAD of the working directory
// before falling back to GITHUB_SHA. Tests that assert the SHA must run outside
// a git checkout (for example after t.Chdir(t.TempDir())).
func SetEnv(t testing.TB, s *Server, opts ...EnvOption) Env {
	defer perf.Track(nil, "ghtest.SetEnv")()

	t.Helper()

	cfg := &envConfig{
		repository: defaultRepository,
		sha:        defaultSHA,
		ref:        defaultRef,
		serverURL:  defaultServerURL,
		token:      defaultToken,
		runID:      defaultRunID,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.eventName == "" {
		cfg.eventName = defaultEventName
		if cfg.pr != nil {
			cfg.eventName = eventPullRequest
		}
	}

	cfg.refName = strings.TrimPrefix(strings.TrimPrefix(cfg.ref, "refs/heads/"), "refs/tags/")
	if cfg.pr != nil {
		cfg.ref = fmt.Sprintf("refs/pull/%d/merge", cfg.pr.number)
		cfg.refName = fmt.Sprintf("%d/merge", cfg.pr.number)
		cfg.headRef, cfg.baseRef = cfg.pr.headRef, cfg.pr.baseRef
	}

	env := newEnvFiles(t)
	writeEventPayload(t, env.EventPath, cfg)

	vars := envVars(s, cfg, &env)
	for k, v := range vars {
		t.Setenv(k, v)
	}

	return env
}

// envVars builds the full set of variables SetEnv exports.
func envVars(s *Server, cfg *envConfig, env *Env) map[string]string {
	return map[string]string{
		"GITHUB_ACTIONS":      "true",
		"GITHUB_API_URL":      s.URL(),
		"GITHUB_TOKEN":        cfg.token,
		"GITHUB_REPOSITORY":   cfg.repository,
		"GITHUB_SHA":          cfg.sha,
		"GITHUB_REF":          cfg.ref,
		"GITHUB_REF_NAME":     cfg.refName,
		"GITHUB_HEAD_REF":     cfg.headRef,
		"GITHUB_BASE_REF":     cfg.baseRef,
		"GITHUB_EVENT_NAME":   cfg.eventName,
		"GITHUB_EVENT_PATH":   env.EventPath,
		"GITHUB_RUN_ID":       cfg.runID,
		"GITHUB_RUN_NUMBER":   "7",
		"GITHUB_SERVER_URL":   cfg.serverURL,
		"GITHUB_ACTOR":        "octocat",
		"GITHUB_WORKFLOW":     "CI",
		"GITHUB_JOB":          "plan",
		"GITHUB_OUTPUT":       env.Output,
		"GITHUB_ENV":          env.EnvFile,
		"GITHUB_PATH":         env.Path,
		"GITHUB_STEP_SUMMARY": env.Summary,

		// Blank ambient overrides so a developer's shell cannot redirect the client.
		"ATMOS_CI_GITHUB_API_URL": "",
		"ATMOS_CI_GITHUB_TOKEN":   "",
		"ATMOS_PRO_GITHUB_TOKEN":  "",
		"GH_TOKEN":                "",
	}
}

// newEnvFiles creates the empty runner files under t.TempDir().
func newEnvFiles(t testing.TB) Env {
	t.Helper()

	dir := t.TempDir()
	env := Env{
		Output:    filepath.Join(dir, "github_output"),
		EnvFile:   filepath.Join(dir, "github_env"),
		Path:      filepath.Join(dir, "github_path"),
		Summary:   filepath.Join(dir, "github_step_summary"),
		EventPath: filepath.Join(dir, "event.json"),
	}
	for _, p := range []string{env.Output, env.EnvFile, env.Path, env.Summary} {
		if err := os.WriteFile(p, nil, filePerm); err != nil {
			t.Fatalf("ghtest: creating %s: %v", p, err)
		}
	}

	return env
}

// writeEventPayload writes the event JSON: a generated pull_request payload when
// WithPullRequest was used, overlaid with any WithEvent payload keys.
func writeEventPayload(t testing.TB, path string, cfg *envConfig) {
	t.Helper()

	payload := map[string]any{}
	if cfg.pr != nil {
		payload["action"] = "opened"
		payload["number"] = cfg.pr.number
		payload["pull_request"] = map[string]any{
			"number":   cfg.pr.number,
			"html_url": fmt.Sprintf("%s/%s/pull/%s", cfg.serverURL, cfg.repository, strconv.Itoa(cfg.pr.number)),
			"head":     map[string]any{"ref": cfg.pr.headRef, "sha": cfg.sha},
			"base":     map[string]any{"ref": cfg.pr.baseRef, "sha": defaultBaseSHA},
		}
	}
	for k, v := range cfg.payload {
		payload[k] = v
	}
	if _, ok := payload["repository"]; !ok {
		payload["repository"] = map[string]any{"full_name": cfg.repository}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("ghtest: marshaling event payload: %v", err)
	}
	if err := os.WriteFile(path, data, filePerm); err != nil {
		t.Fatalf("ghtest: writing event payload: %v", err)
	}
}

// ReadFile returns the contents of path, failing the test on error.
func ReadFile(t testing.TB, path string) string {
	defer perf.Track(nil, "ghtest.ReadFile")()

	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ghtest: reading %s: %v", path, err)
	}

	return string(data)
}

// RegisterProvider isolates the CI provider registry for the duration of the test
// and registers p in it (restoring the previous registry in t.Cleanup). Pass a
// freshly constructed github.NewProvider(): the provider caches its API client on
// first use, and the provider registered at init() may already hold a client
// built before SetEnv pointed GITHUB_API_URL at the fake server.
//
// Call SetEnv first (or at least before the provider makes its first API call).
func RegisterProvider(t testing.TB, p ci.Provider) {
	defer perf.Track(nil, "ghtest.RegisterProvider")()

	t.Helper()

	t.Cleanup(ci.SwapRegistryForTest())
	ci.Register(p)
}
