package engine

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

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
	// Replacement for a sensitive value in event text, the same one the host masker uses.
	maskReplacement = "<MASKED>"
)

type logSinkKey struct{}

// LogSink collects the events of the Terragrunt logger during one call, at or above a level. It is safe
// for concurrent use because Terragrunt may evaluate units in parallel.
type LogSink struct {
	root    string
	entries []logRecord
	secrets map[string]struct{}
	level   log.Level
	dropped int
	size    int
	mu      sync.Mutex
	off     bool
	// Withheld is set when the call may have logged a sensitive value the sink cannot redact.
	withheld bool
}

// logRecord is a collected event together with which of its texts were cut to the length bound, so that a
// sensitive value cut in half can still be redacted.
type logRecord struct {
	entry      model.LogEntry
	cutMessage bool
	cutUnit    bool
	cutFields  map[string]bool
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

// RedactValue registers the text of a sensitive value, so that no event the sink returns contains it. A map
// or list registers each of its scalars, as the host masker does. Registration may happen after the event
// was collected: a unit's inputs are known to be sensitive only once it has been evaluated, and its
// earlier events still must not carry them.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) RedactValue(value any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.register(value)
}

// redactInputs registers the values of the inputs at the given paths with the sink. An empty path means
// every input.
func redactInputs(sink *LogSink, inputs map[string]any, paths [][]string) {
	if sink == nil {
		return
	}
	for _, path := range paths {
		var value any = inputs
		for _, key := range path {
			object, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = object[key]
		}
		sink.RedactValue(value)
	}
}

func (s *LogSink) register(value any) {
	switch value := value.(type) {
	case nil:
	case map[string]any:
		for _, child := range value {
			s.register(child)
		}
	case []any:
		for _, child := range value {
			s.register(child)
		}
	default:
		text := fmt.Sprint(value)
		if text == "" {
			return
		}
		if s.secrets == nil {
			s.secrets = map[string]struct{}{}
		}
		s.secrets[text] = struct{}{}
	}
}

// collecting reports whether the sink keeps any event, so callers can skip work for a sink that is off.
func (s *LogSink) collecting() bool { return s != nil && !s.off }

// Withhold makes the sink return no collected event, only a notice that events were withheld. It is used
// when the call may have logged a sensitive value whose text the sink never learned, such as the output of
// a sensitive run_cmd in a unit that failed to evaluate.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) Withhold() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.withheld = true
}

// Entries returns the collected events in order, with every registered sensitive value redacted. When
// events were dropped, the last entry says how many.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) Entries() []model.LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.withheld {
		if count := len(s.entries) + s.dropped; count > 0 {
			return []model.LogEntry{{Level: log.WarnLevel.String(), Message: fmt.Sprintf("%d Terragrunt log events were withheld because the unit declares sensitive values that could not be redacted", count)}}
		}
		return nil
	}
	secrets := s.sortedSecrets()
	entries := make([]model.LogEntry, 0, len(s.entries)+1)
	for i := range s.entries {
		entries = append(entries, s.entries[i].redacted(secrets))
	}
	if s.dropped > 0 {
		entries = append(entries, model.LogEntry{Level: log.WarnLevel.String(), Message: fmt.Sprintf("%d further Terragrunt log events were omitted", s.dropped)})
	}
	return entries
}

// Hook adapts the sink to a Terragrunt logger.
//
//nolint:lintroller // Plugin evaluator code cannot import the host performance tracker.
func (s *LogSink) Hook() logrus.Hook { return sinkHook{sink: s} }

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
	record := logRecord{cutFields: map[string]bool{}}
	fields := make(map[string]string, len(entry.Data))
	unit := ""
	for _, key := range slices.Sorted(maps.Keys(entry.Data)) {
		raw := fmt.Sprint(entry.Data[key])
		value := truncate(raw)
		if key == unitField {
			unit, record.cutUnit = s.relative(value), len(value) != len(raw)
			continue
		}
		fields[key] = value
		if len(value) != len(raw) {
			record.cutFields[key] = true
		}
	}
	message := truncate(entry.Message)
	record.cutMessage = len(message) != len(entry.Message)
	// The size bound is checked on the collected text. Redaction replaces a value with a short marker, so
	// the returned text stays close to this size.
	s.size += len(message) + len(unit)
	for key, value := range fields {
		s.size += len(key) + len(value)
	}
	record.entry = model.LogEntry{Time: entry.Time, Level: level.String(), Message: message, Fields: fields, Unit: unit}
	s.entries = append(s.entries, record)
}

