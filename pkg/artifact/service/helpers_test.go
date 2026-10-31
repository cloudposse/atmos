package service_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

// startTime is the fixed clock origin for service tests.
var startTime = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

// fakeClock is a settable clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// recordingJournal records lifecycle events.
type recordingJournal struct {
	mu      sync.Mutex
	entries []service.JournalEntry
}

func (j *recordingJournal) Record(_ context.Context, entry *service.JournalEntry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries = append(j.entries, *entry)
}

func (j *recordingJournal) phases() []service.JournalPhase {
	j.mu.Lock()
	defer j.mu.Unlock()
	phases := make([]service.JournalPhase, 0, len(j.entries))
	for _, entry := range j.entries {
		phases = append(phases, entry.Phase)
	}
	return phases
}

func (j *recordingJournal) snapshot() []service.JournalEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]service.JournalEntry(nil), j.entries...)
}

// temporaryError is a retryable failure.
type temporaryError struct{ message string }

func (e *temporaryError) Error() string   { return e.message }
func (e *temporaryError) Temporary() bool { return true }

// env is a service wired to named in-memory repositories.
type env struct {
	t       *testing.T
	repos   map[string]*artifacttest.Repository
	static  *service.StaticRepositories
	clock   *fakeClock
	journal *recordingJournal
	staging string
	sleeps  []time.Duration
	sleepMu sync.Mutex
	onSleep func(time.Duration)
	svc     *service.Service
}

type envConfig struct {
	repos   map[string]artifacttest.Options
	infos   map[string]service.RepositoryInfo
	options []service.Option
}

// newEnv builds an environment with a "dst" repository by default.
func newEnv(t *testing.T, config envConfig) *env {
	t.Helper()
	if config.repos == nil {
		config.repos = map[string]artifacttest.Options{"dst": {Alias: "dst"}}
	}
	e := &env{
		t:       t,
		repos:   map[string]*artifacttest.Repository{},
		clock:   &fakeClock{now: startTime},
		journal: &recordingJournal{},
		staging: t.TempDir(),
	}
	e.static = &service.StaticRepositories{Repositories: map[string]artifact.Repository{}, Info: config.infos}
	if e.static.Info == nil {
		e.static.Info = map[string]service.RepositoryInfo{}
	}
	for alias, opts := range config.repos {
		opts.Alias = alias
		repo := artifacttest.New(opts)
		e.repos[alias] = repo
		e.static.Repositories[alias] = repo
	}
	var counter atomic.Int64
	options := []service.Option{
		service.WithClock(e.clock.Now),
		service.WithIDGenerator(func() string { return fmt.Sprintf("op-%d", counter.Add(1)) }),
		service.WithJournal(e.journal),
		service.WithSleep(func(_ context.Context, d time.Duration) error {
			e.sleepMu.Lock()
			e.sleeps = append(e.sleeps, d)
			hook := e.onSleep
			e.sleepMu.Unlock()
			if hook != nil {
				hook(d)
			}
			return nil
		}),
		service.WithRetry(3, time.Millisecond),
		service.WithStagingRoot(e.staging),
		service.WithOwner("test-owner"),
	}
	svc, err := service.New(e.static, nil, append(options, config.options...)...)
	require.NoError(t, err)
	e.svc = svc
	return e
}

func (e *env) setInfo(alias string, mutate func(*service.RepositoryInfo)) {
	info, err := e.static.Describe(alias)
	require.NoError(e.t, err)
	mutate(&info)
	e.static.Info[alias] = info
}

// source writes a small file package whose data.txt holds content and returns its directory.
func (e *env) source(content string) string {
	e.t.Helper()
	src := e.t.TempDir()
	fixtures.WriteFile(e.t, src, "data.txt", content, 0o644)
	fixtures.WriteFile(e.t, src, "bin/run.sh", fixtures.HelperScript, 0o755)
	fixtures.WriteFile(e.t, src, fixtures.ReservedDir+"/README.md", "package readme\n", 0o644)
	return src
}

func (e *env) publishRequest(repository, src string) artifact.PublishRequest {
	return artifact.PublishRequest{Repository: repository, Source: src, Name: "tools/example", Version: "1.0.0"}
}

func (e *env) publish(repository, src string) *service.PublishOutcome {
	e.t.Helper()
	outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest(repository, src))
	require.NoError(e.t, err)
	return outcome
}

// assertStagingClean asserts no owned temporary data remains.
func (e *env) assertStagingClean() {
	e.t.Helper()
	items, err := os.ReadDir(e.staging)
	require.NoError(e.t, err)
	assert.Empty(e.t, items, "owned temporary staging must be removed")
}

func ref(name, version string) artifact.Ref {
	return artifact.Ref{Name: name, Version: version}
}

var exampleRef = ref("tools/example", "1.0.0")

// missing reports whether the path does not exist, for readable "nothing was created" assertions.
func missing(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
