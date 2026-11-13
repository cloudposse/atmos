package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// withLogs makes the evaluation of one call report the Terragrunt logger's events at the level the host asked
// for. The events return in the response, or in the failure when the call fails, so the host can show them
// through the Atmos logger. Nothing is written to a stream and no event changes the outcome of the call.
// The sensitive parts of the given request values are redacted from every event, as are the inputs the
// evaluation finds sensitive.
func withLogs(ctx context.Context, invocation *pluginv1.Invocation, root string, values ...*pluginv1.ComponentValue) (context.Context, *engine.LogSink) {
	sink := engine.NewLogSink(invocation.GetLogLevel(), root)
	for _, value := range values {
		redactSensitive(sink, value)
	}
	return engine.WithLogSink(ctx, sink), sink
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

// plainValue converts a value to the plain form of JSON decoding, without its marks.
func plainValue(value cty.Value) (any, error) {
	unmarked, _ := value.UnmarkDeep()
	if !unmarked.IsWhollyKnown() {
		return nil, component.ErrInvalidValue
	}
	data, err := ctyjson.Marshal(unmarked, unmarked.Type())
	if err != nil {
		return nil, err
	}
	var plain any
	if err := json.Unmarshal(data, &plain); err != nil {
		return nil, err
	}
	return plain, nil
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
