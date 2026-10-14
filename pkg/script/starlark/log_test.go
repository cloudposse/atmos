package starlark

import (
	"bytes"
	"context"
	"strings"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/script"
)

// Compile-time guard: the Atmos logger must satisfy the Logger seam.
var _ Logger = (*log.AtmosLogger)(nil)

// newTestLogger returns an Atmos logger that writes plain text at level into buf.
func newTestLogger(buf *bytes.Buffer, level log.Level) *log.AtmosLogger {
	return log.NewAtmosLogger(charm.NewWithOptions(buf, charm.Options{Level: level}))
}

// runLog executes source with a logger at level and returns the log output, result, and streams.
func runLog(t *testing.T, level log.Level, source string) (logs string, result script.Result, stdout, stderr string, err error) {
	t.Helper()
	var logBuf, out, errOut bytes.Buffer
	result, err = New(WithLogger(newTestLogger(&logBuf, level))).Execute(context.Background(), script.Spec{
		Name: "deploy", Source: source, Stdout: &out, Stderr: &errOut,
	})
	return logBuf.String(), result, out.String(), errOut.String(), err
}

func TestLogLevelsRouteToLoggerWithFields(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ fn, label string }{
		{"trace", ""}, {"debug", "DEBU"}, {"info", "INFO"}, {"warn", "WARN"}, {"error", "ERRO"},
	} {
		t.Run(tc.fn, func(t *testing.T) {
			t.Parallel()
			logs, _, _, _, err := runLog(t, log.TraceLevel,
				`log.`+tc.fn+`("resolved", component="api", attempts=2, ratio=1.5, ok=True, none=None, items=[1, "a"])`)
			require.NoError(t, err)
			// Plain charm output is a single line; trace has no label without Atmos styles, so
			// assert on the message and the ordered key/value pairs.
			line := strings.TrimSpace(logs)
			assert.Equal(t, 1, strings.Count(logs, "\n"), logs)
			assert.Contains(t, line, "resolved")
			assert.Contains(t, line, "step=deploy component=api attempts=2 ratio=1.5 ok=true none=<nil> items=")
			assert.Contains(t, line, `items="[1, \"a\"]"`)
			if tc.label != "" {
				assert.Contains(t, line, tc.label)
			}
		})
	}
}

func TestLogLevelFiltering(t *testing.T) {
	t.Parallel()
	source := `
log.debug("debug detail")
log.info("info detail")
`
	t.Run("info level hides debug", func(t *testing.T) {
		t.Parallel()
		logs, _, _, _, err := runLog(t, log.InfoLevel, source)
		require.NoError(t, err)
		assert.NotContains(t, logs, "debug detail")
		assert.Contains(t, logs, "info detail")
	})
	t.Run("debug level shows both", func(t *testing.T) {
		t.Parallel()
		logs, _, _, _, err := runLog(t, log.DebugLevel, source)
		require.NoError(t, err)
		assert.Contains(t, logs, "debug detail")
		assert.Contains(t, logs, "info detail")
	})
	t.Run("trace hidden at debug", func(t *testing.T) {
		t.Parallel()
		logs, _, _, _, err := runLog(t, log.DebugLevel, `log.trace("trace detail")`)
		require.NoError(t, err)
		assert.Empty(t, logs)
	})
}

