package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/pkg/config"
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

// The sink learns only the final value of a local. A sensitive local that transforms the output of a call
// leaves the raw output, which may already be logged, unredacted, so those events are withheld. A local that
// keeps the raw value stays redacted, and a transformation outside sensitive locals changes nothing.
func TestResolveWithholdsLogsForSensitiveLocalsThatTransformCallOutput(t *testing.T) {
	for _, test := range []struct {
		name     string
		locals   string
		withheld bool
	}{
		{"direct call output", `tok = sensitive(get_env("TG_UNSET_RAW", "raw-123"))`, false},
		{"literal", `tok = sensitive("raw-123")`, false},
		{"object of sensitive values", `tok = { key = sensitive(get_env("TG_UNSET_RAW", "raw-123")), name = "visible" }`, false},
		{"transformed sibling in a sensitive object", `tok = { key = sensitive(get_env("TG_UNSET_RAW", "raw-123")), name = upper("visible") }`, true},
		{"reference keeps the raw value", "raw = sensitive(get_env(\"TG_UNSET_RAW\", \"raw-123\"))\n  tok = local.raw", false},
		{"transform outside sensitive", "plain = upper(get_env(\"TG_UNSET_RAW\", \"raw-123\"))\n  tok = sensitive(\"raw-123\")", false},
		{"transform inside sensitive", `tok = sensitive(upper(get_env("TG_UNSET_RAW", "raw-123")))`, true},
		{"transform around sensitive", `tok = upper(sensitive(get_env("TG_UNSET_RAW", "raw-123")))`, true},
		{"template around the call", `tok = sensitive("${get_env("TG_UNSET_RAW", "raw-123")}-suffix")`, true},
		{"sensitive wraps a transformed local", "raw = upper(get_env(\"TG_UNSET_RAW\", \"raw-123\"))\n  tok = sensitive(local.raw)", true},
		{"transform through a reference", "raw = sensitive(get_env(\"TG_UNSET_RAW\", \"raw-123\"))\n  tok = upper(local.raw)", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := nativeUnit(t, "locals {\n  "+test.locals+"\n}\ninputs = { name = \"visible\" }\n")
			sink := NewLogSink("debug", request.BasePath)
			logger(t, sink).Debug("before raw-123 visible")

			_, err := Resolve(WithLogSink(t.Context(), sink), request)

			require.NoError(t, err)
			entries := sink.Entries()
			require.NotEmpty(t, entries)
			if test.withheld {
				require.Len(t, entries, 1)
				assert.Contains(t, entries[0].Message, "withheld")
				assert.NotContains(t, entries[0].Message, "raw-123")
				return
			}
			assert.NotContains(t, entries[0].Message, "withheld")
			assert.Equal(t, "before <MASKED> visible", entries[0].Message)
		})
	}
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

// A file only read when the unit is evaluated for apply can declare sensitive values the reparse never
// registers, such as the inputs of an included file, so the events are withheld even when the evaluation succeeds.
func TestResolveWithholdsLogsForSensitiveValuesInApplyTimeFiles(t *testing.T) {
	request := nativeUnit(t, "include \"extra\" {\n  path = get_terraform_command() == \"apply\" ? \"${get_terragrunt_dir()}/apply.hcl\" : \"${get_terragrunt_dir()}/plan.hcl\"\n}\n")
	dir := filepath.Dir(request.Path)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.hcl"), []byte("inputs = { name = \"visible\" }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "apply.hcl"), []byte("inputs = { token = sensitive(\"apply-secret\") }\n"), 0o644))
	sink := NewLogSink("debug", request.BasePath)
	logger(t, sink).Debug("before apply-secret")

	_, err := Resolve(WithLogSink(t.Context(), sink), request)

	require.NoError(t, err)
	entries := sink.Entries()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Message, "withheld")
}

// When the apply-time reparse fails, the values it would have registered are unknown, so a unit that
// declares sensitive values has its events withheld; a unit that declares none keeps them.
func TestResolveWithholdsLogsWhenTheApplyTimeReparseFails(t *testing.T) {
	for _, test := range []struct {
		name     string
		locals   string
		withheld bool
	}{
		{"sensitive", "locals { tok = sensitive(\"tok-123\") }\n", true},
		{"plain", "locals { tok = \"tok-123\" }\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := nativeUnit(t, test.locals+"include \"extra\" {\n  path = get_terraform_command() == \"apply\" ? \"${get_terragrunt_dir()}/missing.hcl\" : \"${get_terragrunt_dir()}/plan.hcl\"\n}\n")
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(request.Path), "plan.hcl"), []byte("inputs = { name = \"visible\" }\n"), 0o644))
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

