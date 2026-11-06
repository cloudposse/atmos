package render

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A slow datasource must not hold rendererMu while it is read, or it would
// block every other render's datasource reads until it finishes.
func TestDatasourceReadDoesNotHoldRendererLock(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHandler)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"slow"}`))
	}))
	t.Cleanup(server.Close)

	d, err := NewData([]string{"slow=" + server.URL + "/data.json"}, nil)
	require.NoError(t, err)

	type readResult struct {
		value any
		err   error
	}
	done := make(chan readResult, 1)
	go func() {
		value, err := d.Datasource("slow")
		done <- readResult{value, err}
	}()

	// Wait for the request to reach the handler; a read that fails first, or
	// never arrives, ends the test instead of hanging it.
	select {
	case <-started:
	case result := <-done:
		t.Fatalf("the read finished before reaching the server: %v", result.err)
	case <-time.After(10 * time.Second):
		t.Fatal("the read did not reach the server")
	}
	locked := rendererMu.TryLock()
	if locked {
		rendererMu.Unlock()
	}
	releaseHandler()

	var result readResult
	select {
	case result = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the read did not finish after the server responded")
	}
	assert.True(t, locked, "rendererMu was held during the datasource read")
	require.NoError(t, result.err)
	assert.Equal(t, map[string]any{"name": "slow"}, result.value)
}

// recordingReporter records deprecation reports.
type recordingReporter struct {
	mu      sync.Mutex
	reports []string
}

func (r *recordingReporter) Deprecated(templateName, deprecated, _, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, templateName+":"+deprecated)
}

// Context renders run the same deprecation lint as plain renders.
func TestRender_ContextSourcesRunDeprecationLint(t *testing.T) {
	top := writeFixture(t, "top.json", `{"name":"atmos"}`)
	reporter := &recordingReporter{}

	// The read fails (top.json is not a directory); the lint runs before execution.
	_, _ = New(WithDeprecationReporter(reporter)).Render(context.Background(), &Request{
		Name:           "stack.yaml",
		Text:           `{{ ds "top" "x.json" }}`,
		ContextSources: map[string]Datasource{"top": {URL: top}},
	})

	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	require.NotEmpty(t, reporter.reports, "the sub-path lint did not run for a context render")
	assert.Contains(t, reporter.reports[0], "stack.yaml:")
}

// countingServer serves a JSON body that names the request number, and counts requests.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := count.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"request":%d}`, n)
	}))
	t.Cleanup(server.Close)
	return server, &count
}

// Reads under the same environment share a cache entry; a different
// environment (for example a per-stack AWS_PROFILE) never sees another
// environment's cached value.
func TestDatasourceCacheIsScopedToTheEnvironment(t *testing.T) {
	server, count := countingServer(t)
	cache := NewDatasourceCache()
	read := func() any {
		t.Helper()
		d, err := NewData([]string{"api=" + server.URL + "/data.json"}, nil)
		require.NoError(t, err)
		value, err := cache.Read(d, "api")
		require.NoError(t, err)
		return value
	}

	t.Setenv("ATMOS_TEST_DATASOURCE_SCOPE", "one")
	first := read()
	assert.Equal(t, first, read(), "the same environment reuses the cached value")
	assert.Equal(t, int32(1), count.Load())

	t.Setenv("ATMOS_TEST_DATASOURCE_SCOPE", "two")
	second := read()
	assert.NotEqual(t, first, second, "a different environment must not reuse the cached value")
	assert.Equal(t, int32(2), count.Load())

	t.Setenv("ATMOS_TEST_DATASOURCE_SCOPE", "one")
	assert.Equal(t, first, read(), "returning to the first environment reuses its entry")
	assert.Equal(t, int32(2), count.Load())
}

// Redefining an alias replaces the reader, so the next read uses the new
// definition instead of the previous one.
func TestRedefiningADatasourceReplacesItsReader(t *testing.T) {
	first := writeFixture(t, "first.json", `{"name":"first"}`)
	second := writeFixture(t, "second.json", `{"name":"second"}`)
	d := &Data{}
	require.NoError(t, d.defineAll(map[string]Datasource{"cfg": {URL: first}}))
	value, err := d.Datasource("cfg")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"name": "first"}, value)

	require.NoError(t, d.defineAll(map[string]Datasource{"cfg": {URL: second}}))
	value, err = d.Datasource("cfg")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"name": "second"}, value)
}

