package provider

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/cloudposse/atmos/sdk/api"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// logReceiver stands in for Atmos: it records what the plugin delivers while a call runs.
type logReceiver struct {
	pluginv1.UnimplementedLogServer
	mu         sync.Mutex
	entries    []*pluginv1.LogEntry
	invocation string
	calls      int
}

func (r *logReceiver) Write(ctx context.Context, request *pluginv1.LogRequest) (*emptypb.Empty, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if values := metadata.ValueFromIncomingContext(ctx, api.InvocationMetadata); len(values) == 1 {
		r.invocation = values[0]
	}
	r.entries = append(r.entries, request.GetEntries()...)
	return &emptypb.Empty{}, nil
}

func (r *logReceiver) snapshot() ([]*pluginv1.LogEntry, int, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*pluginv1.LogEntry(nil), r.entries...), r.calls, r.invocation
}

// apiClient binds an API client to a real in-process connection. A nil receiver serves no log service,
// like an Atmos that predates live delivery.
func apiClient(t *testing.T, receiver *logReceiver) *api.Client {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	if receiver != nil {
		pluginv1.RegisterLogServer(server, receiver)
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///log-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	client := &api.Client{}
	require.NoError(t, client.Bind(connection))
	return client
}

func messagesOf(entries []*pluginv1.LogEntry) []string {
	messages := make([]string, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, entry.GetMessage())
	}
	return messages
}

func discoverAt(t *testing.T, provider *Provider, root, level, id string) *pluginv1.DiscoverComponentsResponse {
	t.Helper()
	opts, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true, "included_paths": []string{"live/**"}}})
	require.NoError(t, err)
	response, err := provider.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, IncludePaths: []string{"live/**"}, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"}, Invocation: &pluginv1.Invocation{Id: id, LogLevel: level}})
	require.NoError(t, err)
	return response
}

func TestEngineLogsAreDeliveredLiveAndNotRepeatedInTheResponse(t *testing.T) {
	receiver := &logReceiver{}
	response := discoverAt(t, New(nil, WithAPI(apiClient(t, receiver))), fixture(t, "explicit-stack"), "debug", "call-42")

	delivered, calls, invocation := receiver.snapshot()
	assert.Equal(t, "call-42", invocation, "the entries belong to the invocation that made the call")
	require.NotEmpty(t, delivered)
	assert.Positive(t, calls)
	// Everything was delivered before the result, so the response repeats nothing.
	assert.Empty(t, response.Logs)
	// The phase event names the unit being evaluated, relative to the project, at info level.
	var phases []string
	for _, entry := range delivered {
		assert.NotEqual(t, pluginv1.LogLevel_LOG_LEVEL_TRACE, entry.GetLevel(), "trace is below the requested level")
		if entry.GetMessage() == "Evaluating unit" {
			phases = append(phases, entry.GetUnit())
			assert.Equal(t, pluginv1.LogLevel_LOG_LEVEL_INFO, entry.GetLevel())
		}
	}
	assert.NotEmpty(t, phases)
	for _, unit := range phases {
		assert.False(t, filepath.IsAbs(unit), "unit %q is relative to the project root", unit)
	}
}

func TestEngineLogsFallBackToTheResponseWhenAtmosCannotTakeThem(t *testing.T) {
	root := fixture(t, "explicit-stack")
	baseline := discoverAt(t, New(nil), root, "debug", "call-1")
	require.NotEmpty(t, baseline.Logs)

	for name, client := range map[string]*api.Client{
		"log service not served": apiClient(t, nil),
		"no connection":          {},
	} {
		t.Run(name, func(t *testing.T) {
			response := discoverAt(t, New(nil, WithAPI(client)), root, "debug", "call-1")
			assert.ElementsMatch(t, messagesOf(baseline.Logs), messagesOf(response.Logs), "the same events return with the response")
		})
	}

	t.Run("without an invocation to attribute them to", func(t *testing.T) {
		receiver := &logReceiver{}
		response := discoverAt(t, New(nil, WithAPI(apiClient(t, receiver))), root, "debug", "")
		_, calls, _ := receiver.snapshot()
		assert.Zero(t, calls)
		assert.ElementsMatch(t, messagesOf(baseline.Logs), messagesOf(response.Logs))
	})
}

func TestNothingIsDeliveredBelowTheInvocationLevel(t *testing.T) {
	for _, level := range []string{"", "warn", "error", "off", "info"} {
		receiver := &logReceiver{}
		response := discoverAt(t, New(nil, WithAPI(apiClient(t, receiver))), fixture(t, "explicit-stack"), level, "call-1")
		_, calls, _ := receiver.snapshot()
		assert.Zero(t, calls, "level %q", level)
		assert.Empty(t, response.Logs, "level %q", level)
	}
}

func brokenUnit(t *testing.T) *pluginv1.ComponentRequest {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "module"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "terragrunt.hcl"), []byte("terraform { source = \"./module\" }\ninputs = { broken = local.absent }\n"), 0o644))
	request := componentRequest(t)
	request.RootDirectory, request.Source = root, "terragrunt.hcl"
	request.Configuration, err = encode(map[string]any{})
	require.NoError(t, err)
	request.Invocation.LogLevel, request.Invocation.Id = "trace", "call-9"
	return request
}

func TestAFailedCallStillShowsItsLogsAndNothingTwice(t *testing.T) {
	receiver := &logReceiver{}
	_, err := New(nil, WithAPI(apiClient(t, receiver))).Prepare(t.Context(), brokenUnit(t))
	require.Error(t, err)
	detail, ok := component.FailureDetail(err)
	require.True(t, ok)
	delivered, _, _ := receiver.snapshot()
	assert.NotEmpty(t, delivered, "the events before the failure were shown while the call ran")
	assert.Empty(t, detail.Logs, "the failure does not repeat them")

	_, err = New(nil, WithAPI(apiClient(t, nil))).Prepare(t.Context(), brokenUnit(t))
	detail, ok = component.FailureDetail(err)
	require.True(t, ok)
	assert.NotEmpty(t, detail.Logs, "without live delivery the failure carries the events that explain it")
}
