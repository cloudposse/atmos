package versionnotice

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	ghprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/installer"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

var _ = schema.VersionCheck{CI: schema.VersionCheckCI{Enabled: true, MinorThreshold: 10}}

type streams struct{ stdout, stderr *bytes.Buffer }

func (s streams) Input() io.Reader     { return &bytes.Buffer{} }
func (s streams) Output() io.Writer    { return s.stdout }
func (s streams) Error() io.Writer     { return s.stderr }
func (s streams) RawOutput() io.Writer { return s.stdout }
func (s streams) RawError() io.Writer  { return s.stderr }

func setup(t *testing.T) (schema.AtmosConfiguration, streams) {
	t.Helper()
	t.Setenv(sentinel, "")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY", "cloudposse/atmos")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("GITHUB_JOB", "test")
	t.Setenv("RUNNER_TEMP", t.TempDir())
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "env"))
	t.Setenv("ATMOS_CI_ENV", "")
	t.Setenv("NO_COLOR", "1")
	t.Cleanup(ci.SwapRegistryForTest())
	ci.Register(ghprovider.NewProvider())
	output := streams{&bytes.Buffer{}, &bytes.Buffer{}}
	ctx, err := iolib.NewContext(iolib.WithStreams(output))
	require.NoError(t, err)
	ui.InitFormatter(ctx)
	data.InitWriter(ctx)
	config := schema.AtmosConfiguration{}
	config.Version.Check = schema.VersionCheck{Enabled: true, Timeout: 1000, Frequency: "daily", CI: schema.VersionCheckCI{Enabled: true, MinorThreshold: 10}}
	return config, output
}

type fakeClient struct {
	budgetErr error
	latest    string
	calls     int
}

func (c *fakeClient) CheckBudget(context.Context) error { return c.budgetErr }
func (c *fakeClient) Latest(context.Context, string, string) (string, error) {
	c.calls++
	return c.latest, nil
}

func fakeDependencies(c *fakeClient) dependencies {
	return dependencies{
		isCI:   func() bool { return true },
		client: func(context.Context) (releaseClient, bool) { return c, true },
		load:   func(context.Context) (cfg.CacheConfig, error) { return cfg.CacheConfig{}, nil },
		update: func(context.Context, func(*cfg.CacheConfig)) error { return nil },
		hint:   func(context.Context, string, string) string { return "Run brew upgrade atmos." },
	}
}

func settings() noticeSettings {
	return noticeSettings{current: "1.100.0", threshold: 10, frequency: "daily"}
}

func TestGates(t *testing.T) {
	for _, name := range []string{"not CI", "master disabled", "CI disabled", "unauthenticated", "sentinel", "marker", "frequency", "cache error", "budget"} {
		t.Run(name, func(t *testing.T) {
			config, out := setup(t)
			client := &fakeClient{latest: "v1.110.0"}
			deps := fakeDependencies(client)
			switch name {
			case "not CI":
				deps.isCI = func() bool { return false }
			case "master disabled":
				config.Version.Check.Enabled = false
			case "CI disabled":
				config.Version.Check.CI.Enabled = false
			case "unauthenticated":
				deps.client = func(context.Context) (releaseClient, bool) { return client, false }
			case "sentinel":
				t.Setenv(sentinel, "1")
			case "marker":
				require.True(t, claimMarker(jobMarker()))
			case "frequency":
				deps.load = func(context.Context) (cfg.CacheConfig, error) {
					return cfg.CacheConfig{LastChecked: time.Now().Unix()}, nil
				}
			case "cache error":
				deps.load = func(context.Context) (cfg.CacheConfig, error) {
					return cfg.CacheConfig{}, errors.New("cache unavailable")
				}
			case "budget":
				client.budgetErr = errors.New("budget too low")
			}
			if !deps.isCI() || !config.Version.Check.Enabled || !config.Version.Check.CI.Enabled {
				deps.client = func(context.Context) (releaseClient, bool) {
					t.Error("auth lookup before enable gates")
					return client, true
				}
				start(&config, "1.100.0", deps)()
			} else {
				_, ok := prepare(context.Background(), settings(), deps)
				assert.False(t, ok)
			}
			assert.Zero(t, client.calls)
			assert.Empty(t, out.stdout.String())
			assert.Empty(t, out.stderr.String())
		})
	}
}