// Binding a new render context replaces the readers, so each render's reads
// honor its own deadline instead of the context a reader was created with.
func TestGomplateFuncsRebindsReadersToTheRenderContext(t *testing.T) {
	server, _ := countingServer(t)
	d, err := NewData([]string{"api=" + server.URL + "/data.json"}, nil)
	require.NoError(t, err)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	GomplateFuncs(canceled, d)
	_, err = d.Datasource("api")
	require.ErrorIs(t, err, context.Canceled, "the read uses the bound, canceled context")

	GomplateFuncs(context.Background(), d)
	value, err := d.Datasource("api")
	require.NoError(t, err, "a new render context replaces the reader bound to the canceled one")
	assert.NotNil(t, value)
}

// An undefined absolute-URL alias is cached under the definition its first
// read registers, so a later read, even through a Data where the alias is now
// registered, reuses the entry instead of reading and parsing again.
func TestDatasourceCacheKeyIsStableForAbsoluteURLAliases(t *testing.T) {
	server, count := countingServer(t)
	cache := NewDatasourceCache()
	alias := server.URL + "/data.json"

	d := &Data{}
	first, err := cache.Read(d, alias)
	require.NoError(t, err)
	require.True(t, d.DatasourceExists(alias), "the first read registers the alias")

	second, err := cache.Read(d, alias)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, int32(1), count.Load(), "the second read hits the same cache entry")
	entries := 0
	cache.entries.Range(func(_, _ any) bool { entries++; return true })
	assert.Equal(t, 1, entries, "both reads share one cache entry")

	// Negative path: a relative, undefined alias has no definition to key on and still fails.
	_, err = cache.Read(&Data{}, "not-a-url")
	require.Error(t, err)
}

// A template that changes a cached datasource value (for example with Sprig's
// `set`) changes only its own copy: neither the first caller nor later ones
// can alter what the cache returns.
func TestDatasourceCacheReturnsCopies(t *testing.T) {
	cfg := writeFixture(t, "config.json", `{"name":"atmos","list":["a"],"nested":{"key":"value"}}`)
	csv := writeFixture(t, "table.csv", "k,v\n1,2\n")
	d, err := NewData([]string{"cfg=" + cfg, "csv=" + csv}, nil)
	require.NoError(t, err)
	cache := NewDatasourceCache()

	first, err := cache.Read(d, "cfg")
	require.NoError(t, err)
	first.(map[string]any)["name"] = "changed"
	first.(map[string]any)["list"].([]any)[0] = "changed"
	first.(map[string]any)["nested"].(map[string]any)["key"] = "changed"

	second, err := cache.Read(d, "cfg")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"name": "atmos", "list": []any{"a"}, "nested": map[string]any{"key": "value"}}, second)

	table, err := cache.Read(d, "csv")
	require.NoError(t, err)
	rows, ok := table.([][]string)
	require.True(t, ok, "CSV keeps its type: %T", table)
	rows[1][0] = "changed"
	again, err := cache.Read(d, "csv")
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"k", "v"}, {"1", "2"}}, again)
}

// With Gomplate disabled, context sources are still read under the render's
// own context, not a stale or background one.
func TestContextSourcesUseTheRenderContextWithGomplateDisabled(t *testing.T) {
	server, _ := countingServer(t)
	sources := &Data{}
	req := func() *Request {
		return &Request{
			Name: "t", Text: `{{ .request }}`, Sources: sources,
			ContextSources: map[string]Datasource{".": {URL: server.URL + "/data.json"}},
		}
	}
	engine := New(WithGomplate(false))

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := engine.Render(canceled, req())
	require.ErrorIs(t, err, context.Canceled, "the read uses the render's canceled context")

	out, err := engine.Render(context.Background(), req())
	require.NoError(t, err, "a later render with a live context is not stuck with the canceled one")
	assert.NotEmpty(t, out)
}
