package cache_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/discovery/cache"
)

var (
	errOffline  = errors.New("offline")
	errTooLarge = errors.New("too large")
	errFetch    = errors.New("fetch failed")
)

// listing is a minimal snapshot schema: the shared header plus a payload.
type listing struct {
	cache.Header
	Items []string `json:"items"`
}

// clock is a deterministic, settable time source.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var epoch = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// harness counts fetches and authorizations of one source.
type harness struct {
	store      *cache.Store
	clock      *clock
	fetches    atomic.Int64
	authorized []bool
	mu         sync.Mutex
	items      []string
	fail       error
	authorize  error
}

func newHarness(t *testing.T, scope string, opts ...cache.Option) *harness {
	t.Helper()
	h := &harness{clock: &clock{now: epoch}, items: []string{"a"}}
	all := append([]cache.Option{cache.WithClock(h.clock.Now)}, opts...)
	h.store = cache.New(filepath.Join(t.TempDir(), "discovery"), scope, all...)
	return h
}

func (h *harness) source(kind string) *cache.Source[listing, *listing] {
	const reference = "ref"
	return &cache.Source[listing, *listing]{
		Kind: kind, Reference: reference, Version: 1,
		Decode: func(raw []byte) (*listing, error) {
			var value listing
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
			return &value, nil
		},
		Fetch: func(_ context.Context, previous *listing) (*listing, error) {
			h.fetches.Add(1)
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.fail != nil {
				return nil, h.fail
			}
			_ = previous
			return &listing{Items: append([]string(nil), h.items...)}, nil
		},
		Authorize: func(_ context.Context, cached bool) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.authorized = append(h.authorized, cached)
			return h.authorize
		},
		ErrOffline: errOffline, ErrTooLarge: errTooLarge,
	}
}

func (h *harness) load(t *testing.T, source *cache.Source[listing, *listing], mode cache.Mode) (*listing, bool, error) {
	t.Helper()
	return cache.Load(t.Context(), h.store, source, mode)
}

func TestLoadFreshSnapshotIsServedWithoutFetching(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	first, cached, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	assert.False(t, cached)
	assert.Equal(t, []string{"a"}, first.Items)
	assert.Equal(t, epoch, first.FetchedAt, "the fetch time comes from the injected clock, in UTC")

	h.clock.advance(cache.DefaultTTL - time.Second)
	second, cached, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	assert.True(t, cached)
	assert.Equal(t, first.Items, second.Items)
	assert.Equal(t, int64(1), h.fetches.Load(), "a fresh snapshot performs no fetch")
	assert.Equal(t, []bool{false, true}, h.authorized, "policy is evaluated for the fetch and again for the cached read")
}

func TestLoadStaleSnapshotRefreshesAtTheTTLBoundary(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)

	h.clock.advance(cache.DefaultTTL)
	h.mu.Lock()
	h.items = []string{"a", "b"}
	h.mu.Unlock()
	refreshed, cached, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	assert.False(t, cached, "a snapshot exactly one TTL old is stale")
	assert.Equal(t, []string{"a", "b"}, refreshed.Items)
	assert.Equal(t, int64(2), h.fetches.Load())
}

func TestLoadRefreshBypassesFreshness(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	_, cached, err := h.load(t, source, cache.Mode{Refresh: true})
	require.NoError(t, err)
	assert.False(t, cached)
	assert.Equal(t, int64(2), h.fetches.Load())
}

func TestLoadZeroTTLRevalidatesEveryOnlineLoad(t *testing.T) {
	h := newHarness(t, "scope", cache.WithTTL(0))
	source := h.source("repository")
	for range 3 {
		_, cached, err := h.load(t, source, cache.Mode{})
		require.NoError(t, err)
		assert.False(t, cached)
	}
	assert.Equal(t, int64(3), h.fetches.Load())
}