func TestLogNeverTouchesStdoutOrStepValue(t *testing.T) {
	t.Parallel()
	logs, result, stdout, stderr, err := runLog(t, log.TraceLevel, `
log.trace("a")
log.debug("b")
log.info("c")
log.warn("d")
log.error("e")
`)
	require.NoError(t, err)
	assert.NotEmpty(t, logs)
	assert.False(t, result.HasOutput)
	assert.Empty(t, result.Value)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestLogCallerFieldsOverrideAutomaticFields(t *testing.T) {
	t.Parallel()
	logs, _, _, _, err := runLog(t, log.InfoLevel, `log.info("hi", step="custom", n=1)`)
	require.NoError(t, err)
	assert.Contains(t, logs, "step=custom n=1")
	assert.Equal(t, 1, strings.Count(logs, "step="), logs)
}

func TestLogLargeIntegerFallsBackToString(t *testing.T) {
	t.Parallel()
	logs, _, _, _, err := runLog(t, log.InfoLevel, `log.info("big", n=1 << 80)`)
	require.NoError(t, err)
	assert.Contains(t, logs, "n=1208925819614629174706176")
}

func TestLogTaskFieldInsideParallel(t *testing.T) {
	t.Parallel()
	logs, _, stdout, _, err := runLog(t, log.InfoLevel, `
def work(name):
    log.info("working", who=name)
def nested():
    steps.parallel(tasks = [steps.task(name = "inner", function = work, args = ["i"]), steps.task(name = "other", function = work, args = ["o"])])
steps.parallel(tasks = [
    steps.task(name = "a", function = work, args = ["x"]),
    steps.task(name = "b", function = work, args = ["y"]),
    steps.task(name = "grp", function = nested),
])
log.info("done")
`)
	require.NoError(t, err)
	assert.Empty(t, stdout)
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	assert.Len(t, lines, 5, logs)
	assert.Contains(t, logs, "step=deploy task=a who=x")
	assert.Contains(t, logs, "step=deploy task=b who=y")
	assert.Contains(t, logs, "task=grp/inner who=i")
	assert.Contains(t, logs, "task=grp/other who=o")
	// The main thread has no task field.
	assert.Contains(t, lines[len(lines)-1], "step=deploy")
	assert.NotContains(t, lines[len(lines)-1], "task=")
}

func TestLogConcurrentCallsAreLineAtomic(t *testing.T) {
	t.Parallel()
	logs, _, _, _, err := runLog(t, log.InfoLevel, `
def work(name):
    for i in range(50):
        log.info("line", task_name=name, i=i)
steps.parallel(tasks = [steps.task(name = "t%d" % n, function = work, args = ["t%d" % n]) for n in range(6)], max_concurrency = 6)
`)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	require.Len(t, lines, 300)
	for _, line := range lines {
		assert.Equal(t, 1, strings.Count(line, "line"), line)
		assert.Contains(t, line, "task_name=")
	}
}

func TestLogInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`log.info(1)`,
		`log.info(None)`,
		`log.info()`,
		`log.info("a", "b")`,
		`log.info(message="a")`,
		`log.debug("a", **{"not an identifier": 1})`,
		`log.warn("a", **{"1abc": 1})`,
		`log.error("a", **{"": 1})`,
	} {
		logs, _, _, _, err := runLog(t, log.TraceLevel, source)
		require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument, source)
		assert.Empty(t, logs, source)
	}
}

func TestLogMasksSecretsInMessageAndFields(t *testing.T) {
	// Not parallel: registers a secret with the process-wide masker.
	iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
	const secret = "log-builtin-secret-9f3a71"
	iolib.RegisterSecret(secret)
	require.NotContains(t, iolib.MaskString(secret), secret, "precondition: masker must hide the secret")

	logs, _, _, _, err := runLog(t, log.InfoLevel,
		`log.info("token is `+secret+`", token="`+secret+`", nested=["`+secret+`"])`)
	require.NoError(t, err)
	assert.NotContains(t, logs, secret)
	assert.Contains(t, logs, "token is")
}

func TestLogDefaultsToAtmosLogger(t *testing.T) {
	// Not parallel: swaps the process-wide default logger.
	var buf bytes.Buffer
	previous := log.Default()
	log.SetDefault(newTestLogger(&buf, log.InfoLevel))
	t.Cleanup(func() { log.SetDefault(previous) })

	_, err := New().Execute(context.Background(), script.Spec{Name: "s", Source: `log.info("via default")`})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "via default")
}
