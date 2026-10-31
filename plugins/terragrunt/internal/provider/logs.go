package provider

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/api"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// callLogs carries the Terragrunt logger's events of one call to Atmos at the level it asked for. With an
// API connection the events are delivered while the call runs, so a slow run_cmd or registry fetch shows
// progress. Events that could not be delivered, and all events when there is no connection, are collected
// and returned with the response, or with the failure, for Atmos to show when the call ends. Nothing is
// written to a stream of its own and no event changes the outcome of the call.
type callLogs struct {
	sink   *engine.LogSink
	stream *api.LogStream
}

// withLogs makes the evaluation of one call report the Terragrunt logger's events. The caller must call
// entries before it returns, so the events delivered live are shown before the result.
func (p *Provider) withLogs(ctx context.Context, invocation *pluginv1.Invocation, root string) (context.Context, *callLogs) {
	sink := engine.NewLogSink(invocation.GetLogLevel(), root)
	logs := &callLogs{sink: sink}
	if p.api != nil && sink.Enabled() {
		logs.stream = p.api.OpenLogs(ctx, invocation.GetId())
		sink.Forward(func(entry *model.LogEntry) { logs.stream.Send(logEntry(entry)) })
	}
	return engine.WithLogSink(ctx, sink), logs
}

// entries waits for the events delivered live to be shown, for a bounded time, stops delivery, and returns the
// events Atmos has not shown, in order, in their wire form. It is safe to call more than once.
func (l *callLogs) entries() []*pluginv1.LogEntry {
	collected := l.sink.Entries()
	entries := make([]*pluginv1.LogEntry, 0, len(collected))
	for i := range collected {
		entries = append(entries, logEntry(&collected[i]))
	}
	// The stream is empty of anything already shown; what it returns could not be delivered.
	entries = append(entries, l.stream.Close()...)
	if len(entries) == 0 {
		return nil
	}
	return entries
}

// logEntry converts one event to its wire form.
func logEntry(entry *model.LogEntry) *pluginv1.LogEntry {
	converted := &pluginv1.LogEntry{Level: logLevel(entry.Level), Message: entry.Message, Fields: entry.Fields, Unit: entry.Unit}
	if !entry.Time.IsZero() {
		converted.Time = timestamppb.New(entry.Time)
	}
	return converted
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

// withFailureLogs attaches the events Atmos has not shown to the failure Atmos renders, and makes sure the
// events delivered live are shown before the failure.
func withFailureLogs(err error, logs *callLogs) error {
	entries := logs.entries()
	var failure *rpcFailure
	if !errors.As(err, &failure) {
		return err
	}
	if len(entries) > 0 {
		failure.failure.Logs(entries)
	}
	return err
}