func TestResolveWithholdsTransformedSensitiveInputCommandOutput(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		withheld         bool
	}{
		{"direct call", "sensitive(%s)", false},
		{"transform inside", "sensitive(upper(%s))", true},
		{"transform outside", "upper(sensitive(%s))", true},
		{"nested input", "{ key = sensitive(upper(%s)) }", true},
		{"merge overwrite", `merge({ token = sensitive(%s) }, { token = "replacement" })`, true},
		{"merge disjoint", `merge({ token = sensitive(%s) }, { public = "visible" })`, false},
		{"merge unknown keys", `merge({ token = sensitive(%s) }, jsondecode("{\"token\":\"replacement\"}"))`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := effectCommand(t, "raw-secret-73ea")
			request := nativeUnit(t, "inputs = { token = "+fmt.Sprintf(test.expression, call)+", public = upper(\"visible\") }\n")
			request.Environment[effectChildEnv] = "1"
			sink := NewLogSink("debug", request.BasePath)
			logger(t, sink).Debug("before raw-secret-73ea")
			result, err := Resolve(WithLogSink(t.Context(), sink), request)
			require.NoError(t, err)
			require.NotEmpty(t, result.SensitiveInputs)
			effects, err := os.ReadFile(filepath.Join(request.BasePath, "effects"))
			require.NoError(t, err, "the Go subprocess must actually execute")
			assert.Contains(t, string(effects), "raw-secret-73ea")
			entries := sink.Entries()
			require.NotEmpty(t, entries)
			for _, entry := range entries {
				assert.NotContains(t, entry.Message, "raw-secret-73ea")
			}
			if test.withheld {
				require.Len(t, entries, 1)
				assert.Contains(t, entries[0].Message, "withheld")
			} else {
				assert.Equal(t, "before <MASKED>", entries[0].Message)
			}
		})
	}
}

func TestApplyTimeReparseWithholdsTransformedSensitiveInput(t *testing.T) {
	request := nativeUnit(t, `inputs = { token = "plan-input", phase = get_terraform_command() }`)
	request.Environment[effectChildEnv] = "1"
	sink := NewLogSink("debug", request.BasePath)
	ctx := config.WithCaches(WithLogSink(t.Context(), sink))
	l, err := evaluationLogger(ctx, "debug")
	require.NoError(t, err)
	environment, err := evaluationEnvironment(ctx, &request, nil, request.Path)
	require.NoError(t, err)
	pctx := requestParsingContext(&request, request.Path)
	evaluator := unitEvaluator{logger: l, environment: environment, pctx: pctx, request: &request, path: request.Path}
	cfg, err := evaluator.parseUnit(ctx)
	require.NoError(t, err)
	body, err := os.ReadFile(request.Path)
	require.NoError(t, err)
	body = []byte(strings.ReplaceAll(string(body), `"plan-input"`, "sensitive(upper("+effectCommand(t, "apply-raw-secret")+"))"))
	require.NoError(t, os.WriteFile(request.Path, body, 0o600))
	l.Debug("before apply-raw-secret")
	require.NotContains(t, sink.Entries()[0].Message, "withheld")
	_, _, err = evaluator.unitExecution(ctx, cfg, &sensitivityScan{})
	require.NoError(t, err)
	entries := sink.Entries()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Message, "withheld")
	assert.NotContains(t, entries[0].Message, "apply-raw-secret")
}

func TestSensitiveInputsSelectExposedInclude(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	unit := filepath.Join(root, "app", unitFile)
	parent := filepath.Join(root, "parent.hcl")
	for _, test := range []struct {
		name   string
		path   string
		expose string
		found  bool
	}{
		{"ancestor", `find_in_parent_folders("parent.hcl")`, "true", true},
		{"absolute", fmt.Sprintf("%q", filepath.ToSlash(parent)), "true", true},
		{"missing", `"missing.hcl"`, "true", false},
		{"hidden", `"parent.hcl"`, "false", false},
		{"unknown exposure", `"parent.hcl"`, "local.expose", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			require.NoError(t, fsys.MkdirAll(filepath.Dir(unit), 0o755))
			require.NoError(t, afero.WriteFile(fsys, parent, []byte(`locals { token = sensitive("private") }`), 0o644))
			content := fmt.Sprintf("include \"root\" {\n path = %s\n expose = %s\n}\ninputs = { token = include.root.locals.token }", test.path, test.expose)
			require.NoError(t, afero.WriteFile(fsys, unit, []byte(content), 0o644))
			paths := SensitiveInputs(fsys, []string{unit, parent}, map[string]any{"token": "private"})
			if test.found {
				require.Equal(t, [][]string{{"token"}}, paths)
			} else {
				require.Empty(t, paths)
			}
		})
	}
}

func TestApplyValidationFailureRetainsSensitiveFileReads(t *testing.T) {
	request := nativeUnit(t, "include \"extra\" {\n path = get_terraform_command() == \"apply\" ? \"${get_terragrunt_dir()}/apply.hcl\" : \"${get_terragrunt_dir()}/plan.hcl\"\n}\n")
	request.Environment[effectChildEnv] = "1"
	dir := filepath.Dir(request.Path)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.hcl"), []byte("inputs = {}\n"), 0o600))
	body := fmt.Sprintf("locals {\n token = sensitive(%s)\n args = get_terraform_cli_args()\n}\n", effectCommand(t, "apply-validation-secret"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "apply.hcl"), []byte(body), 0o600))
	sink := NewLogSink("debug", request.BasePath)
	logger(t, sink).Debug("before apply-validation-secret")
	_, err := Resolve(WithLogSink(t.Context(), sink), request)
	require.ErrorIs(t, err, ErrUnsupportedExecution)
	effects, err := os.ReadFile(filepath.Join(request.BasePath, "effects"))
	require.NoError(t, err)
	require.Contains(t, string(effects), "apply-validation-secret")
	entries := sink.Entries()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "withheld")
	require.NotContains(t, entries[0].Message, "apply-validation-secret")
}

func writeFiles(t *testing.T, fsys afero.Fs, files map[string]string) {
	t.Helper()
	for name, content := range files {
		require.NoError(t, afero.WriteFile(fsys, name, []byte(content), 0o644))
	}
}