func TestLoadOfflineServesAnySnapshotAgeButNeverFetches(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	h.clock.advance(1000 * time.Hour)
	value, cached, err := h.load(t, source, cache.Mode{Offline: true})
	require.NoError(t, err)
	assert.True(t, cached)
	assert.Equal(t, []string{"a"}, value.Items)
	assert.Equal(t, int64(1), h.fetches.Load())
}

func TestLoadOfflineFailsWithoutAValidSnapshot(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		h := newHarness(t, "scope")
		_, _, err := h.load(t, h.source("repository"), cache.Mode{Offline: true})
		require.ErrorIs(t, err, errOffline)
		assert.Zero(t, h.fetches.Load())
	})
	t.Run("refresh requested", func(t *testing.T) {
		h := newHarness(t, "scope")
		source := h.source("repository")
		_, _, err := h.load(t, source, cache.Mode{})
		require.NoError(t, err)
		_, _, err = h.load(t, source, cache.Mode{Offline: true, Refresh: true})
		// Offline wins over the age: the existing snapshot is served and no fetch happens.
		require.NoError(t, err)
		assert.Equal(t, int64(1), h.fetches.Load())
	})
}

func TestLoadRejectsCorruptSnapshotsOfflineAndReplacesThemOnline(t *testing.T) {
	for name, corrupt := range map[string]string{
		"not json":        `{`,
		"other version":   `{"version":2,"source":"ref","kind":"repository","fetched_at":"2026-03-04T05:06:07Z"}`,
		"other source":    `{"version":1,"source":"other","kind":"repository","fetched_at":"2026-03-04T05:06:07Z"}`,
		"other kind":      `{"version":1,"source":"ref","kind":"catalog","fetched_at":"2026-03-04T05:06:07Z"}`,
		"oversized":       strings.Repeat(" ", 128),
		"empty":           ``,
		"truncated array": `{"version":1,"source":"ref","kind":"repository","items":[`,
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, "scope", cache.WithMaxSize(100))
			source := h.source("repository")
			require.NoError(t, os.MkdirAll(h.store.Directory(), cache.DirectoryMode))
			require.NoError(t, os.WriteFile(h.store.Path("repository", "ref"), []byte(corrupt), cache.FileMode))

			_, _, err := h.load(t, source, cache.Mode{Offline: true})
			require.ErrorIs(t, err, errOffline, "offline never trusts a corrupt snapshot")
			assert.Zero(t, h.fetches.Load())

			value, cached, err := h.load(t, source, cache.Mode{})
			require.NoError(t, err)
			assert.False(t, cached)
			assert.Equal(t, []string{"a"}, value.Items)
			_, cached, err = h.load(t, source, cache.Mode{Offline: true})
			require.NoError(t, err, "the replacement is a valid snapshot")
			assert.True(t, cached)
		})
	}
}

func TestLoadDecoderRejectionIsTreatedAsAbsent(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	source.Decode = func([]byte) (*listing, error) { return nil, errors.New("content validation failed") }
	_, _, err = h.load(t, source, cache.Mode{Offline: true})
	require.ErrorIs(t, err, errOffline)
}

func TestLoadFailedFetchKeepsThePreviousSnapshot(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	before, err := os.ReadFile(h.store.Path("repository", "ref"))
	require.NoError(t, err)

	h.mu.Lock()
	h.fail = errFetch
	h.mu.Unlock()
	_, _, err = h.load(t, source, cache.Mode{Refresh: true})
	require.ErrorIs(t, err, errFetch)
	after, err := os.ReadFile(h.store.Path("repository", "ref"))
	require.NoError(t, err)
	assert.Equal(t, before, after, "a failed refresh never replaces or truncates the snapshot")
}

func TestLoadFailedFirstFetchCachesNothing(t *testing.T) {
	h := newHarness(t, "scope")
	h.fail = errFetch
	_, _, err := h.load(t, h.source("repository"), cache.Mode{})
	require.ErrorIs(t, err, errFetch)
	files, globErr := filepath.Glob(filepath.Join(h.store.Directory(), "*.json"))
	require.NoError(t, globErr)
	assert.Empty(t, files)
}

