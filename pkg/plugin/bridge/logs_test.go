package bridge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func logEntries(messages ...string) []*pluginv1.LogEntry {
	entries := make([]*pluginv1.LogEntry, 0, len(messages))
	for _, message := range messages {
		entries = append(entries, &pluginv1.LogEntry{Level: pluginv1.LogLevel_LOG_LEVEL_DEBUG, Message: message})
	}
	return entries
}

func TestLogDeliveryIsGrantedPerInvocation(t *testing.T) {
	t.Parallel()
	server := New()
	sink := NewMockLogSink(gomock.NewController(t))
	granted, err := server.Begin("granted", Invocation{Context: t.Context(), Namespace: "plugin", Logs: sink})
	require.NoError(t, err)
	t.Cleanup(granted)
	withheld, err := server.Begin("withheld", Invocation{Context: t.Context(), Namespace: "plugin"})
	require.NoError(t, err)
	t.Cleanup(withheld)
	with := func(id string) context.Context {
		return metadata.NewIncomingContext(t.Context(), metadata.Pairs(InvocationMetadata, id))
	}

	// Entries reach the sink of the invocation that made the call, in order, before the call returns.
	var delivered []string
	sink.EXPECT().WriteLogs(gomock.Any(), gomock.Any()).Do(func(_ any, entries []*pluginv1.LogEntry) {
		for _, entry := range entries {
			delivered = append(delivered, entry.GetMessage())
		}
	}).Times(2)
	_, err = server.Write(with("granted"), &pluginv1.LogRequest{Entries: logEntries("first", "second")})
	require.NoError(t, err)
	_, err = server.Write(with("granted"), &pluginv1.LogRequest{Entries: logEntries("third")})
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second", "third"}, delivered)

	// An invocation without a grant, an unknown one, and a missing one deliver nothing.
	for _, id := range []string{"withheld", "unknown", ""} {
		_, err := server.Write(with(id), &pluginv1.LogRequest{Entries: logEntries("hidden")})
		require.Equal(t, codes.PermissionDenied, status.Code(err), id)
	}

	// A batch above the bound is refused so one call cannot hold the terminal.
	_, err = server.Write(with("granted"), &pluginv1.LogRequest{Entries: logEntries(make([]string, maxLogBatch+1)...)})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	// After the invocation ends nothing is accepted, so a late call cannot print after the result.
	granted()
	_, err = server.Write(with("granted"), &pluginv1.LogRequest{Entries: logEntries("late")})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