// Terragrunt reads any file as configuration when an include points at it, whatever its suffix.
func TestSensitivityScansConfigurationWithoutAnHCLSuffix(t *testing.T) {
	fsys := afero.NewMemMapFs()
	root := filepath.Join(string(filepath.Separator), "repo")
	unit, included, data := filepath.Join(root, "unit.hcl"), filepath.Join(root, "shared.config"), filepath.Join(root, "values.yaml")
	writeFiles(t, fsys, map[string]string{
		unit:     "inputs = { name = \"visible\" }\n",
		included: "inputs = { token = sensitive(\"secret\") }\n",
		data:     "token: sensitive(not-hcl\n",
	})
	inputs := map[string]any{"name": "visible", "token": "secret"}

	paths := sensitiveInputs(fsys, []string{unit, included}, inputs, unit, nil)

	assert.Equal(t, [][]string{{"token"}}, paths)
	assert.True(t, unredactedSensitivity(fsys, []string{unit, included}, unit, true), "the included file's sensitive call is unknown to the sink")
	assert.False(t, unredactedSensitivity(fsys, []string{unit, data}, unit, true), "a data file that is not HCL is not configuration")
}

func TestSensitivityKeepsJSONConfigurationConservative(t *testing.T) {
	fsys := afero.NewMemMapFs()
	unit := filepath.Join(string(filepath.Separator), "repo", "unit.hcl.json")
	writeFiles(t, fsys, map[string]string{unit: "{}"})

	assert.Equal(t, [][]string{nil}, sensitiveInputs(fsys, []string{unit}, map[string]any{"a": 1}, unit, nil))
	assert.True(t, unredactedSensitivity(fsys, []string{unit}, unit, true))
}

// A later member with the same key replaces a sensitive command result, so only the replacement reaches
// the sink while the discarded result may already have been logged.
func TestTransformsSensitiveInputsWhenALaterMemberReplacesTheKey(t *testing.T) {
	for _, test := range []struct {
		name   string
		inputs string
		want   bool
	}{
		{"same key replaced", `{ token = sensitive(run_cmd("echo", "x")), token = "other" }`, true},
		{"quoted spelling of the same key", `{ token = sensitive(run_cmd("echo", "x")), "token" = "other" }`, true},
		{"computed later key may replace it", `{ token = sensitive(run_cmd("echo", "x")), (local.name) = "other" }`, true},
		{"different keys", `{ token = sensitive(run_cmd("echo", "x")), name = "other" }`, false},
		{"replaced literal cannot have logged", `{ token = sensitive("literal"), token = "other" }`, false},
		{"replaced non-sensitive value", `{ token = run_cmd("echo", "x"), token = "other" }`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			unit := filepath.Join(string(filepath.Separator), "repo", "unit.hcl")
			writeFiles(t, fsys, map[string]string{unit: "locals { name = \"n\" }\ninputs = " + test.inputs + "\n"})

			assert.Equal(t, test.want, transformsSensitiveInputs(fsys, unit))
		})
	}
}

func readThrough(t *testing.T, fsys afero.Fs, name string) string {
	t.Helper()
	content, err := afero.ReadFile(fsys, name)
	require.NoError(t, err)
	return string(content)
}

// The checks scan the bytes that were evaluated, not whatever the disk holds afterwards.
func TestEvaluatedFSServesTheBytesTerragruntRead(t *testing.T) {
	disk := afero.NewMemMapFs()
	unit, other, unread := "/repo/unit.hcl", "/repo/other.hcl", "/repo/unread.hcl"
	writeFiles(t, disk, map[string]string{unit: "inputs = { a = 1 }\n", other: "first", unread: "never read"})
	recording := newEvaluatedFS(disk)
	assert.Equal(t, "inputs = { a = 1 }\n", readThrough(t, recording, unit))
	assert.Equal(t, "first", readThrough(t, recording, other))
	writeFiles(t, disk, map[string]string{unit: "inputs = { token = sensitive(\"x\") }\n"})

	view := recording.Evaluated()

	assert.Equal(t, "inputs = { a = 1 }\n", readThrough(t, view, unit), "the file changed on disk after evaluation")
	_, err := view.Stat(unread)
	require.ErrorIs(t, err, errNotEvaluated, "an existing file that was never read cannot be vouched for")
	assert.True(t, unverifiable(view, unread))
	_, err = view.Stat("/repo/absent.hcl")
	require.ErrorIs(t, err, os.ErrNotExist, "a file that does not exist was never evaluated")
	assert.False(t, unverifiable(view, "/repo/absent.hcl"))
}

func TestEvaluatedFSDropsFilesThatChangedBetweenReads(t *testing.T) {
	disk := afero.NewMemMapFs()
	writeFiles(t, disk, map[string]string{"/repo/unit.hcl": "first"})
	recording := newEvaluatedFS(disk)
	readThrough(t, recording, "/repo/unit.hcl")
	writeFiles(t, disk, map[string]string{"/repo/unit.hcl": "second"})
	readThrough(t, recording, "/repo/unit.hcl")

	assert.True(t, unverifiable(recording.Evaluated(), "/repo/unit.hcl"))
}

