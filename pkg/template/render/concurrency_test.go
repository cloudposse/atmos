package render

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
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