func TestLoadUnavailableOptionalSourceIsNotCached(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("catalog")
	source.Fetch = func(context.Context, *listing) (*listing, error) { return nil, nil }
	value, cached, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	assert.Nil(t, value)
	assert.False(t, cached)
	files, globErr := filepath.Glob(filepath.Join(h.store.Directory(), "*.json"))
	require.NoError(t, globErr)
	assert.Empty(t, files, "an unavailable optional source is never cached as an empty result")
}

func TestLoadPolicyRunsOnCachedReadsAndCanRejectThem(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)

	policy := errors.New("source no longer permitted")
	h.mu.Lock()
	h.authorize = policy
	h.mu.Unlock()
	_, _, err = h.load(t, source, cache.Mode{})
	require.ErrorIs(t, err, policy, "a fresh snapshot is not an authorization")
	_, _, err = h.load(t, source, cache.Mode{Offline: true})
	require.ErrorIs(t, err, policy, "offline reads re-evaluate the policy too")
	assert.Equal(t, int64(1), h.fetches.Load())
}

func TestLoadPolicyRejectionBlocksTheFetch(t *testing.T) {
	h := newHarness(t, "scope")
	h.authorize = errors.New("denied")
	_, _, err := h.load(t, h.source("repository"), cache.Mode{})
	require.Error(t, err)
	assert.Zero(t, h.fetches.Load(), "the source is not contacted when policy rejects it")
}

func TestLoadOversizedFetchFailsWithoutReplacingTheFile(t *testing.T) {
	h := newHarness(t, "scope", cache.WithMaxSize(200))
	source := h.source("repository")
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	before, err := os.ReadFile(h.store.Path("repository", "ref"))
	require.NoError(t, err)

	h.mu.Lock()
	h.items = []string{strings.Repeat("x", 500)}
	h.mu.Unlock()
	_, _, err = h.load(t, source, cache.Mode{Refresh: true})
	require.ErrorIs(t, err, errTooLarge)
	after, err := os.ReadFile(h.store.Path("repository", "ref"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestLoadCanceledContextWritesNothing(t *testing.T) {
	h := newHarness(t, "scope")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := cache.Load(ctx, h.store, h.source("repository"), cache.Mode{})
	require.Error(t, err)
	files, globErr := filepath.Glob(filepath.Join(h.store.Directory(), "*.json"))
	require.NoError(t, globErr)
	assert.Empty(t, files)
}

func TestLoadConcurrentColdLoadsShareOneFetch(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	const workers = 12
	start := make(chan struct{})
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			<-start
			_, _, err := cache.Load(t.Context(), h.store, source, cache.Mode{})
			errs <- err
		})
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int64(1), h.fetches.Load())
}

func TestLoadPassesThePreviousSnapshotToFetch(t *testing.T) {
	h := newHarness(t, "scope")
	source := h.source("repository")
	var seen []*listing
	inner := source.Fetch
	source.Fetch = func(ctx context.Context, previous *listing) (*listing, error) {
		seen = append(seen, previous)
		return inner(ctx, previous)
	}
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)
	_, _, err = h.load(t, source, cache.Mode{Refresh: true})
	require.NoError(t, err)
	require.Len(t, seen, 2)
	assert.Nil(t, seen[0])
	require.NotNil(t, seen[1])
	assert.Equal(t, []string{"a"}, seen[1].Items)
}