func TestEvaluatedFSCapturesOnlyCompleteSequentialReads(t *testing.T) {
	disk := afero.NewMemMapFs()
	writeFiles(t, disk, map[string]string{"/repo/partial.hcl": "abcdef", "/repo/rewound.hcl": "abcdef"})
	recording := newEvaluatedFS(disk)

	partial, err := recording.Open("/repo/partial.hcl")
	require.NoError(t, err)
	_, err = partial.Read(make([]byte, 2))
	require.NoError(t, err)
	require.NoError(t, partial.Close())

	rewound, err := recording.Open("/repo/rewound.hcl")
	require.NoError(t, err)
	_, err = rewound.Seek(3, io.SeekStart)
	require.NoError(t, err)
	_, err = io.ReadAll(rewound)
	require.NoError(t, err)
	_, err = rewound.Seek(0, io.SeekStart)
	require.NoError(t, err)
	_, err = io.ReadAll(rewound)
	require.NoError(t, err)

	view := recording.Evaluated()
	assert.True(t, unverifiable(view, "/repo/partial.hcl"), "an unfinished read is incomplete")
	assert.Equal(t, "abcdef", readThrough(t, view, "/repo/rewound.hcl"), "a rewound read that reaches the end is complete")
}

// A file that cannot be vouched for protects every input and withholds the logs.
func TestSensitivityProtectsWhatWasNotEvaluated(t *testing.T) {
	disk := afero.NewMemMapFs()
	unit := "/repo/unit.hcl"
	writeFiles(t, disk, map[string]string{unit: "inputs = { a = 1 }\n"})
	view := newEvaluatedFS(disk).Evaluated()

	assert.Equal(t, [][]string{nil}, sensitiveInputs(view, []string{unit}, map[string]any{"a": 1}, unit, nil))
	assert.True(t, unredactedSensitivity(view, []string{unit}, unit, true))
}

// A local named through a literal index is the same local as one named through an attribute.
func TestSensitiveInputsResolveLiteralIndexedLocalNames(t *testing.T) {
	fsys := afero.NewMemMapFs()
	writeFiles(t, fsys, map[string]string{"/unit.hcl": `locals {
  token = sensitive("t")
  plain = "visible"
}
inputs = {
  attribute = local.token
  indexed   = local["token"]
  nested    = { key = local["token"] }
  open      = local["plain"]
  attr_open = local.plain
}
`})
	inputs := map[string]any{"attribute": "t", "indexed": "t", "nested": map[string]any{"key": "t"}, "open": "visible", "attr_open": "visible"}

	assert.Equal(t, [][]string{{"attribute"}, {"indexed"}, {"nested", "key"}}, SensitiveInputs(fsys, []string{"/unit.hcl"}, inputs))
}

