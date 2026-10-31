package component

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	charm "github.com/charmbracelet/log"

	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// LogScope identifies the call a plugin's log events belong to.
type LogScope struct {
	Provider  string
	Component string
	Stack     string
}

type logScopeKey struct{}

// WithLogScope names the component and stack that log events delivered during a call belong to. A plugin
// delivers its log events while the call runs, so the call must carry the scope; a call without one is scoped
// to the plugin alone.
func WithLogScope(ctx context.Context, scope LogScope) context.Context {
	defer perf.Track(nil, "componentadapter.WithLogScope")()

	return context.WithValue(ctx, logScopeKey{}, scope)
}

// liveLogs renders the entries a plugin delivers while a call runs, scoped to the call.
type liveLogs struct{ scope LogScope }

// newLiveLogs returns the sink for the call made with ctx by the provider named name.
func newLiveLogs(ctx context.Context, name string) liveLogs {
	scope, _ := ctx.Value(logScopeKey{}).(LogScope)
	if scope.Provider == "" {
		scope.Provider = name
	}
	return liveLogs{scope: scope}
}

// WriteLogs renders the entries now, in order, through the same path as entries a response carries.
func (l liveLogs) WriteLogs(_ context.Context, entries []*pluginv1.LogEntry) {
	defer perf.Track(nil, "componentadapter.liveLogs.WriteLogs")()

	RenderLogs(l.scope, entries)
}

// logOff is the level that tells a plugin to return no log events.
const logOff = "off"

// shown holds the messages already shown during this invocation, so that the several evaluations one command
// makes (preparation reruns before every step) do not repeat the same warning.
var shown = struct {
	sync.Mutex
	keys map[string]struct{}
}{keys: map[string]struct{}{}}

// firstTime reports whether the message identified by parts is shown for the first time in this invocation.
func firstTime(parts ...string) bool {
	key := strings.Join(parts, "\x00")
	shown.Lock()
	defer shown.Unlock()
	if _, seen := shown.keys[key]; seen {
		return false
	}
	shown.keys[key] = struct{}{}
	return true
}

// ResetShownMessages forgets which warnings were shown. Tests use it for isolation.
func ResetShownMessages() {
	defer perf.Track(nil, "componentadapter.ResetShownMessages")()

	shown.Lock()
	defer shown.Unlock()
	clear(shown.keys)
}

// HostLogLevel returns the Atmos log level in the vocabulary of the plugin protocol, so a plugin can skip
// events nobody will see. A level above error means off.
func HostLogLevel() string {
	defer perf.Track(nil, "componentadapter.HostLogLevel")()

	level := log.GetLevel()
	switch {
	case level == log.TraceLevel:
		return "trace"
	case level > charm.ErrorLevel:
		return logOff
	default:
		return log.LevelToString(level)
	}
}

// RenderLogs shows a plugin's log events through the Atmos logger, on the UI channel (stderr), scoped to the
// component and stack of the call. The Atmos log level decides what is visible: the logger drops events
// below it. Messages and field values are masked. Rendering never fails the call. A warning or error that was
// already shown for the same component and stack in this invocation is not shown again.
func RenderLogs(scope LogScope, entries []*pluginv1.LogEntry) {
	defer perf.Track(nil, "componentadapter.RenderLogs")()

	for _, entry := range entries {
		level, ok := logLevel(entry.GetLevel())
		// The plugin drops events below the level it was given, and the host checks again here: a plugin may
		// ignore the level, and masking and formatting an event nobody will see is wasted work.
		if !ok || level < log.GetLevel() {
			continue
		}
		message := atmosio.MaskString(entry.GetMessage())
		fields := logFields(scope, entry)
		// The key covers the provider and the fields, so warnings that share a message but name a
		// different dependency or file are each shown; only an identical repeat is suppressed.
		if level >= charm.WarnLevel && !firstTime(append([]string{"log", entry.GetLevel().String(), message}, fieldStrings(fields)...)...) {
			continue
		}
		log.Log(level, message, fields...)
	}
}

func logLevel(level pluginv1.LogLevel) (charm.Level, bool) {
	switch level {
	case pluginv1.LogLevel_LOG_LEVEL_TRACE:
		return log.TraceLevel, true
	case pluginv1.LogLevel_LOG_LEVEL_DEBUG:
		return charm.DebugLevel, true
	case pluginv1.LogLevel_LOG_LEVEL_INFO:
		return charm.InfoLevel, true
	case pluginv1.LogLevel_LOG_LEVEL_WARN:
		return charm.WarnLevel, true
	case pluginv1.LogLevel_LOG_LEVEL_ERROR:
		return charm.ErrorLevel, true
	}
	return 0, false
}

// fieldStrings renders logger key/value pairs as strings for a deduplication key.
func fieldStrings(fields []any) []string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		value, _ := field.(string)
		parts = append(parts, value)
	}
	return parts
}

// logFields lists the scope first, then the event's own fields in key order, all masked.
func logFields(scope LogScope, entry *pluginv1.LogEntry) []any {
	fields := make([]any, 0, 8+2*len(entry.GetFields()))
	for _, pair := range [][2]string{{"plugin", scope.Provider}, {log.FieldComponent, scope.Component}, {log.FieldStack, scope.Stack}, {"unit", entry.GetUnit()}} {
		if pair[1] != "" {
			fields = append(fields, pair[0], atmosio.MaskString(pair[1]))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(entry.GetFields())) {
		fields = append(fields, key, atmosio.MaskString(entry.Fields[key]))
	}
	return fields
}
