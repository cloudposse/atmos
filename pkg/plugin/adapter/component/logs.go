package component

import (
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
	Warnings  *WarningDeduper
}

// logOff is the level that tells a plugin to return no log events.
const logOff = "off"

// WarningDeduper is owned by one configuration session, so concurrent stack queries
// cannot hide each other's warnings and its cache leaves with the session.
type WarningDeduper struct {
	sync.Mutex
	keys map[string]struct{}
}

// firstTime reports whether the message identified by parts is shown for the first time in this invocation.
func (d *WarningDeduper) firstTime(parts ...string) bool {
	if d == nil {
		return true
	}
	key := strings.Join(parts, "\x00")
	d.Lock()
	defer d.Unlock()
	if d.keys == nil {
		d.keys = make(map[string]struct{})
	}
	if _, seen := d.keys[key]; seen {
		return false
	}
	d.keys[key] = struct{}{}
	return true
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
		if !ok {
			continue
		}
		message := atmosio.MaskString(entry.GetMessage())
		fields := logFields(scope, entry)
		// The key covers the provider and the fields, so warnings that share a message but name a
		// different dependency or file are each shown; only an identical repeat is suppressed.
		if level >= charm.WarnLevel && !scope.Warnings.firstTime(append([]string{"log", entry.GetLevel().String(), message}, fieldStrings(fields)...)...) {
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