// A child that copies a parent's exposed input keeps the sensitivity even when the parent's inputs are not
// merged into the child's, because the parent's own path is then absent from the evaluated inputs.
func TestSensitiveInputsFollowExposedParentInputs(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	unit := filepath.Join(root, "app", unitFile)
	parent := filepath.Join(root, "parent.hcl")
	child := "include \"root\" {\n  path = find_in_parent_folders(\"parent.hcl\")\n  expose = true\n}\n"
	for _, test := range []struct {
		name, parent, inputs string
		files                []string
		want                 [][]string
	}{
		{"attribute copy", `inputs = { token = sensitive("s"), open = "o" }`, "copy = include.root.inputs.token\n open = include.root.inputs.open", []string{unit, parent}, [][]string{{"copy"}}},
		{"indexed copy", `inputs = { token = sensitive("s"), open = "o" }`, `copy = include.root.inputs["token"]`, []string{unit, parent}, [][]string{{"copy"}}},
		{"copy of every input", `inputs = { token = sensitive("s") }`, "copy = include.root.inputs", []string{unit, parent}, [][]string{{"copy", "token"}}},
		{"parent sensitive through its local", "locals { t = sensitive(\"s\") }\ninputs = { token = local.t }", "copy = include.root.inputs.token", []string{unit, parent}, [][]string{{"copy"}}},
		{"only non-sensitive inputs are copied", `inputs = { token = sensitive("s"), open = "o" }`, "open = include.root.inputs.open", []string{unit, parent}, nil},
		{"parent without inputs", `locals { a = 1 }`, "copy = include.root.inputs.token", []string{unit, parent}, nil},
		{"exposed parent that cannot be read", `inputs = { token = sensitive("s") }`, "copy = include.root.inputs.token", []string{unit}, [][]string{{"copy"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			require.NoError(t, fsys.MkdirAll(filepath.Dir(unit), 0o755))
			writeFiles(t, fsys, map[string]string{parent: test.parent, unit: child + "inputs = {\n " + test.inputs + "\n}\n"})
			// With no_merge the child's evaluated inputs hold only its own keys.
			inputs := map[string]any{"copy": "s", "open": "o"}
			if test.name == "copy of every input" {
				inputs["copy"] = map[string]any{"token": "s"}
			}

			assert.Equal(t, test.want, SensitiveInputs(fsys, test.files, inputs))
		})
	}
}

// A copy of an exposed include's whole locals object carries every sensitive local with it, at the path of
// that local, while selecting one local keeps the precise result.
func TestSensitiveInputsFollowACopyOfExposedIncludeLocals(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	unit := filepath.Join(root, "app", unitFile)
	parent := filepath.Join(root, "parent.hcl")
	child := "include \"root\" {\n  path = find_in_parent_folders(\"parent.hcl\")\n  expose = true\n}\n"
	for _, test := range []struct {
		name, parent, inputs string
		files                []string
		value                any
		want                 [][]string
	}{
		{"sensitive local", "locals {\n token = sensitive(\"s\")\n open = \"o\"\n}", "copy = include.root.locals", []string{unit, parent}, map[string]any{"token": "s", "open": "o"}, [][]string{{"copy", "token"}}},
		{"nested sensitive member", `locals { a = { x = sensitive("s"), y = "o" } }`, "copy = include.root.locals", []string{unit, parent}, map[string]any{"a": map[string]any{"x": "s", "y": "o"}}, [][]string{{"copy", "a", "x"}}},
		{"sensitive through another local", "locals {\n t = sensitive(\"s\")\n copy = local.t\n}", "copy = include.root.locals", []string{unit, parent}, map[string]any{"t": "s", "copy": "s"}, [][]string{{"copy", "copy"}, {"copy", "t"}}},
		{"no sensitive local", "locals {\n a = \"o\"\n b = \"p\"\n}", "copy = include.root.locals", []string{unit, parent}, map[string]any{"a": "o", "b": "p"}, nil},
		{"exposed parent that cannot be read", `locals { token = sensitive("s") }`, "copy = include.root.locals", []string{unit}, "s", [][]string{{"copy"}}},
		{"one local selected", "locals {\n token = sensitive(\"s\")\n open = \"o\"\n}", "copy = include.root.locals.open", []string{unit, parent}, "o", nil},
		{"sensitive local selected", "locals {\n token = sensitive(\"s\")\n open = \"o\"\n}", "copy = include.root.locals.token", []string{unit, parent}, "s", [][]string{{"copy"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			require.NoError(t, fsys.MkdirAll(filepath.Dir(unit), 0o755))
			writeFiles(t, fsys, map[string]string{parent: test.parent, unit: child + "inputs = {\n " + test.inputs + "\n}\n"})

			assert.Equal(t, test.want, SensitiveInputs(fsys, test.files, map[string]any{"copy": test.value}))
		})
	}
}

// A later computed key may name the requested member, so an earlier static member cannot be selected
// without scanning the whole object, unless the later key is provably different.
func TestSensitiveInputsReverseMemberLookupAccountsForComputedKeys(t *testing.T) {
	fsys := afero.NewMemMapFs()
	writeFiles(t, fsys, map[string]string{"/unit.hcl": `locals {
  name      = "token"
  computed  = { token = "open", (local.name) = sensitive("s") }
  different = { token = "open", ("other") = sensitive("s") }
  replaced  = { token = sensitive("s"), (local.name) = "open" }
  static    = { token = "open", token = sensitive("s") }
  earlier   = { (local.name) = "open", token = "visible" }
}
inputs = {
  computed  = local.computed.token
  different = local.different.token
  replaced  = local.replaced.token
  static    = local.static.token
  earlier   = local.earlier.token
}
`})
	inputs := map[string]any{"computed": "s", "different": "open", "replaced": "open", "static": "s", "earlier": "visible"}

	// Statically different keys keep the precise result; only an unresolvable later key falls back.
	assert.Equal(t, [][]string{{"computed"}, {"replaced"}, {"static"}}, SensitiveInputs(fsys, []string{"/unit.hcl"}, inputs))
}

// A sensitive predicate is unknown to the sink even when neither branch is sensitive: only the selected
// result is registered, so the command output the predicate consumed may already have been logged.
func TestTransformsSensitiveInputsChecksSensitivePredicates(t *testing.T) {
	for _, test := range []struct {
		name, inputs string
		want         bool
	}{
		{"sensitive command in the predicate", `{ flag = sensitive(run_cmd("echo", "x")) == "x" ? "a" : "b" }`, true},
		{"predicate of a plain value", `sensitive(run_cmd("echo", "x")) == "x" ? "a" : "b"`, true},
		{"predicate nested in a call", `{ flag = upper(sensitive(run_cmd("echo", "x")) == "x" ? "a" : "b") }`, true},
		{"predicate under an object member", `{ nested = { flag = sensitive(run_cmd("echo", "x")) == "x" ? 1 : 2 } }`, true},
		{"literal predicate", `{ flag = true ? "a" : "b" }`, false},
		{"literal sensitive predicate cannot have logged", `{ flag = sensitive("x") == "x" ? "a" : "b" }`, false},
		{"command predicate that is not sensitive", `{ flag = run_cmd("echo", "x") == "x" ? "a" : "b" }`, false},
		{"literal conditional with a sensitive literal branch", `{ flag = true ? sensitive("s") : "b" }`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			writeFiles(t, fsys, map[string]string{"/unit.hcl": "inputs = " + test.inputs + "\n"})

			assert.Equal(t, test.want, transformsSensitiveInputs(fsys, "/unit.hcl"))
		})
	}
}

// A sensitive predicate in a local is as unknown to the sink as one in an input.
func TestTransformsSensitiveLocalChecksSensitivePredicates(t *testing.T) {
	for _, test := range []struct {
		name, local string
		want        bool
	}{
		{"sensitive command in the predicate", `sensitive(run_cmd("echo", "x")) == "x" ? "a" : "b"`, true},
		{"literal predicate", `true ? "a" : "b"`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			writeFiles(t, fsys, map[string]string{"/unit.hcl": "locals { choice = " + test.local + " }\n"})

			assert.Equal(t, test.want, transformsSensitiveLocal(fsys, "/unit.hcl"))
		})
	}
}

