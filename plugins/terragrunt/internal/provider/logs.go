package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/api"
	"github.com/cloudposse/atmos/sdk/component"
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
func (p *Provider) withLogs(ctx context.Context, invocation *pluginv1.Invocation, root string, values ...*pluginv1.ComponentValue) (context.Context, *callLogs) {
	sink := engine.NewLogSink(invocation.GetLogLevel(), root)
	for _, value := range values {
		redactSensitive(sink, value)
	}
	logs := &callLogs{sink: sink}
	if p.api != nil && sink.Enabled() {
		logs.stream = p.api.OpenLogs(ctx, invocation.GetId())
		sink.Forward(func(entry *model.LogEntry) { logs.stream.Send(logEntry(entry)) })
	}
	return engine.WithLogSink(ctx, sink), logs
}

// redactSensitive registers the sensitive parts of a request value with the sink. A value that does not
// decode registers nothing: the call rejects it before evaluating anything that could log.
func redactSensitive(sink *engine.LogSink, encoded *pluginv1.ComponentValue) {
	if encoded == nil || len(encoded.GetSensitivePaths()) == 0 {
		return
	}
	value, err := component.Decode(encoded)
	if err != nil {
		return
	}
	_ = cty.Walk(value, func(_ cty.Path, v cty.Value) (bool, error) {
		if !v.HasMark(component.Sensitive) {
			return true, nil
		}
		if plain, err := plainValue(v); err == nil {
			sink.RedactValue(plain)
		}
		return false, nil
	})
}

// plainValue converts a value to the plain form of JSON decoding, without its marks. Numbers stay
// json.Number, so a sensitive integer above 2^53 keeps its exact digits when its text is registered.
func plainValue(value cty.Value) (any, error) {
	unmarked, _ := value.UnmarkDeep()
	if !unmarked.IsWhollyKnown() {
		return nil, component.ErrInvalidValue
	}
	data, err := ctyjson.Marshal(unmarked, unmarked.Type())
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var plain any
	if err := decoder.Decode(&plain); err != nil {
		return nil, err
	}
	return plain, nil
}

// entries waits for the events delivered live to be shown, for a bounded time, stops delivery, and returns the
// events Atmos has not shown, in order, in their wire form. It is safe to call more than once.
func (l *callLogs) entries() []*pluginv1.LogEntry {
	collected := l.sink.Entries()
	entries := make([]*pluginv1.LogEntry, 0, len(collected))
	for i := range collected {
		converted := logEntry(&collected[i])
		if l.stream != nil {
			// Evaluation has finished, so every discovered secret has been registered or the
			// sink has withheld the call. Only now may detailed Terragrunt events be delivered.
			l.stream.Send(converted)
		} else {
			entries = append(entries, converted)
		}
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