func TestComparisonAndOutput(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"1.101.0", "1.110.0", false},
		{"1.100.0", "1.110.0", true},
		{"v1.100.0", "v1.110.0", true},
		{"1.100.0-rc.1", "1.110.0", false},
		{"dev", "1.110.0", false},
		{"1.100.0+dev", "1.110.0", false},
		{"1.100.0", "1.110.0-rc.1", false},
		{"1.100.0", "invalid", false},
		{"1.100.0", "1.100.99", false},
		{"1.120.0", "1.110.0", false},
		{"1.100.0", "2.110.0", false},
	} {
		t.Run(tc.current+"_"+tc.latest, func(t *testing.T) {
			_, out := setup(t)
			deps := fakeDependencies(&fakeClient{latest: tc.latest})
			s := settings()
			s.current = tc.current
			notice, ok := prepare(context.Background(), s, deps)
			require.Equal(t, tc.want, ok)
			if ok {
				emit(&notice)
				assert.Contains(t, out.stderr.String(), "::warning title=Atmos 1.100.0 is 10 releases behind::Latest is 1.110.0. Run brew upgrade atmos.")
				assert.Equal(t, "1", os.Getenv(sentinel))
				exported, err := os.ReadFile(os.Getenv("GITHUB_ENV"))
				require.NoError(t, err)
				assert.Contains(t, string(exported), sentinel+"=1")
				emit(&notice)
				assert.Equal(t, 1, bytes.Count(out.stderr.Bytes(), []byte("::warning")))
				// An independent process has no inherited sentinel but shares the job marker.
				t.Setenv(sentinel, "")
				emit(&notice)
				assert.Equal(t, 1, bytes.Count(out.stderr.Bytes(), []byte("::warning")))
			} else {
				assert.Empty(t, out.stderr.String())
			}
			assert.Empty(t, out.stdout.String())
		})
	}
}

func TestFinishDoesNotWaitOrEmitLate(t *testing.T) {
	for _, phase := range []string{"auth", "installer"} {
		t.Run(phase, func(t *testing.T) {
			config, out := setup(t)
			deps := fakeDependencies(&fakeClient{latest: "1.110.0"})
			entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
			block := func() { close(entered); <-release; close(exited) }
			if phase == "auth" {
				deps.client = func(context.Context) (releaseClient, bool) { block(); return &fakeClient{}, false }
			} else {
				deps.hint = func(context.Context, string, string) string { block(); return "upgrade" }
			}
			finish := start(&config, "1.100.0", deps)
			<-entered
			returned := make(chan struct{})
			go func() { finish(); close(returned) }()
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("command exit waited for worker")
			}
			close(release)
			<-exited
			finish()
			assert.Empty(t, out.stdout.String())
			assert.Empty(t, out.stderr.String())
			_, err := os.Stat(jobMarker())
			assert.True(t, os.IsNotExist(err))
		})
	}
}

func TestReadyResultSurvivesDeadline(t *testing.T) {
	config, out := setup(t)
	config.Version.Check.Timeout = 20
	deps := fakeDependencies(&fakeClient{latest: "1.110.0"})
	done := make(chan struct{})
	deps.update = func(context.Context, func(*cfg.CacheConfig)) error { close(done); return nil }
	finish := start(&config, "1.100.0", deps)
	<-done
	// A completed check remains useful when the command itself takes longer.
	time.Sleep(50 * time.Millisecond)
	finish()
	assert.Contains(t, out.stderr.String(), "::warning")
	assert.Empty(t, out.stdout.String())
}

func TestHardDeadline(t *testing.T) {
	config, out := setup(t)
	config.Version.Check.Timeout = 10
	canceled := make(chan struct{})
	deps := fakeDependencies(&fakeClient{})
	deps.client = func(ctx context.Context) (releaseClient, bool) { <-ctx.Done(); close(canceled); return nil, false }
	finish := start(&config, "1.100.0", deps)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("auth lookup did not receive deadline")
	}
	finish()
	assert.Empty(t, out.stderr.String())
}