func TestScopeSeparatesSnapshotsAndIsDeterministic(t *testing.T) {
	first, err := cache.Scope(map[string]string{"realm": "a"}, "x", []string{"h"})
	require.NoError(t, err)
	again, err := cache.Scope(map[string]string{"realm": "a"}, "x", []string{"h"})
	require.NoError(t, err)
	other, err := cache.Scope(map[string]string{"realm": "b"}, "x", []string{"h"})
	require.NoError(t, err)
	assert.Equal(t, first, again)
	assert.NotEqual(t, first, other)

	dir := filepath.Join(t.TempDir(), "discovery")
	a := cache.New(dir, first)
	b := cache.New(dir, other)
	assert.NotEqual(t, a.Path("repository", "ref"), b.Path("repository", "ref"))
	assert.NotEqual(t, a.Path("repository", "ref"), a.Path("catalog", "ref"))
	assert.NotEqual(t, a.Path("repository", "ref"), a.Path("repository", "ref2"))
	assert.Equal(t, a.Path("repository", "ref"), cache.New(dir, first).Path("repository", "ref"))
	assert.Equal(t, filepath.Join(dir, cache.Key(first, "repository", "ref")+".json"), a.Path("repository", "ref"))

	_, err = cache.Scope(make(chan int))
	require.Error(t, err)
}

func TestSnapshotsInDifferentScopesNeverCrossOver(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "discovery")
	h := newHarness(t, "scope-a")
	h.store = cache.New(dir, "scope-a", cache.WithClock(h.clock.Now))
	source := h.source("repository")
	_, _, err := h.load(t, source, cache.Mode{})
	require.NoError(t, err)

	h.store = cache.New(dir, "scope-b", cache.WithClock(h.clock.Now))
	_, _, err = h.load(t, source, cache.Mode{Offline: true})
	require.ErrorIs(t, err, errOffline, "another scope cannot reuse the snapshot")
}

func TestWriteSnapshotIsAtomicAndSizeBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	original := []byte("previous verified snapshot")
	require.NoError(t, os.WriteFile(path, original, cache.FileMode))
	store := cache.New("", "", cache.WithMaxSize(128))

	require.ErrorIs(t, store.WriteSnapshot(path, listing{Items: []string{strings.Repeat("x", 200)}}, errTooLarge), errTooLarge)
	require.Error(t, store.WriteSnapshot(path, listing{Header: cache.Header{FetchedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}, errTooLarge))
	current, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, current, "a rejected write leaves the previous contents in place")

	require.NoError(t, store.WriteSnapshot(path, listing{Items: []string{"ok"}}, errTooLarge))
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(written), `"items":["ok"]`)
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(cache.FileMode), info.Mode().Perm())
	}
}

func TestLoadFailsWhenTheDirectoryCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "occupied")
	require.NoError(t, os.WriteFile(blocker, []byte("file"), cache.FileMode))
	store := cache.New(filepath.Join(blocker, "discovery"), "scope")
	h := newHarness(t, "scope")
	_, _, err := cache.Load(t.Context(), store, h.source("repository"), cache.Mode{})
	require.Error(t, err)
	assert.Zero(t, h.fetches.Load(), "nothing is fetched when the cache cannot be written")
}

func TestStoreAccessorsAndHeaderRoundTrip(t *testing.T) {
	store := cache.New("dir", "scope", cache.WithTTL(time.Minute))
	assert.Equal(t, "dir", store.Directory())
	assert.Equal(t, time.Minute, store.TTL())
	assert.Equal(t, cache.DefaultTTL, cache.New("dir", "scope").TTL())

	var header cache.Header
	header.SetSnapshotHeader(cache.Header{Version: 3, Source: "s", Kind: "k", FetchedAt: epoch})
	assert.Equal(t, cache.Header{Version: 3, Source: "s", Kind: "k", FetchedAt: epoch}, header.SnapshotHeader())
	assert.Len(t, cache.Hash([]byte("x")), 64)
}

func TestFreshnessRejectsFutureTimestamps(t *testing.T) {
	h := newHarness(t, "scope")
	assert.True(t, h.store.Fresh(epoch.Add(-time.Minute)))
	assert.False(t, h.store.Fresh(epoch.Add(time.Minute)), "a snapshot from the future is never fresh")
	assert.False(t, h.store.Fresh(epoch.Add(-cache.DefaultTTL)))
}
