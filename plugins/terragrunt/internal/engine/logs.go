package engine

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

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
	if len(s.entries) >= maxLogEntries || s.size >= maxLogBytes {
		s.dropped++
		return
	}
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
	message := truncate(entry.Message)
	s.size += len(message) + len(unit)
	for key, value := range fields {
		s.size += len(key) + len(value)
	}
	s.entries = append(s.entries, model.LogEntry{Time: entry.Time, Level: level.String(), Message: message, Fields: fields, Unit: unit})
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