func TestAtomicMarkerAndJobIsolation(t *testing.T) {
	setup(t)
	path := jobMarker()
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if claimMarker(path) {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, winners.Load())
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")
	assert.NotEqual(t, path, jobMarker())
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("GITHUB_JOB", "other")
	assert.NotEqual(t, path, jobMarker())
	assert.False(t, claimMarker(filepath.Join(t.TempDir(), "absent", "marker")))
}

func tlsMock(t *testing.T) *httpmock.GitHubMockServer {
	t.Helper()
	mock := httpmock.NewGitHubMockServer(t)
	handler := mock.Server.Config.Handler
	mock.Server.Close()
	mock.Server = httptest.NewTLSServer(handler)
	t.Cleanup(mock.Server.Close)
	transport := http.DefaultTransport
	http.DefaultTransport = mock.Server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = transport })
	mock.Setenv(t)
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_CLI", "")
	return mock
}

func TestRealReleaseClient(t *testing.T) {
	for _, remaining := range []int{3749, 3750} {
		t.Run(strconv.Itoa(remaining), func(t *testing.T) {
			_, out := setup(t)
			mock := tlsMock(t)
			mock.RegisterRelease("cloudposse", "atmos", httpmock.ReleaseSpec{TagName: "v1.110.0"})
			mock.SetRateLimit(remaining, time.Now().Add(time.Hour))
			deps := defaultDependencies()
			deps.load = func(context.Context) (cfg.CacheConfig, error) { return cfg.CacheConfig{}, nil }
			deps.update = func(context.Context, func(*cfg.CacheConfig)) error { return nil }
			deps.hint = fakeDependencies(nil).hint
			notice, ok := prepare(context.Background(), settings(), deps)
			assert.Equal(t, remaining == 3750, ok)
			requests := mock.Requests()
			if ok {
				require.Len(t, requests, 2)
				emit(&notice)
				assert.Contains(t, out.stderr.String(), "::warning")
			} else {
				require.Len(t, requests, 1)
				assert.Empty(t, out.stderr.String())
			}
			assert.Equal(t, "/api/v3/rate_limit", requests[0].Path)
			assert.Empty(t, out.stdout.String())
		})
	}
}

func TestSlowServerDoesNotDelayExit(t *testing.T) {
	config, out := setup(t)
	mock := tlsMock(t)
	mock.RegisterRelease("cloudposse", "atmos", httpmock.ReleaseSpec{TagName: "v1.110.0"})
	entered, canceled := make(chan struct{}), make(chan struct{})
	handler := mock.Server.Config.Handler
	mock.Server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			close(entered)
			<-r.Context().Done()
			close(canceled)
			return
		}
		handler.ServeHTTP(w, r)
	})
	deps := defaultDependencies()
	deps.load = fakeDependencies(nil).load
	deps.update = func(context.Context, func(*cfg.CacheConfig)) error {
		t.Error("canceled fetch must not update cache")
		return nil
	}
	finish := start(&config, "1.100.0", deps)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("release request did not start")
	}
	finish()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("command exit did not cancel HTTP request")
	}
	assert.Empty(t, out.stdout.String())
	assert.Empty(t, out.stderr.String())
}

func TestNestedInvocationSentinel(t *testing.T) {
	if os.Getenv("ATMOS_TEST_VERSION_NOTICE_CHILD") == "1" {
		require.Equal(t, "1", os.Getenv(sentinel), "prerequisite: subprocess inherited sentinel")
		deps := fakeDependencies(&fakeClient{latest: "1.110.0"})
		deps.load = func(context.Context) (cfg.CacheConfig, error) {
			t.Fatal("nested invocation reached cache")
			return cfg.CacheConfig{}, nil
		}
		_, ok := prepare(context.Background(), settings(), deps)
		require.False(t, ok)
		return
	}
	_, out := setup(t)
	deps := fakeDependencies(&fakeClient{latest: "1.110.0"})
	notice, ok := prepare(context.Background(), settings(), deps)
	require.True(t, ok)
	emit(&notice)
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestNestedInvocationSentinel$")
	command.Env = append(os.Environ(), "ATMOS_TEST_VERSION_NOTICE_CHILD=1")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	assert.NotContains(t, string(output), "::warning")
	assert.Equal(t, 1, bytes.Count(out.stderr.Bytes(), []byte("::warning")))
}

