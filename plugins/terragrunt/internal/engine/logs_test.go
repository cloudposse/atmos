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
	"github.com/spf13/afero"
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
	l := log.New(log.WithOutput(io.Discard), log.WithHooks(sink.Hook()))
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

// A value registered as sensitive is redacted from every event, including events collected before the
// registration, in the message, the fields and the unit. Other text is unchanged.
func TestLogSinkRedactsSensitiveValuesRegisteredLater(t *testing.T) {
	sink := NewLogSink("debug", "")
	l := logger(t, sink)
	l.WithField("token", "tok-123").WithField("port", 8443).WithField("prefix", "/units/tok-123").Debug("using tok-123 and visible")

	sink.RedactValue(map[string]any{"token": "tok-123", "ports": []any{8443}, "none": nil})

	entries := sink.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "using <MASKED> and visible", entries[0].Message)
	assert.Equal(t, map[string]string{"token": "<MASKED>", "port": "<MASKED>"}, entries[0].Fields)
	assert.Equal(t, "/units/<MASKED>", entries[0].Unit)

	l.Debug("later tok-123")
	entries = sink.Entries()
	require.Len(t, entries, 2)
	assert.Equal(t, "using <MASKED> and visible", entries[0].Message)
	assert.Equal(t, "later <MASKED>", entries[1].Message)
}

// A longer value that contains a shorter one is redacted whole.
func TestLogSinkRedactsTheLongestValueFirst(t *testing.T) {
	sink := NewLogSink("debug", "")
	logger(t, sink).Debug("key abc-123-xyz")
	sink.RedactValue("abc")
	sink.RedactValue("abc-123-xyz")

	entries := sink.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "key <MASKED>", entries[0].Message)
}

// The length bound can cut a sensitive value in half; its beginning before the ellipsis is redacted too.
// Text that was not cut keeps a matching ending.
func TestLogSinkRedactsAValueSplitByTheLengthBound(t *testing.T) {
	secret := "SECRET-VALUE-0123456789"
	sink := NewLogSink("debug", "")
	l := logger(t, sink)
	l.Debug(strings.Repeat("a", maxLogMessage-6) + secret)
	l.WithField("value", strings.Repeat("b", maxLogMessage-4)+secret).Debug("field")
	l.Debug("ends with SECRET")
	sink.RedactValue(secret)

	entries := sink.Entries()
	require.Len(t, entries, 3)
	assert.Equal(t, strings.Repeat("a", maxLogMessage-6)+"<MASKED>...", entries[0].Message)
	assert.NotContains(t, entries[0].Message, "SECRET")
	assert.Equal(t, strings.Repeat("b", maxLogMessage-4)+"<MASKED>...", entries[1].Fields["value"])
	assert.Equal(t, "ends with SECRET", entries[2].Message, "uncut text is redacted only where the whole value occurs")
}

// A shorter value that is also the beginning of a value the length bound cut must not hide the rest of that
// beginning: replacing the short value first would leave "DEF" visible.
func TestLogSinkRedactsACutValueThatOverlapsAShorterOne(t *testing.T) {
	sink := NewLogSink("debug", "")
	l := logger(t, sink)
	l.Debug(strings.Repeat("x", maxLogMessage-6) + "ABCDEFuvwxyz")
	l.Debug("ABC then ABCDEFuvwxyz then ABCABC")
	sink.RedactValue("ABCDEFuvwxyz")
	sink.RedactValue("ABC")

	entries := sink.Entries()
	require.Len(t, entries, 2)
	assert.Equal(t, strings.Repeat("x", maxLogMessage-6)+"<MASKED>...", entries[0].Message)
	assert.NotContains(t, entries[0].Message, "DEF")
	assert.Equal(t, "<MASKED> then <MASKED> then <MASKED><MASKED>", entries[1].Message, "whole values still mask one by one")
}

// Evaluation registers the inputs the unit declares sensitive, so events collected before or during the
// evaluation never carry their values, while other inputs stay readable.
func TestResolveRedactsSensitiveInputsFromLogs(t *testing.T) {
	request := nativeUnit(t, "locals { tok = \"tok-123\" }\ninputs = { token = sensitive(local.tok), nested = { key = sensitive(\"key-456\") }, name = \"visible\" }\n")
	sink := NewLogSink("trace", request.BasePath)
	logger(t, sink).Debug("before tok-123 key-456 visible")

	result, err := Resolve(WithLogSink(t.Context(), sink), request)

	require.NoError(t, err)
	require.ElementsMatch(t, [][]string{{"token"}, {"nested", "key"}}, result.SensitiveInputs)
	entries := sink.Entries()
	require.NotEmpty(t, entries)
	assert.Equal(t, "before <MASKED> <MASKED> visible", entries[0].Message)
	for _, entry := range entries {
		assert.NotContains(t, entry.Message, "tok-123")
		assert.NotContains(t, entry.Message, "key-456")
	}
}