// Selecting one member, or one element, drops the rest of the collection. A dropped sensitive command
// result is never registered with the sink, so the events are withheld unless the selection retains it.
func TestTransformsSensitiveInputsWithholdsWhenASelectionDiscardsSensitiveValues(t *testing.T) {
	const call = `run_cmd("echo", "x")`
	for _, test := range []struct {
		name, inputs string
		want         bool
	}{
		{"index drops a sensitive element", `{ token = [sensitive(` + call + `), "b"][1] }`, true},
		{"index keeps the sensitive element", `{ token = [sensitive(` + call + `), "b"][0] }`, false},
		{"key drops a sensitive member", `{ token = { a = sensitive(` + call + `), b = "b" }["b"] }`, true},
		{"key keeps the sensitive member", `{ token = { a = sensitive(` + call + `), b = "b" }["a"] }`, false},
		{"attribute drops a sensitive member", `{ token = ({ a = sensitive(` + call + `), b = "b" }).b }`, true},
		{"attribute keeps the sensitive member", `{ token = ({ a = sensitive(` + call + `), b = "b" }).a }`, false},
		{"nested selection drops a sibling", `{ token = { a = { x = sensitive(` + call + `), y = "y" } }["a"].y }`, true},
		{"nested selection keeps the member", `{ token = { a = { x = sensitive(` + call + `), y = "y" } }["a"].x }`, false},
		{"sensitive wrapper makes every member sensitive", `{ token = sensitive({ a = ` + call + `, b = "b" })["b"] }`, true},
		{"unknown key may drop it", `{ token = [sensitive(` + call + `), "b"][length("ab") - 1] }`, true},
		{"sensitive command as the key", `{ token = { a = "a" }[sensitive(` + call + `)] }`, true},
		{"selection of a literal cannot have logged", `{ token = [sensitive("lit"), "b"][1] }`, false},
		{"selection with no sensitive value", `{ token = ["a", "b"][1] }`, false},
		{"numeric string selects the tuple element it names", `{ token = [sensitive(` + call + `), "b"]["0"] }`, false},
		{"numeric string drops the other tuple element", `{ token = [sensitive(` + call + `), "b"]["1"] }`, true},
		{"fractional number with an integral value selects an element", `{ token = [sensitive(` + call + `), "b"][1.0] }`, true},
		{"string key beyond the tuple fails closed", `{ token = [sensitive(` + call + `), "b"]["5"] }`, true},
		{"string key that is not a number fails closed", `{ token = [sensitive(` + call + `), "b"]["x"] }`, true},
		{"fractional key fails closed", `{ token = [sensitive(` + call + `), "b"][0.5] }`, true},
		{"number selects the object member it names", `{ token = { "1" = sensitive(` + call + `), "2" = "b" }[1] }`, false},
		{"number drops the other object member", `{ token = { "1" = sensitive(` + call + `), "2" = "b" }[2] }`, true},
		{"splat drops a sensitive member", `{ token = [{ a = sensitive(` + call + `), b = "b" }][*].b }`, true},
		{"splat keeps the sensitive member", `{ token = [{ a = sensitive(` + call + `), b = "b" }][*].a }`, false},
		{"splat keeps every element", `{ token = [sensitive(` + call + `), "b"][*] }`, false},
		{"splat keeps the sensitive member of each element", `{ token = [{ a = sensitive(` + call + `), b = "b" }, { a = "x", b = "y" }][*].a }`, false},
		{"splat drops a sensitive member of a later element", `{ token = [{ a = "x", b = "y" }, { a = sensitive(` + call + `), b = "b" }][*].b }`, true},
		{"legacy attribute splat drops a sensitive member", `{ token = [{ a = sensitive(` + call + `), b = "b" }].*.b }`, true},
		{"splat of a single object drops a sensitive member", `{ token = { a = sensitive(` + call + `), b = "b" }[*].b }`, true},
		{"splat through a sensitive wrapper drops a sibling", `{ token = sensitive([{ a = ` + call + `, b = "b" }])[*].b }`, true},
		{"splat of a literal cannot have logged", `{ token = [{ a = sensitive("lit"), b = "b" }][*].b }`, false},
		{"splat with no sensitive value", `{ token = [{ a = "a", b = "b" }][*].b }`, false},
		{"splat of an unfollowable source fails closed", `{ token = sensitive(` + call + `)[*].b }`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			writeFiles(t, fsys, map[string]string{"/unit.hcl": "inputs = " + test.inputs + "\n"})

			assert.Equal(t, test.want, transformsSensitiveInputs(fsys, "/unit.hcl"))
		})
	}
}

