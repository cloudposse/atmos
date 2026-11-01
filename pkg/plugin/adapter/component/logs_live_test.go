package component

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// syncBuffer is a log destination that the test reads while the logger writes from another goroutine.
type syncBuffer struct {
	mu   sync.Mutex
	text strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.String()
}

func captureLiveLogs(t *testing.T, level charm.Level) *syncBuffer {
	t.Helper()
	output := &syncBuffer{}
	previousLevel := log.GetLevel()
	log.SetOutput(output)
	log.SetLevel(level)
	ResetShownMessages()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(previousLevel)
		ResetShownMessages()
	})
	return output
}

func liveRequest(t *testing.T, root, command string) *pluginv1.ComponentRequest {
	t.Helper()
	return &pluginv1.ComponentRequest{
		RootDirectory: root, Address: fixtureAddress(), Invocation: &pluginv1.Invocation{LogLevel: "debug"},
		Operation:             &pluginv1.ComponentOperation{Command: command, Mode: "native"},
		Configuration:         wireValue(t, cty.ObjectVal(map[string]cty.Value{})),
		ProviderConfiguration: wireValue(t, cty.ObjectVal(map[string]cty.Value{})),
	}
}

type prepared struct {
	response *pluginv1.PrepareComponentResponse
	err      error
}

// waitFor polls until the condition holds, failing the test when it does not within the limit.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	require.Eventually(t, condition, 20*time.Second, 5*time.Millisecond, what)
}

func TestLiveLogsReachTheLoggerWhileTheCallIsStillRunning(t *testing.T) {
	output := captureLiveLogs(t, charm.DebugLevel)
	atmosio.RegisterSecret("live-secret-value-91ab")
	root := t.TempDir()
	provider := fixtureProvider(t)
	ctx := WithLogScope(t.Context(), LogScope{Component: "app", Stack: "live"})
	finished := make(chan prepared, 1)
	go func() {
		response, err := provider.Prepare(ctx, liveRequest(t, root, "live-logs"))
		finished <- prepared{response, err}
	}()

	// The plugin has delivered an entry and is blocked: the call has not returned, yet the entry is on screen.
	waitFor(t, "first live entry reaches the Atmos logger", func() bool { return strings.Contains(output.String(), "first live entry") })
	select {
	case result := <-finished:
		t.Fatalf("the call returned before it was released: %+v", result)
	default:
	}
	early := output.String()
	assert.Contains(t, early, "component=app")
	assert.Contains(t, early, "stack=live")
	assert.Contains(t, early, "plugin=fixture/hcl")
	assert.Contains(t, early, "unit=live/app")
	assert.NotContains(t, early, "last live entry", "an entry the plugin has not produced yet cannot be on screen")
	// Secrets in a live entry are masked like those in a response.
	assert.NotContains(t, early, "live-secret-value-91ab")

	require.NoError(t, os.WriteFile(filepath.Join(root, "gate"), nil, 0o600))
	result := <-finished
	require.NoError(t, result.err)
	// The final entry is on screen once the call has returned, in the order the plugin produced them, and the
	// response does not repeat what was delivered live.
	final := output.String()
	first, info, last := strings.Index(final, "first live entry"), strings.Index(final, "info live entry"), strings.Index(final, "last live entry")
	assert.True(t, first >= 0 && first < info && info < last, "order: %d %d %d in %q", first, info, last, final)
	assert.Empty(t, result.response.GetLogs())
	assert.Equal(t, 1, strings.Count(final, "first live entry"))
}

func TestLiveLogsAreGatedByTheAtmosLevel(t *testing.T) {
	output := captureLiveLogs(t, charm.InfoLevel)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "gate"), nil, 0o600))
	_, err := fixtureProvider(t).Prepare(t.Context(), liveRequest(t, root, "live-logs"))
	require.NoError(t, err)
	assert.Contains(t, output.String(), "info live entry")
	assert.NotContains(t, output.String(), "first live entry")
	assert.NotContains(t, output.String(), "last live entry")

	warn := captureLiveLogs(t, charm.WarnLevel)
	_, err = fixtureProvider(t).Prepare(t.Context(), liveRequest(t, root, "live-logs"))
	require.NoError(t, err)
	assert.Empty(t, warn.String())
}

