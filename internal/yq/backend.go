// Package yq centralizes yq's process-global state: its go-logging backend,
// module log level, and expression parser. It forwards yq's internal
// diagnostics through the Atmos logger so they inherit Atmos's formatting,
// configured log destination, and secret masking, instead of yq's own
// go-logging backend writing straight to stderr, unformatted and unmasked.
package yq

import (
	"sync"
	"sync/atomic"

	"github.com/mikefarah/yq/v4/pkg/yqlib"
	logging "gopkg.in/op/go-logging.v1"

	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// moduleKey is the Atmos logger key used to tag forwarded yq log lines with
// their originating go-logging module.
const moduleKey = "module"

// logModule is the go-logging module name that yq registers its internal
// logger under.
const logModule = "yq-lib"

// SilentLevel is lower than go-logging's CRITICAL level, so IsEnabledFor()
// rejects every message. Exported so callers that always want yq silent
// (e.g. the YAML editor) share this sentinel instead of each defining
// their own copy.
const SilentLevel logging.Level = -1

// backend implements go-logging's Backend and Leveled interfaces directly,
// storing its level in an atomic int rather than a map.
//
// Calling logging.SetBackend wraps a plain Backend in go-logging's built-in
// moduleLeveled type, whose level registry is a plain map with no
// synchronization of its own; yq reads that registry from every decoder it
// runs, so an unguarded write from one goroutine while another decodes ends
// the process with a runtime fatal error that recover cannot intercept
// (#2821). AddModuleLevel only wraps a backend that does not already
// satisfy LeveledBackend, so a backend that implements Leveled itself, as
// this one does, is installed as-is and the map is never in the picture.
type backend struct {
	level atomic.Int32
	log   levelLogger
}

// levelLogger abstracts the level-dispatched logging calls that Log() uses.
// Production code uses the global Atmos logger via atmosLevelLogger; tests
// inject a recording implementation to assert level routing and masking
// without touching the real global logger. Mirrors pkg/logger's own
// logrusAdapter test-injection pattern.
type levelLogger interface {
	Error(msg string, keyvals ...interface{})
	Warn(msg string, keyvals ...interface{})
	Info(msg string, keyvals ...interface{})
	Debug(msg string, keyvals ...interface{})
}

// atmosLevelLogger delegates to the package-level Atmos log functions.
type atmosLevelLogger struct{}

func (atmosLevelLogger) Error(msg string, kv ...interface{}) {
	defer perf.Track(nil, "yq.atmosLevelLogger.Error")()

	log.Error(msg, kv...)
}

func (atmosLevelLogger) Warn(msg string, kv ...interface{}) {
	defer perf.Track(nil, "yq.atmosLevelLogger.Warn")()

	log.Warn(msg, kv...)
}

func (atmosLevelLogger) Info(msg string, kv ...interface{}) {
	defer perf.Track(nil, "yq.atmosLevelLogger.Info")()

	log.Info(msg, kv...)
}

func (atmosLevelLogger) Debug(msg string, kv ...interface{}) {
	defer perf.Track(nil, "yq.atmosLevelLogger.Debug")()

	log.Debug(msg, kv...)
}

// GetLevel implements logging.Leveled. The module parameter is accepted
// only to satisfy the interface; this backend only ever serves yq's single
// "yq-lib" module.
func (b *backend) GetLevel(_ string) logging.Level {
	defer perf.Track(nil, "yq.backend.GetLevel")()

	return logging.Level(b.level.Load())
}

// SetLevel implements logging.Leveled. Safe for concurrent callers: the
// level is stored atomically instead of in go-logging's own unsynchronized
// module-level map.
func (b *backend) SetLevel(level logging.Level, _ string) {
	defer perf.Track(nil, "yq.backend.SetLevel")()

	//nolint:gosec // G115: level is always one of go-logging's small Level constants (-1..5), never a value that could overflow int32.
	b.level.Store(int32(level))
}

// IsEnabledFor implements logging.Leveled. This is yq's hot path: every
// decoder call checks it before formatting and logging a record.
func (b *backend) IsEnabledFor(level logging.Level, module string) bool {
	defer perf.Track(nil, "yq.backend.IsEnabledFor")()

	return level <= b.GetLevel(module)
}

// Log implements logging.Backend, forwarding yq's log records to the Atmos
// logger so they inherit Atmos's formatting and configured output, and
// masking the message with the same global masker Atmos's I/O layer uses
// for its own output, so any secret data yq's diagnostics echo back is
// redacted the same way.
func (b *backend) Log(level logging.Level, _ int, rec *logging.Record) error {
	defer perf.Track(nil, "yq.backend.Log")()

	msg := iolib.MaskString(rec.Message())
	switch level {
	case logging.CRITICAL, logging.ERROR:
		b.log.Error(msg, moduleKey, rec.Module)
	case logging.WARNING:
		b.log.Warn(msg, moduleKey, rec.Module)
	case logging.NOTICE, logging.INFO:
		b.log.Info(msg, moduleKey, rec.Module)
	default: // logging.DEBUG
		b.log.Debug(msg, moduleKey, rec.Module)
	}
	return nil
}

// goLoggingBackend is the process-global backend installed for yq's
// "yq-lib" module. Yq only ever logs under that one module, so a single
// backend instance is enough state.
var goLoggingBackend = &backend{log: atmosLevelLogger{}}

// parserOnce guards yq's process-global expression parser.
var parserOnce sync.Once

// init installs the Atmos-backed logging backend and the default (silent)
// yq log level before main runs, and therefore before any goroutine exists.
func init() {
	logging.SetBackend(goLoggingBackend)
	logging.SetLevel(SilentLevel, logModule)
}

// SetLevel installs level for yq's logger. Safe for concurrent callers.
func SetLevel(level logging.Level) {
	defer perf.Track(nil, "yq.SetLevel")()

	if goLoggingBackend.GetLevel(logModule) == level {
		return
	}
	goLoggingBackend.SetLevel(level, logModule)
}

// evalMu serializes "set level, then evaluate" as one unit across every
// caller. The atomic level on backend prevents a data race on the value
// itself, but does not keep that value stable for the duration of one
// evaluation: without this lock, a Trace-configured stack-processing
// evaluation and a concurrent Silent-configured YAML-editor call (the
// editor always wants silent, regardless of Atmos's configured log level)
// could each set their own level and then run under whichever level the
// other caller set last, instead of their own.
var evalMu sync.Mutex

// WithEvaluationLevel sets yq's logger level and runs fn with that level
// held fixed for the whole call, serialized against every other caller of
// WithEvaluationLevel. Every caller that evaluates a yq expression -- both
// pkg/utils, whose level depends on Atmos's configured log level, and
// pkg/yaml's editor, which always wants SilentLevel -- must run its
// evaluation through this function rather than calling SetLevel and
// evaluating separately.
func WithEvaluationLevel(level logging.Level, fn func()) {
	defer perf.Track(nil, "yq.WithEvaluationLevel")()

	evalMu.Lock()
	defer evalMu.Unlock()

	SetLevel(level)
	fn()
}

// InitExpressionParser initializes yq's process-global expression parser
// exactly once. Yqlib's InitExpressionParser guards yqlib.ExpressionParser
// with a plain nil check, and every Evaluate call runs it, so concurrent
// first evaluations would otherwise write and read that global at the same
// time (#2821). A sync.Once gives later readers a happens-before edge and
// keeps the one-time cost off commands that never evaluate a yq expression.
func InitExpressionParser() {
	defer perf.Track(nil, "yq.InitExpressionParser")()

	parserOnce.Do(yqlib.InitExpressionParser)
}