func TestResolveWithholdsLogsWhenASelectionDiscardsSensitiveCommandOutput(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		withheld         bool
	}{
		{"index drops the sensitive element", `[sensitive(%s), "b"][1]`, true},
		{"index keeps the sensitive element", `[sensitive(%s), "b"][0]`, false},
		{"key drops the sensitive member", `{ a = sensitive(%s), b = "b" }["b"]`, true},
		{"key keeps the sensitive member", `{ a = sensitive(%s), b = "b" }["a"]`, false},
		{"numeric string keeps the sensitive element", `[sensitive(%s), "b"]["0"]`, false},
		{"numeric string drops the sensitive element", `[sensitive(%s), "b"]["1"]`, true},
		{"splat drops the sensitive member", `[{ a = sensitive(%s), b = "b" }][*].b`, true},
		{"splat keeps the sensitive member", `[{ a = sensitive(%s), b = "b" }][*].a`, false},
		{"sensitive predicate", `sensitive(%s) == "raw-secret-91cd" ? "a" : "b"`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := effectCommand(t, "raw-secret-91cd")
			request := nativeUnit(t, "inputs = { token = "+fmt.Sprintf(test.expression, call)+" }\n")
			request.Environment[effectChildEnv] = "1"
			sink := NewLogSink("debug", request.BasePath)
			logger(t, sink).Debug("before raw-secret-91cd")

			_, err := Resolve(WithLogSink(t.Context(), sink), request)

			require.NoError(t, err)
			effects, err := os.ReadFile(filepath.Join(request.BasePath, "effects"))
			require.NoError(t, err, "the Go subprocess must actually execute")
			assert.Contains(t, string(effects), "raw-secret-91cd")
			entries := sink.Entries()
			require.NotEmpty(t, entries)
			for _, entry := range entries {
				assert.NotContains(t, entry.Message, "raw-secret-91cd")
			}
			if test.withheld {
				require.Len(t, entries, 1)
				assert.Contains(t, entries[0].Message, "withheld")
			} else {
				assert.Equal(t, "before <MASKED>", entries[0].Message)
			}
		})
	}
}

// The sink registers values, never object keys, so a sensitive command result used as a key cannot be
// masked later. An ordinary computed key must not trigger the fallback.
func TestTransformsSensitiveInputsWithholdsSensitiveObjectKeys(t *testing.T) {
	for _, test := range []struct {
		name, inputs string
		want         bool
	}{
		{"sensitive command as a key", `{ (sensitive(run_cmd("echo", "x"))) = "v" }`, true},
		{"sensitive command key in a nested object", `{ nested = { (sensitive(run_cmd("echo", "x"))) = "v" } }`, true},
		{"computed key from a local", "{ (local.name) = \"v\" }", false},
		{"computed key from a command that is not sensitive", `{ (run_cmd("echo", "x")) = "v" }`, false},
		{"static key", `{ name = "v" }`, false},
		{"sensitive literal key cannot have logged", `{ (sensitive("lit")) = "v" }`, false},
		{"sensitive command as a value", `{ name = sensitive(run_cmd("echo", "x")) }`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			writeFiles(t, fsys, map[string]string{"/unit.hcl": "locals { name = \"n\" }\ninputs = " + test.inputs + "\n"})

			assert.Equal(t, test.want, transformsSensitiveInputs(fsys, "/unit.hcl"))
		})
	}
}

// A file that changed between reads cannot be vouched for, whether it still exists or was deleted since.
func TestEvaluatedFSKeepsChangedFilesUnverifiableAfterDeletion(t *testing.T) {
	disk := afero.NewMemMapFs()
	writeFiles(t, disk, map[string]string{"/repo/unit.hcl": "first", "/repo/stable.hcl": "stable"})
	recording := newEvaluatedFS(disk)
	readThrough(t, recording, "/repo/unit.hcl")
	readThrough(t, recording, "/repo/stable.hcl")
	writeFiles(t, disk, map[string]string{"/repo/unit.hcl": "second"})
	readThrough(t, recording, "/repo/unit.hcl")
	require.NoError(t, disk.Remove("/repo/unit.hcl"))
	require.NoError(t, disk.Remove("/repo/stable.hcl"))

	view := recording.Evaluated()

	_, err := view.Stat("/repo/unit.hcl")
	require.ErrorIs(t, err, errNotEvaluated, "the deleted file changed while it was evaluated")
	_, err = view.Open("/repo/unit.hcl")
	require.ErrorIs(t, err, errNotEvaluated)
	assert.True(t, unverifiable(view, "/repo/unit.hcl"))
	assert.Equal(t, "stable", readThrough(t, view, "/repo/stable.hcl"), "an unchanged file keeps its captured bytes after deletion")
	_, err = view.Stat("/repo/never-read.hcl")
	require.ErrorIs(t, err, os.ErrNotExist, "a path that was never read stays missing")
	assert.False(t, unverifiable(view, "/repo/never-read.hcl"))
}