func TestGenericCIOutput(t *testing.T) {
	_, out := setup(t)
	t.Setenv("GITHUB_ACTIONS", "false")
	t.Setenv("GITHUB_ENV", "")
	ci.Register(generic.NewProvider())
	emit(&result{annotation: provider.Annotation{Level: provider.AnnotationWarning, Title: "Atmos 1.100.0 is 10 releases behind", Message: "Latest is 1.110.0."}})
	assert.Empty(t, out.stdout.String())
	assert.Contains(t, out.stderr.String(), "Atmos 1.100.0 is 10 releases behind")
	assert.NotContains(t, out.stderr.String(), "export ")
}

func TestHintPreservesConditions(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "If available in repositories: Run: apt upgrade atmos Pin the version. More info: https://atmos.tools/install", formatHint(installer.Hint{
		Condition: "If available in repositories", Command: "apt upgrade atmos", Message: "Pin the version.", URL: "https://atmos.tools/install",
	}))
}

func TestJobIdentityFallbacks(t *testing.T) {
	setup(t)
	t.Setenv("RUNNER_TEMP", "")
	assert.Empty(t, jobMarker())
	t.Setenv("GITHUB_JOB", "")
	assert.Empty(t, jobMarker())
	t.Setenv("GITHUB_ACTIONS", "false")
	for _, key := range []string{"CI_JOB_ID", "BUILDKITE_JOB_ID", "CIRCLE_WORKFLOW_JOB_ID", "BUILD_TAG", "ATMOS_CI_RUN_ID"} {
		t.Setenv(key, "")
	}
	assert.Empty(t, jobMarker())
	t.Setenv("CI_JOB_ID", "job-one")
	first := jobMarker()
	require.NotEmpty(t, first)
	t.Setenv("CI_JOB_ID", "job-two")
	assert.NotEqual(t, first, jobMarker())
	// A NUL makes Stat fail on every platform without looking like an absent marker.
	assert.True(t, alreadyShown(filepath.Join(t.TempDir(), "marker")+"\x00"))
	// Windows reports a file parent as not found; Unix reports ENOTDIR.
	// In either case, the final claim must prevent emission.
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.False(t, claimMarker(filepath.Join(file, "marker")))
}

func TestPublicStartAndInstallerHint(t *testing.T) {
	config, out := setup(t)
	config.Version.Check.Enabled = false
	Start(&config)()
	Start(nil)()
	assert.Empty(t, out.stderr.String())
	assert.Contains(t, upgradeHint(context.Background(), "", "1.110.0"), "https://atmos.tools/install")
}

func TestInvalidSettingsAndCacheFailure(t *testing.T) {
	config, out := setup(t)
	config.Version.Check.Timeout = -1
	config.Version.Check.CI.MinorThreshold = 0
	config.Version.Check.Frequency = "invalid"
	config.BasePathAbsolute = t.TempDir()
	config.Toolchain.InstallPath = "tools"
	gotSettings, timeout := normalizeSettings(&config, "1.100.0")
	assert.Equal(t, time.Second, timeout)
	assert.EqualValues(t, 10, gotSettings.threshold)
	assert.Equal(t, filepath.Join(config.BasePathAbsolute, "tools"), gotSettings.nativeRoot)
	deps := fakeDependencies(&fakeClient{latest: "1.110.0"})
	// A failed cache save must not discard a completed result or fail the command.
	deps.update = func(context.Context, func(*cfg.CacheConfig)) error { return errors.New("read-only cache") }
	notice, ok := prepare(context.Background(), gotSettings, deps)
	require.True(t, ok)
	// An unwritable environment export must not add noisy diagnostics or stdout.
	t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "absent", "env"))
	emit(&notice)
	assert.Contains(t, out.stderr.String(), "::warning")
	assert.NotContains(t, out.stderr.String(), "error")
	assert.Empty(t, out.stdout.String())
}
