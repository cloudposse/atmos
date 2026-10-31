package engine

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

const (
	// Bound on the events one call returns, so a chatty trace log cannot exceed the RPC size limit.
	maxLogEntries = 500
	// Bound on the length of one message or field value.
	maxLogMessage = 2048
	// Bound on the text of all events one call returns, well under the default 4 MiB RPC message limit.
	maxLogBytes = 1 << 20
	// Field that Terragrunt uses for the directory of the unit an event is about.
	unitField = "prefix"
)

type logSinkKey struct{}

// LogSink collects the events of the Terragrunt logger during one call, at or above a level. It is safe
// for concurrent use because Terragrunt may evaluate units in parallel.
type LogSink struct {
	root    string
	entries []model.LogEntry
	level   log.Level
	dropped int
	size    int
	mu      sync.Mutex
	off     bool
	// forward receives each kept event as it happens instead of the sink collecting it.
	forward func(*model.LogEntry)
}

// NewLogSink returns a sink for the events of a call at the host's log level. Terragrunt's logger reports
// engine chatter, such as how it located files, which is a diagnostic aid and not a message for the user:
// anything the user must act on reaches the host as a diagnostic or an error. So events are collected only
// when the host asks for debug or trace output, and the Atmos default of Warn collects nothing. At debug
// the sink keeps events at debug and above; at trace it keeps everything. The root argument makes the unit
// field project relative.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func NewLogSink(level, root string) *LogSink {
	sink := &LogSink{root: root, off: true}
	switch strings.ToLower(level) {
	case "debug":
		sink.level, sink.off = log.DebugLevel, false
	case "trace":
		sink.level, sink.off = log.TraceLevel, false
	}
	return sink
}

// Enabled reports whether the sink keeps any event, which depends on the level the host asked for.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) Enabled() bool { return !s.off }

// Forward hands every kept event to deliver as it happens, in order, instead of collecting it for Entries.
// Terragrunt may log from several goroutines, so deliver is called while the sink holds its lock to keep the
// order of events; it must be fast and must not call back into the sink. Events are no longer bounded by
// the sink: the deliverer owns that.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) Forward(deliver func(*model.LogEntry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forward = deliver
}

// Event reports an event of the plugin itself, such as the start of an evaluation, the way the Terragrunt
// logger's events are reported: kept at the sink's level, collected or forwarded in order.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) Event(level log.Level, message, unit string) {
	if s == nil || s.off || level > s.level {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keep(&model.LogEntry{Time: time.Now(), Level: level.String(), Message: message, Unit: s.relative(unit)})
}

// TerragruntLevel returns the Terragrunt logger level that produces every event the sink keeps.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) TerragruntLevel() string {
	if s.off {
		return log.ErrorLevel.String()
	}
	return s.level.String()
}

// WithLogSink makes evaluations started with the returned context report their log events to sink.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func WithLogSink(ctx context.Context, sink *LogSink) context.Context {
	return context.WithValue(ctx, logSinkKey{}, sink)
}

func logSinkFrom(ctx context.Context) *LogSink {
	sink, _ := ctx.Value(logSinkKey{}).(*LogSink)
	return sink
}

// Entries returns the collected events in order. When events were dropped, the last entry says how many.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) Entries() []model.LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := slices.Clone(s.entries)
	if s.dropped > 0 {
		entries = append(entries, model.LogEntry{Level: log.WarnLevel.String(), Message: fmt.Sprintf("%d further Terragrunt log events were omitted", s.dropped)})
	}
	return entries
}

// Adapts the sink to the Terragrunt logger.
func (s *LogSink) hook() logrus.Hook { return sinkHook{sink: s} }

type sinkHook struct{ sink *LogSink }

//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (sinkHook) Levels() []logrus.Level {
	levels := make([]logrus.Level, 0, len(log.AllLevels))
	for _, level := range log.AllLevels {
		levels = append(levels, level.ToLogrusLevel())
	}
	return levels
}

//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (h sinkHook) Fire(entry *logrus.Entry) error {
	// Terragrunt shifts the numeric levels of Logrus, so entry.Level.String() would mislabel them.
	level := log.FromLogrusLevel(entry.Level)
	// Command output is not logging: the host streams it separately.
	if level == log.StdoutLevel || level == log.StderrLevel {
		return nil
	}
	h.sink.add(level, entry)
	return nil
}

func (s *LogSink) add(level log.Level, entry *logrus.Entry) {
	if s.off || level > s.level {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fields := make(map[string]string, len(entry.Data))
	unit := ""
	for _, key := range slices.Sorted(maps.Keys(entry.Data)) {
		value := truncate(fmt.Sprint(entry.Data[key]))
		if key == unitField {
			unit = s.relative(value)
			continue
		}
		fields[key] = value
	}
	s.keep(&model.LogEntry{Time: entry.Time, Level: level.String(), Message: truncate(entry.Message), Fields: fields, Unit: unit})
}

// keep forwards the event when a deliverer is set, and otherwise collects it up to the bounds. The caller
// holds the lock.
func (s *LogSink) keep(entry *model.LogEntry) {
	if s.forward != nil {
		s.forward(entry)
		return
	}
	size := len(entry.Message) + len(entry.Unit)
	for key, value := range entry.Fields {
		size += len(key) + len(value)
	}
	if len(s.entries) >= maxLogEntries || s.size >= maxLogBytes {
		s.dropped++
		return
	}
	s.size += size
	s.entries = append(s.entries, *entry)
}

// relative makes a unit directory project relative, leaving a path outside the project unchanged.
func (s *LogSink) relative(directory string) string {
	if s.root == "" || !filepath.IsAbs(directory) {
		return directory
	}
	if rel, err := filepath.Rel(s.root, directory); err == nil && filepath.IsLocal(rel) {
		return filepath.ToSlash(rel)
	}
	return directory
}

func truncate(text string) string {
	if len(text) <= maxLogMessage {
		return text
	}
	return text[:maxLogMessage] + "..."
}