// Copying the whole local object carries every sensitive local with it, at the path of that local, while
// naming one local keeps the precise result so unrelated siblings gain no mark.
func TestSensitiveInputsFollowACopyOfTheWholeLocalObject(t *testing.T) {
	for _, test := range []struct {
		name, locals, inputs string
		value                any
		want                 [][]string
	}{
		{"sensitive local", "token = sensitive(\"s\")\n open = \"o\"", "copy = local", map[string]any{"token": "s", "open": "o"}, [][]string{{"copy", "token"}}},
		{"nested sensitive member", `a = { x = sensitive("s"), y = "o" }`, "copy = local", map[string]any{"a": map[string]any{"x": "s", "y": "o"}}, [][]string{{"copy", "a", "x"}}},
		{"no sensitive local", "a = \"o\"\n b = \"p\"", "copy = local", map[string]any{"a": "o", "b": "p"}, nil},
		{"named local keeps siblings clean", "token = sensitive(\"s\")\n open = \"o\"", "copy = local.open", "o", nil},
		{"named sensitive local", "token = sensitive(\"s\")\n open = \"o\"", "copy = local.token", "s", [][]string{{"copy"}}},
		{"copy through merge", "token = sensitive(\"s\")", "copy = merge(local, { extra = \"e\" })", map[string]any{"token": "s", "extra": "e"}, [][]string{{"copy", "token"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			writeFiles(t, fsys, map[string]string{"/unit.hcl": "locals {\n " + test.locals + "\n}\ninputs = {\n " + test.inputs + "\n}\n"})

			assert.Equal(t, test.want, SensitiveInputs(fsys, []string{"/unit.hcl"}, map[string]any{"copy": test.value}))
		})
	}
}

// A value read from another configuration file is sensitive when that file's inputs or locals can hold a
// sensitive value, and fails closed when the file cannot be inspected.
func TestSensitiveInputsFollowReadTerragruntConfig(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	unit := filepath.Join(root, "app", unitFile)
	settings := filepath.Join(root, "app", "settings.hcl")
	other := filepath.Join(root, "app", "other.hcl")
	for _, test := range []struct {
		name, unit string
		files      map[string]string
		read       []string
		want       [][]string
	}{
		{"sensitive input", `copy = read_terragrunt_config("settings.hcl").inputs.token`, map[string]string{settings: `inputs = { token = sensitive("s") }`}, []string{settings}, [][]string{{"copy"}}},
		{"whole result", `copy = read_terragrunt_config("settings.hcl")`, map[string]string{settings: `inputs = { token = sensitive("s") }`}, []string{settings}, [][]string{{"copy"}}},
		{"indexed result", `copy = read_terragrunt_config("settings.hcl")["inputs"]["token"]`, map[string]string{settings: `inputs = { token = sensitive("s") }`}, []string{settings}, [][]string{{"copy"}}},
		{"sensitive local", `copy = read_terragrunt_config("settings.hcl").locals.token`, map[string]string{settings: "locals { token = sensitive(\"s\") }"}, []string{settings}, [][]string{{"copy"}}},
		{"through a local of the unit", "copy = local.cfg.inputs.token", map[string]string{settings: `inputs = { token = sensitive("s") }`}, []string{settings}, [][]string{{"copy"}}},
		{"through another read", `copy = read_terragrunt_config("other.hcl").inputs.token`, map[string]string{other: `inputs = { token = read_terragrunt_config("settings.hcl").inputs.token }`, settings: `inputs = { token = sensitive("s") }`}, []string{other, settings}, [][]string{{"copy"}}},
		{"found in a parent folder", `copy = read_terragrunt_config(find_in_parent_folders("settings.hcl")).inputs.token`, map[string]string{filepath.Join(root, "settings.hcl"): `inputs = { token = sensitive("s") }`}, []string{filepath.Join(root, "settings.hcl")}, [][]string{{"copy"}}},
		{"file that cannot be inspected", `copy = read_terragrunt_config("missing.hcl").inputs.token`, nil, nil, [][]string{{"copy"}}},
		{"argument that is not static", `copy = read_terragrunt_config(get_env("FILE")).inputs.token`, nil, nil, [][]string{{"copy"}}},
		{"nothing sensitive", `copy = read_terragrunt_config("settings.hcl").inputs.token`, map[string]string{settings: `inputs = { token = "t" }`}, []string{settings}, nil},
		{"sensitive only outside inputs and locals", `copy = read_terragrunt_config("settings.hcl").inputs.token`, map[string]string{settings: "inputs = { token = \"t\" }\nterraform { source = sensitive(\"x\") }"}, []string{settings}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			require.NoError(t, fsys.MkdirAll(filepath.Dir(unit), 0o755))
			files := map[string]string{unit: "locals {\n cfg = read_terragrunt_config(\"settings.hcl\")\n}\ninputs = {\n " + test.unit + "\n}\n"}
			for name, content := range test.files {
				files[name] = content
			}
			writeFiles(t, fsys, files)

			assert.Equal(t, test.want, SensitiveInputs(fsys, append([]string{unit}, test.read...), map[string]any{"copy": "s"}))
		})
	}
}

// An expanded final argument is a list of merge operands, so its members replace earlier ones like any other
// argument; a final argument that is not a literal tuple fails closed.
func TestTransformsSensitiveInputsWhenAnExpandedMergeArgumentReplacesTheKey(t *testing.T) {
	const call = `sensitive(run_cmd("echo", "x"))`
	for _, test := range []struct {
		name, inputs string
		want         bool
	}{
		{"tuple member replaces the key", `merge({ token = ` + call + ` }, [{ token = "other" }]...)`, true},
		{"later tuple member replaces the key", `merge({ token = ` + call + ` }, [{ name = "n" }, { token = "other" }]...)`, true},
		{"tuple members keep different keys", `merge({ token = ` + call + ` }, [{ name = "n" }, { other = "o" }]...)`, false},
		{"tuple member with an unknown key", `merge({ token = ` + call + ` }, [{ (local.name) = "n" }]...)`, true},
		{"sensitive member of the tuple replaced by a later one", `merge([{ token = ` + call + ` }, { token = "other" }]...)`, true},
		{"tuple of sensitive members with distinct keys", `merge([{ token = ` + call + ` }, { name = "n" }]...)`, false},
		{"final argument that is not a tuple fails closed", `merge({ token = ` + call + ` }, local.list...)`, true},
		{"expanded final argument holding no sensitive value", `merge({ token = "t" }, local.list...)`, false},
		{"unexpanded tuple is one operand", `merge({ token = ` + call + ` }, { name = "n" })`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			writeFiles(t, fsys, map[string]string{"/unit.hcl": "locals {\n name = \"n\"\n list = [{ name = \"n\" }]\n}\ninputs = " + test.inputs + "\n"})

			assert.Equal(t, test.want, transformsSensitiveInputs(fsys, "/unit.hcl"))
		})
	}
}
