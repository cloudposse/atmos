package exec

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	gh "github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// captureLog redirects the default logger output to a buffer for the duration
// of the test at the requested level, and fully restores the original writer
// and log level on cleanup so other tests in the same binary are unaffected.
//
// NOTE: This function operates on the *current* default logger (via log.Default()).
// If log.SetDefault() is called between captureLog and the t.Cleanup restore, the
// restore will apply to the logger that was default at call time, not the new default.
// To avoid this, tests that call captureLog must not call log.SetDefault().
func captureLog(t *testing.T, level log.Level) *bytes.Buffer {
	t.Helper()

	origLevel := log.GetLevel()
	origWriter := log.GetOutput()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(level)

	t.Cleanup(func() {
		log.SetOutput(origWriter)
		log.SetLevel(origLevel)
	})

	return &buf
}

// resetWarnedRepos clears the per-run warning-deduplication map so tests don't
// interfere with each other.
func resetWarnedRepos() {
	warnedArchivedRepos.Range(func(k, _ any) bool {
		warnedArchivedRepos.Delete(k)
		return true
	})
}

func TestWarnIfArchivedGitHubRepo(t *testing.T) {
	tests := []struct {
		name          string
		uri           string
		component     string
		seedArchived  *bool // nil means no cache seed (real API call avoided via non-GitHub URI)
		cancelCtx     bool  // pass a cancelled context to force an API error
		wantWarn      bool
		wantTrace     bool // expect a trace-level "Skipping" message
		wantComponent bool
	}{
		{
			name:         "non-GitHub URI — no warning, no API call",
			uri:          "oci://registry.example.com/org/image:tag",
			seedArchived: nil,
			wantWarn:     false,
		},
		{
			name:         "local path — no warning, no API call",
			uri:          "./local/path",
			seedArchived: nil,
			wantWarn:     false,
		},
		{
			name:      "archived GitHub repo — warning without component",
			uri:       "github.com/cloudposse/archived-repo",
			component: "",
			seedArchived: func() *bool {
				v := true
				return &v
			}(),
			wantWarn:      true,
			wantComponent: false,
		},
		{
			name:      "archived GitHub repo — warning with component name",
			uri:       "github.com/cloudposse/archived-repo2",
			component: "my-component",
			seedArchived: func() *bool {
				v := true
				return &v
			}(),
			wantWarn:      true,
			wantComponent: true,
		},
		{
			name:      "active GitHub repo — no warning",
			uri:       "github.com/cloudposse/active-repo",
			component: "active",
			seedArchived: func() *bool {
				v := false
				return &v
			}(),
			wantWarn: false,
		},
		{
			name:      "API error (cancelled context) — no warning, trace logged",
			uri:       "github.com/cloudposse/error-repo",
			cancelCtx: true,
			wantWarn:  false,
			wantTrace: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Register cleanup before any state mutation so that even a panic or
			// early t.Fatal leaves a clean environment for subsequent sub-tests.
			t.Cleanup(resetWarnedRepos)
			t.Cleanup(gh.ResetArchivedRepoCache)

			// Pre-populate the cache so no real API call is made.
			if tt.seedArchived != nil {
				owner, repo, ok := gh.ParseGitHubOwnerRepo(tt.uri)
				if ok {
					gh.SeedArchivedRepoCache(owner, repo, *tt.seedArchived)
				}
			}

			// Capture at Trace level so both warnings and trace messages are visible.
			buf := captureLog(t, log.TraceLevel)

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel() // cancelled immediately to force API error
			}

			warnIfArchivedGitHubRepo(ctx, tt.uri, tt.component)

			output := buf.String()

			if tt.wantWarn {
				assert.Contains(t, output, "GitHub repository is archived", "expected archived warning in log output")
				owner, repo, _ := gh.ParseGitHubOwnerRepo(tt.uri)
				assert.Contains(t, output, owner+"/"+repo, "expected repo name in log output")
			} else {
				assert.NotContains(t, output, "GitHub repository is archived", "did not expect archived warning")
			}

			if tt.wantTrace {
				assert.Contains(t, output, "Skipping archived-repo check", "expected trace-level skip message")
			}

			if tt.wantComponent {
				assert.Contains(t, output, tt.component, "expected component name in log output")
			}
		})
	}
}

// TestWarnIfArchivedGitHubRepo_Deduplication verifies that the same archived repo
// only emits a warning once, even when called from multiple code paths (vendor.yaml
// and component.yaml both referencing the same repository).
func TestWarnIfArchivedGitHubRepo_Deduplication(t *testing.T) {
	// Register cleanup before any state mutation so that even a panic or
	// early t.Fatal leaves a clean environment for subsequent tests.
	t.Cleanup(resetWarnedRepos)
	t.Cleanup(gh.ResetArchivedRepoCache)

	const uri = "github.com/cloudposse/dedup-repo"
	owner, repo, ok := gh.ParseGitHubOwnerRepo(uri)
	assert.True(t, ok)

	gh.SeedArchivedRepoCache(owner, repo, true)

	buf := captureLog(t, log.TraceLevel)

	// Call twice (simulating vendor.yaml + component.yaml).
	warnIfArchivedGitHubRepo(context.Background(), uri, "comp-a")
	warnIfArchivedGitHubRepo(context.Background(), uri, "comp-b")

	output := buf.String()

	// Count occurrences of the warning message: exactly one.
	count := bytes.Count([]byte(output), []byte("GitHub repository is archived"))
	assert.Equal(t, 1, count, "expected exactly one warning for the same archived repo")

	// The second call (comp-b) must emit a trace-level suppression note.
	assert.Contains(t, output, "Archived-repo warning already emitted", "expected dedup trace log for comp-b")
	assert.Contains(t, output, "comp-b", "expected suppressed component name in dedup trace log")
}

// TestWarnIfArchivedGitHubRepo_CachedArchivedRepo verifies that warnIfArchivedGitHubRepo
// emits a warning when the repo is already marked archived in the cache. This complements
// the full HTTP-path test in pkg/github/repo_test.go (TestIsRepoArchived_ViaHook) which
// exercises the IsRepoArchived → HTTP transport → archived=true path end-to-end.
//
// The HTTP-layer e2e test was moved to pkg/github because SetNewGitHubClientHookForTest
// lives in pkg/github/export_test.go and is only compiled during pkg/github tests.
// Tests in internal/exec rely on cache-seeding (SeedArchivedRepoCache) for full
// warnIfArchivedGitHubRepo coverage.
func TestWarnIfArchivedGitHubRepo_CachedArchivedRepo(t *testing.T) {
	const (
		owner = "cached-owner"
		repo  = "cached-repo"
	)

	t.Cleanup(resetWarnedRepos)
	t.Cleanup(gh.ResetArchivedRepoCache)

	// Pre-seed the cache so no real API call is made.
	gh.SeedArchivedRepoCache(owner, repo, true)

	buf := captureLog(t, log.TraceLevel)

	uri := "github.com/" + owner + "/" + repo
	warnIfArchivedGitHubRepo(context.Background(), uri, "my-component")

	output := buf.String()
	assert.Contains(t, output, "GitHub repository is archived",
		"expected archived warning for cache-seeded archived repo")
	assert.Contains(t, output, owner+"/"+repo,
		"expected repo name in log output")
	assert.Contains(t, output, "my-component",
		"expected component name in log output")
}