func TestLiveLogsDefaultToThePluginScopeAndNeverTouchTheResponse(t *testing.T) {
	output := captureLiveLogs(t, charm.DebugLevel)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "gate"), nil, 0o600))
	response, err := fixtureProvider(t).Prepare(t.Context(), liveRequest(t, root, "live-logs"))
	require.NoError(t, err)
	assert.Contains(t, output.String(), "plugin=fixture/hcl")
	assert.NotContains(t, output.String(), "component=")
	assert.Empty(t, response.GetLogs())
}

func TestEntriesTheResponseCarriesStayInTheResponse(t *testing.T) {
	output := captureLiveLogs(t, charm.DebugLevel)
	response, err := fixtureProvider(t).Prepare(t.Context(), liveRequest(t, t.TempDir(), "return-logs"))
	require.NoError(t, err)
	// A plugin that did not deliver live returns its entries as before, and the adapter renders nothing itself.
	require.Len(t, response.GetLogs(), 1)
	assert.Equal(t, "returned in the response", response.GetLogs()[0].GetMessage())
	assert.Empty(t, output.String())
}

func TestCancellingTheCallStopsLiveDeliveryWithoutLeakingGoroutines(t *testing.T) {
	output := captureLiveLogs(t, charm.DebugLevel)
	provider := fixtureProvider(t)
	// Warm up so the baseline includes the long-lived runtime goroutines.
	warm := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(warm, "gate"), nil, 0o600))
	_, err := provider.Prepare(t.Context(), liveRequest(t, warm, "live-logs"))
	require.NoError(t, err)
	baseline := settledGoroutines()
	require.Positive(t, baseline, "the counter must see at least this test goroutine")

	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan prepared, 1)
	go func() {
		response, err := provider.Prepare(ctx, liveRequest(t, root, "live-logs"))
		finished <- prepared{response, err}
	}()
	waitFor(t, "first live entry of the second call", func() bool { return strings.Count(output.String(), "first live entry") == 2 })
	cancel()
	select {
	case result := <-finished:
		require.Error(t, result.err)
	case <-time.After(20 * time.Second):
		t.Fatal("cancelling the command did not stop the call")
	}
	// Poll directly: a testify Eventually condition runs on a goroutine of its own and would count itself.
	// Count only goroutines running Atmos code, so runtime and scheduler goroutines that come and go
	// (garbage collection workers, timers, the race detector) cannot make the comparison flaky.
	deadline := time.Now().Add(10 * time.Second)
	for atmosGoroutines() > baseline && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if atmosGoroutines() > baseline {
		buffer := make([]byte, 1<<18)
		t.Fatalf("Atmos goroutines after cancel: %d, baseline %d\n%s", atmosGoroutines(), baseline, buffer[:runtime.Stack(buffer, true)])
	}
	// Only the warm-up call produced a last entry: the cancelled call never reached its end.
	assert.Equal(t, 1, strings.Count(output.String(), "last live entry"))
	assert.Equal(t, 2, strings.Count(output.String(), "first live entry"))
}

func settledGoroutines() int {
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	return atmosGoroutines()
}

// atmosGoroutines counts goroutines with a frame in Atmos code.
func atmosGoroutines() int {
	buffer := make([]byte, 1<<20)
	buffer = buffer[:runtime.Stack(buffer, true)]
	count := 0
	for _, stack := range strings.Split(string(buffer), "\n\n") {
		if strings.Contains(stack, "github.com/cloudposse/atmos/") {
			count++
		}
	}
	return count
}
