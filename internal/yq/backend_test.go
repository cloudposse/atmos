package yq

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	logging "gopkg.in/op/go-logging.v1"

	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// recordingLogger captures the level and message of each log call, mirroring
// pkg/logger's own logrus-adapter test double.
type recordingLogger struct {
	level string
	msg   string
}

func (r *recordingLogger) Error(msg string, _ ...interface{}) { r.level = "error"; r.msg = msg }
func (r *recordingLogger) Warn(msg string, _ ...interface{})  { r.level = "warn"; r.msg = msg }
func (r *recordingLogger) Info(msg string, _ ...interface{})  { r.level = "info"; r.msg = msg }
func (r *recordingLogger) Debug(msg string, _ ...interface{}) { r.level = "debug"; r.msg = msg }

// withCapturedBackend swaps the shared backend's logger for rec and raises
// the shared level to DEBUG so every level below reaches Log, runs fn, then
// restores both. Go-logging's level gate (Logger.IsEnabledFor) always
// consults the process-global default backend, even for a per-logger
// override, so these tests exercise the real installed backend rather than
// a substitute.
func withCapturedBackend(t *testing.T, rec *recordingLogger, fn func()) {
	t.Helper()

	prevLog := goLoggingBackend.log
	prevLevel := goLoggingBackend.GetLevel(logModule)
	goLoggingBackend.log = rec
	SetLevel(logging.DEBUG)
	t.Cleanup(func() {
		goLoggingBackend.log = prevLog
		SetLevel(prevLevel)
	})

	fn()
}

func TestBackend_Log_RoutesToCorrectLevel(t *testing.T) {
	testLogger := logging.MustGetLogger(logModule)

	tests := []struct {
		name  string
		emit  func()
		level string
	}{
		{"critical routes to error", func() { testLogger.Critical("boom") }, "error"},
		{"error routes to error", func() { testLogger.Error("boom") }, "error"},
		{"warning routes to warn", func() { testLogger.Warning("careful") }, "warn"},
		{"notice routes to info", func() { testLogger.Notice("fyi") }, "info"},
		{"info routes to info", func() { testLogger.Info("fyi") }, "info"},
		{"debug routes to debug", func() { testLogger.Debug("trace") }, "debug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingLogger{}
			withCapturedBackend(t, rec, tt.emit)
			assert.Equal(t, tt.level, rec.level)
		})
	}
}

// TestBackend_Log_SilentLevelBlocksEverything pins the default (non-Trace)
// behavior: yq's own CRITICAL records must not reach the Atmos logger at all
// once SilentLevel is installed.
func TestBackend_Log_SilentLevelBlocksEverything(t *testing.T) {
	rec := &recordingLogger{}
	prevLog := goLoggingBackend.log
	prevLevel := goLoggingBackend.GetLevel(logModule)
	goLoggingBackend.log = rec
	SetLevel(SilentLevel)
	t.Cleanup(func() {
		goLoggingBackend.log = prevLog
		SetLevel(prevLevel)
	})

	logging.MustGetLogger(logModule).Critical("should not be forwarded")

	assert.Empty(t, rec.level, "SilentLevel must block even CRITICAL records")
}

// TestBackend_Log_MasksRegisteredSecrets is the regression test for routing
// yq's diagnostics through Atmos's masking-aware I/O layer: a message that
// echoes a registered secret must reach the Atmos logger already redacted.
func TestBackend_Log_MasksRegisteredSecrets(t *testing.T) {
	iolib.RegisterSecret("s3cr3t-token")
	t.Cleanup(iolib.Reset)

	rec := &recordingLogger{}
	withCapturedBackend(t, rec, func() {
		logging.MustGetLogger(logModule).Debug("using token %s", "s3cr3t-token")
	})

	assert.NotContains(t, rec.msg, "s3cr3t-token")
	assert.Contains(t, rec.msg, iolib.MaskReplacement)
}

// TestBackend_Log_ReachesConfiguredAtmosDestination proves yq diagnostics
// flow through the Atmos logger's actual configured output, not straight to
// stderr via yq's own go-logging backend.
func TestBackend_Log_ReachesConfiguredAtmosDestination(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.DebugLevel)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.WarnLevel)
	})

	prevLevel := goLoggingBackend.GetLevel(logModule)
	SetLevel(logging.DEBUG)
	t.Cleanup(func() { SetLevel(prevLevel) })

	logging.MustGetLogger(logModule).Debug("hello from yq")

	assert.Contains(t, buf.String(), "hello from yq")
}

func TestInitExpressionParser_Idempotent(t *testing.T) {
	InitExpressionParser()
	InitExpressionParser()
}

// TestWithEvaluationLevel_ScopesLevelToCall is the regression test for a gap
// the atomic level alone left open: it prevents a data race on the level's
// value, but not a Trace-configured caller and a concurrent Silent-configured
// one (e.g. pkg/yaml's editor, which always wants silent regardless of
// Atmos's configured log level) from each running under whichever level the
// other last set. Without WithEvaluationLevel's shared lock, two goroutines
// calling SetLevel and evaluating separately could interleave that way; with
// it, every caller must observe its own requested level for its entire call.
func TestWithEvaluationLevel_ScopesLevelToCall(t *testing.T) {
	prevLevel := goLoggingBackend.GetLevel(logModule)
	t.Cleanup(func() { SetLevel(prevLevel) })

	const iterations = 50

	var wg sync.WaitGroup
	violations := make(chan string, iterations*2)

	observe := func(level logging.Level) {
		defer wg.Done()
		WithEvaluationLevel(level, func() {
			start := goLoggingBackend.GetLevel(logModule)
			runtime.Gosched()
			end := goLoggingBackend.GetLevel(logModule)
			if start != level || end != level {
				violations <- fmt.Sprintf("wanted %v, observed start=%v end=%v", level, start, end)
			}
		})
	}

	for range iterations {
		wg.Add(2)
		go observe(logging.DEBUG) // stands in for a Trace-configured stack-processing evaluation.
		go observe(SilentLevel)   // stands in for the YAML editor's always-silent evaluation.
	}
	wg.Wait()
	close(violations)

	for v := range violations {
		t.Error(v)
	}
}
