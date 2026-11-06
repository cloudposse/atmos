package render

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A slow datasource must not hold rendererMu while it is read, or it would
// block every other render's datasource reads until it finishes.
func TestDatasourceReadDoesNotHoldRendererLock(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"slow"}`))
	}))
	t.Cleanup(server.Close)

	d, err := NewData([]string{"slow=" + server.URL + "/data.json"}, nil)
	require.NoError(t, err)

	var (
		wg      sync.WaitGroup
		result  any
		readErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, readErr = d.Datasource("slow")
	}()

	<-started
	locked := rendererMu.TryLock()
	if locked {
		rendererMu.Unlock()
	}
	close(release)
	wg.Wait()

	assert.True(t, locked, "rendererMu was held during the datasource read")
	require.NoError(t, readErr)
	assert.Equal(t, map[string]any{"name": "slow"}, result)
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
