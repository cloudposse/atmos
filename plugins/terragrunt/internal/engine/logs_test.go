package engine

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncatePreservesUTF8Boundary(t *testing.T) {
	short := strings.Repeat("a", maxLogMessage-1) + "é"
	got := truncate(short)
	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, strings.Repeat("a", maxLogMessage-1)+"...", got)
	assert.Equal(t, "é", truncate("é"))
}

// logger returns a Terragrunt logger that reports to the sink, at the level the sink asks for.
func logger(t *testing.T, sink *LogSink) log.Logger {
	t.Helper()
	l := log.New(log.WithOutput(io.Discard), log.WithHooks(sink.hook()))
	require.NoError(t, l.SetLevel(sink.TerragruntLevel()))
	return l
}

func TestLogSinkKeepsEventsAtOrAboveTheLevel(t *testing.T) {
	for _, test := range []struct {
		level string
		want  []string
	}{
		{"", nil},
		{"warn", nil},
		{"warning", nil},
		{"error", nil},
		{"info", nil},
		{"unknown", nil},
		{"off", nil},
		{"debug", []string{"debug", "info", "warn", "error"}},
		{"DEBUG", []string{"debug", "info", "warn", "error"}},
		{"trace", []string{"trace", "debug", "info", "warn", "error"}},
	} {
		t.Run(test.level, func(t *testing.T) {
			sink := NewLogSink(test.level, "")
			l := logger(t, sink)
			l.Trace("trace")
			l.Debug("debug")
			l.Info("info")
			l.Warn("warn")
			l.Error("error")

			var got []string
			for _, entry := range sink.Entries() {
				assert.Equal(t, entry.Level, entry.Message)
				got = append(got, entry.Level)
			}
			assert.Equal(t, test.want, got)
		})
	}
}

func TestLogSinkReportsFieldsUnitAndTimeWithoutCommandOutput(t *testing.T) {
	root := t.TempDir()
	sink := NewLogSink("debug", root)
	l := logger(t, sink)

	l.WithField("prefix", filepath.Join(root, "live", "dev", "app")).WithField("attempt", 3).WithField("failure", fmt.Errorf("boom")).Debug("evaluating")
	l.WithField("prefix", "/elsewhere/unit").Warn("outside")
	l.Logf(log.StdoutLevel, "command output")
	l.Logf(log.StderrLevel, "command error")

	entries := sink.Entries()
	require.Len(t, entries, 2, "command output is not logging")
	assert.Equal(t, "live/dev/app", entries[0].Unit)
	assert.Equal(t, map[string]string{"attempt": "3", "failure": "boom"}, entries[0].Fields)
	assert.NotContains(t, entries[0].Fields, "prefix")
	assert.False(t, entries[0].Time.IsZero())
	assert.Equal(t, "/elsewhere/unit", entries[1].Unit, "a unit outside the project keeps its path")
}

func TestLogSinkBoundsVolumeAndSaysSo(t *testing.T) {
	sink := NewLogSink("debug", "")
	l := logger(t, sink)
	for range maxLogEntries + 5 {
		l.Debug("event")
	}

	entries := sink.Entries()

	require.Len(t, entries, maxLogEntries+1)
	last := entries[len(entries)-1]
	assert.Equal(t, "warn", last.Level)
	assert.Equal(t, "5 further Terragrunt log events were omitted", last.Message)

	long := NewLogSink("debug", "")
	l = logger(t, long)
	wide := strings.Repeat("y", maxLogMessage+10)
	for range maxLogEntries {
		l.WithField("a", wide).WithField("b", wide).Debug(strings.Repeat("x", maxLogMessage+10))
	}
	entries = long.Entries()
	assert.Len(t, entries[0].Message, maxLogMessage+len("..."), "one message is truncated")
	assert.Len(t, entries[0].Fields["a"], maxLogMessage+len("..."), "one field value is truncated")
	assert.Less(t, len(entries), maxLogEntries, "the byte budget stops collection before the entry limit")
	assert.Contains(t, entries[len(entries)-1].Message, "further Terragrunt log events were omitted")
}

func TestLogSinkIsSafeForConcurrentUnits(t *testing.T) {
	sink := NewLogSink("debug", "")
	l := logger(t, sink)
	var wg sync.WaitGroup
	for unit := range 8 {
		wg.Go(func() {
			for range 50 {
				l.WithField("unit", unit).Info("working")
			}
		})
	}
	wg.Wait()
	assert.Len(t, sink.Entries(), 400)
}

// The evaluator reports the Terragrunt logger's events through the sink in the context, and only there.
func TestResolveReportsLoggerEventsToTheContextSink(t *testing.T) {
	request := nativeUnit(t, "inputs = { name = \"value\" }\n")
	sink := NewLogSink("trace", request.BasePath)

	_, err := Resolve(WithLogSink(t.Context(), sink), request)

	require.NoError(t, err)
	entries := sink.Entries()
	require.NotEmpty(t, entries, "a trace-level evaluation logs")
	for _, entry := range entries {
		assert.NotEmpty(t, entry.Message)
		assert.NotContains(t, entry.Message, "must-not-execute")
	}

	quiet := NewLogSink("warn", request.BasePath)
	_, err = Resolve(WithLogSink(t.Context(), quiet), request)
	require.NoError(t, err)
	assert.Empty(t, quiet.Entries(), "engine chatter is hidden at the Atmos default level")

	_, err = Resolve(t.Context(), request)
	require.NoError(t, err, "evaluation without a sink still works")
}
