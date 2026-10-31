package provider

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// withLogs makes the evaluation of one call report the Terragrunt logger's events at the level the host asked
// for. The events return in the response, or in the failure when the call fails, so the host can show them
// through the Atmos logger. Nothing is written to a stream and no event changes the outcome of the call.
func withLogs(ctx context.Context, invocation *pluginv1.Invocation, root string) (context.Context, *engine.LogSink) {
	sink := engine.NewLogSink(invocation.GetLogLevel(), root)
	return engine.WithLogSink(ctx, sink), sink
}

// logEntries converts the collected events to their wire form.
func logEntries(sink *engine.LogSink) []*pluginv1.LogEntry {
	collected := sink.Entries()
	if len(collected) == 0 {
		return nil
	}
	entries := make([]*pluginv1.LogEntry, 0, len(collected))
	for i := range collected {
		entry := &collected[i]
		converted := &pluginv1.LogEntry{Level: logLevel(entry.Level), Message: entry.Message, Fields: entry.Fields, Unit: entry.Unit}
		if !entry.Time.IsZero() {
			converted.Time = timestamppb.New(entry.Time)
		}
		entries = append(entries, converted)
	}
	return entries
}

func logLevel(level string) pluginv1.LogLevel {
	switch strings.ToLower(level) {
	case "trace":
		return pluginv1.LogLevel_LOG_LEVEL_TRACE
	case "debug":
		return pluginv1.LogLevel_LOG_LEVEL_DEBUG
	case "info":
		return pluginv1.LogLevel_LOG_LEVEL_INFO
	case "warn":
		return pluginv1.LogLevel_LOG_LEVEL_WARN
	case "error":
		return pluginv1.LogLevel_LOG_LEVEL_ERROR
	}
	return pluginv1.LogLevel_LOG_LEVEL_UNSPECIFIED
}

// withFailureLogs attaches the events collected before a failure to the failure the host renders.
func withFailureLogs(err error, sink *engine.LogSink) error {
	var failure *rpcFailure
	if !errors.As(err, &failure) {
		return err
	}
	if entries := logEntries(sink); len(entries) > 0 {
		failure.failure.Logs(entries)
	}
	return err
}