// A sensitive value that only exists when the unit is evaluated for apply is registered by the
// apply-time reparse, so events collected before it are masked as well.
func TestResolveRedactsApplyTimeSensitiveInputsFromLogs(t *testing.T) {
	request := nativeUnit(t, "inputs = { token = sensitive(get_terraform_command() == \"apply\" ? \"apply-secret\" : \"plan-secret\"), name = \"visible\" }\n")
	sink := NewLogSink("trace", request.BasePath)
	logger(t, sink).Debug("before apply-secret plan-secret visible")

	_, err := Resolve(WithLogSink(t.Context(), sink), request)

	require.NoError(t, err)
	entries := sink.Entries()
	require.NotEmpty(t, entries)
	assert.Equal(t, "before <MASKED> <MASKED> visible", entries[0].Message)
	for _, entry := range entries {
		assert.NotContains(t, entry.Message, "apply-secret")
		assert.NotContains(t, entry.Message, "plan-secret")
	}
}

// A withheld sink returns only a notice of how many events it withheld.
func TestLogSinkWithholdReturnsOnlyANotice(t *testing.T) {
	sink := NewLogSink("debug", "")
	logger(t, sink).Debug("run_cmd output: [secret]")
	sink.Withhold()

	entries := sink.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "warn", entries[0].Level)
	assert.Equal(t, "1 Terragrunt log events were withheld because the unit declares sensitive values that could not be redacted", entries[0].Message)

	empty := NewLogSink("debug", "")
	empty.Withhold()
	assert.Empty(t, empty.Entries())
}

// The unit's own sensitive locals are redacted even when they never reach the inputs.
func TestResolveRedactsSensitiveLocalsFromLogs(t *testing.T) {
	request := nativeUnit(t, "locals {\n  tok = sensitive(\"tok-123\")\n  conf = { key = sensitive(\"key-456\"), name = \"visible\" }\n}\ninputs = { name = local.conf.name }\n")
	sink := NewLogSink("debug", request.BasePath)
	logger(t, sink).Debug("before tok-123 key-456 visible")

	_, err := Resolve(WithLogSink(t.Context(), sink), request)

	require.NoError(t, err)
	entries := sink.Entries()
	require.NotEmpty(t, entries)
	assert.Equal(t, "before <MASKED> <MASKED> visible", entries[0].Message)
}

// When the evaluation fails before the sensitive values are known, the events are withheld; a unit that
// declares nothing sensitive keeps them.
func TestResolveWithholdsLogsWhenSensitiveValuesAreUnknown(t *testing.T) {
	for _, test := range []struct {
		name     string
		hcl      string
		withheld bool
	}{
		{"sensitive", "locals { tok = sensitive(\"tok-123\") }\ninputs = { token = local.tok, broken = local.absent }\n", true},
		{"plain", "locals { tok = \"tok-123\" }\ninputs = { token = local.tok, broken = local.absent }\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := nativeUnit(t, test.hcl)
			sink := NewLogSink("debug", request.BasePath)
			logger(t, sink).Debug("before tok-123")

			_, err := Resolve(WithLogSink(t.Context(), sink), request)

			require.Error(t, err)
			entries := sink.Entries()
			require.NotEmpty(t, entries)
			if test.withheld {
				require.Len(t, entries, 1)
				assert.Contains(t, entries[0].Message, "withheld")
				return
			}
			assert.Equal(t, "before tok-123", entries[0].Message)
		})
	}
}

// A sensitive value declared where its text cannot be read back, such as in a block other than locals or in
// an included file's locals or inputs, withholds the events even when the evaluation succeeds.
func TestUnredactedSensitivityCountsOnlyUnknownValues(t *testing.T) {
	fsys := afero.NewMemMapFs()
	write := func(name, content string) string {
		path := filepath.Join(string(filepath.Separator), "repo", name)
		require.NoError(t, afero.WriteFile(fsys, path, []byte(content), 0o644))
		return path
	}
	unit := write("unit.hcl", "locals { tok = sensitive(\"a\") }\ninputs = { token = sensitive(\"b\") }\n")
	root := write("root.hcl", "locals { tok = sensitive(\"c\") }\n")
	plainRoot := write("plain.hcl", "inputs = { token = sensitive(\"d\") }\n")
	backend := write("backend.hcl", "remote_state { config = { key = sensitive(\"e\") } }\n")
	broken := write("broken.hcl", "locals {\n")
	missing := filepath.Join(string(filepath.Separator), "repo", "missing.hcl")

	assert.False(t, unredactedSensitivity(fsys, []string{unit, missing}, unit, true), "the unit's own inputs and locals are redacted")
	// A child can override an included input, so only the merged value is registered; the included value
	// may already have been logged while the included file was parsed.
	assert.True(t, unredactedSensitivity(fsys, []string{unit, plainRoot}, unit, true), "an included file's inputs are unknown")
	assert.True(t, unredactedSensitivity(fsys, []string{unit}, unit, false), "nothing is known before resolution")
	assert.True(t, unredactedSensitivity(fsys, []string{unit, root}, unit, true), "an included file's locals are unknown")
	assert.True(t, unredactedSensitivity(fsys, []string{backend}, backend, true), "other blocks are unknown")
	assert.True(t, unredactedSensitivity(fsys, []string{broken}, unit, true), "an unparseable file fails closed")
}