// sortedSecrets returns the registered values longest first, so a value that contains another is redacted
// whole.
func (s *LogSink) sortedSecrets() []string {
	secrets := slices.Collect(maps.Keys(s.secrets))
	slices.SortFunc(secrets, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	return secrets
}

// redacted returns a copy of the event with every registered value replaced.
func (r *logRecord) redacted(secrets []string) model.LogEntry {
	entry := r.entry
	entry.Message = redact(entry.Message, secrets, r.cutMessage)
	entry.Unit = redact(entry.Unit, secrets, r.cutUnit)
	if entry.Fields != nil {
		fields := make(map[string]string, len(entry.Fields))
		for key, value := range entry.Fields {
			fields[key] = redact(value, secrets, r.cutFields[key])
		}
		entry.Fields = fields
	}
	return entry
}

// redact replaces each occurrence of a registered value in text. When the text was cut to the length bound,
// a value the cut split leaves its beginning just before the ellipsis, so that beginning is replaced too.
// Every match is found in the original text before anything is replaced, so replacing a short value cannot
// hide the beginning of a longer one that overlaps it.
func redact(text string, secrets []string, cut bool) string {
	if len(secrets) == 0 || text == "" {
		return text
	}
	body, suffix := text, ""
	if cut {
		if trimmed, ok := strings.CutSuffix(text, ellipsis); ok {
			body, suffix = trimmed, ellipsis
		}
	}
	spans := secretSpans(body, secrets)
	if suffix != "" {
		if longest := longestCutPrefix(body, secrets); longest > 0 {
			spans = append(spans, [2]int{len(body) - longest, len(body)})
		}
	}
	if len(spans) == 0 {
		return text
	}
	return maskSpans(body, spans) + suffix
}

// secretSpans returns the byte ranges of every occurrence of every value in text, overlapping ones included.
func secretSpans(text string, secrets []string) [][2]int {
	var spans [][2]int
	for _, secret := range secrets {
		for offset := 0; offset < len(text); {
			index := strings.Index(text[offset:], secret)
			if index < 0 {
				break
			}
			begin := offset + index
			spans = append(spans, [2]int{begin, begin + len(secret)})
			offset = begin + 1
		}
	}
	return spans
}

// longestCutPrefix returns the length of the longest beginning of any value that ends the text, so that no
// part of a longer value stays visible.
func longestCutPrefix(body string, secrets []string) int {
	longest := 0
	for _, secret := range secrets {
		for end := min(len(secret)-1, len(body)); end > longest; end-- {
			if strings.HasSuffix(body, secret[:end]) {
				longest = end
				break
			}
		}
	}
	return longest
}

// maskSpans replaces each group of overlapping ranges with one mask. Ranges that only touch stay separate,
// as successive replacements would have left them.
func maskSpans(text string, spans [][2]int) string {
	slices.SortFunc(spans, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return b[1] - a[1]
	})
	var out strings.Builder
	position := 0
	for i := 0; i < len(spans); {
		begin, end := spans[i][0], spans[i][1]
		for i++; i < len(spans) && spans[i][0] < end; i++ {
			end = max(end, spans[i][1])
		}
		out.WriteString(text[position:begin])
		out.WriteString(maskReplacement)
		position = end
	}
	out.WriteString(text[position:])
	return out.String()
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
	end := maxLogMessage
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + ellipsis
}

// ellipsis marks text cut to the length bound.
const ellipsis = "..."
