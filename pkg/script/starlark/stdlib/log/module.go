package log

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"go.starlark.net/starlark"

	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

// Logger is the diagnostics sink behind the `log` module. It is satisfied by *log.AtmosLogger,
// which honors --logs-level, ATMOS_LOGS_LEVEL, and logs.file.
type Logger interface {
	Trace(msg interface{}, keyvals ...interface{})
	Debug(msg interface{}, keyvals ...interface{})
	Info(msg interface{}, keyvals ...interface{})
	Warn(msg interface{}, keyvals ...interface{})
	Error(msg interface{}, keyvals ...interface{})
}

const (
	stepField = "step"
	taskField = "task"
)

// logFieldName matches the identifiers allowed as structured field names.
var logFieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// logLevels maps each log.* function to the Logger method it calls.
var logLevels = map[string]func(Logger, string, []any){
	"trace": func(l Logger, msg string, kv []any) { l.Trace(msg, kv...) },
	"debug": func(l Logger, msg string, kv []any) { l.Debug(msg, kv...) },
	"info":  func(l Logger, msg string, kv []any) { l.Info(msg, kv...) },
	"warn":  func(l Logger, msg string, kv []any) { l.Warn(msg, kv...) },
	"error": func(l Logger, msg string, kv []any) { l.Error(msg, kv...) },
}

// Members builds log.trace/debug/info/warn/error. Diagnostics go to the Atmos logger only:
// never to the script's stdout, so they cannot become the step value.
func Members(step string, prefix func(*starlark.Thread) string, sink func() Logger) starlark.StringDict {
	defer perf.Track(nil, "log.Members")()

	members := make(starlark.StringDict, len(logLevels))
	for name, write := range logLevels {
		members[name] = starlark.NewBuiltin("log."+name, func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			message, keyvals, err := logCall(step, prefix(t), b.Name(), args, kwargs)
			if err != nil {
				return nil, err
			}
			logger := sink()
			if logger == nil {
				logger = log.Default()
			}
			write(logger, message, keyvals)
			return starlark.None, nil
		})
	}
	return members
}

// logCall validates a log.* call and returns the masked message and key/value pairs. The step name
// and, inside steps.parallel, the task path lead the fields unless the script sets the same key.
func logCall(step, prefix, name string, args starlark.Tuple, kwargs []starlark.Tuple) (string, []any, error) {
	if len(args) != 1 {
		return "", nil, convert.InvalidArgument("%s: takes exactly one positional argument (message), got %d", name, len(args))
	}
	text, ok := starlark.AsString(args[0])
	if !ok {
		return "", nil, convert.InvalidArgument("%s: message must be a string, got %s", name, args[0].Type())
	}
	keyvals := make([]any, 0, 2*(len(kwargs)+2))
	provided := make(map[string]bool, len(kwargs))
	fields := make([]any, 0, 2*len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if !logFieldName.MatchString(key) {
			return "", nil, convert.InvalidArgument("%s: field name %q is not an identifier", name, key)
		}
		provided[key] = true
		fields = append(fields, key, logValue(kv[1]))
	}
	if !provided[stepField] {
		keyvals = append(keyvals, stepField, iolib.MaskString(step))
	}
	if task := taskPath(prefix); task != "" && !provided[taskField] {
		keyvals = append(keyvals, taskField, iolib.MaskString(task))
	}
	return iolib.MaskString(text), append(keyvals, fields...), nil
}

// logValue converts a Starlark value to a native field value. Its rendered value
// is masked before reaching a log sink; unmasked scalars retain their native types.
func logValue(v starlark.Value) any {
	switch x := v.(type) {
	case starlark.NoneType:
		return nil
	case starlark.Bool:
		return maskedScalar(bool(x))
	case starlark.Int:
		if i, ok := x.Int64(); ok {
			return maskedScalar(i)
		}
	case starlark.Float:
		if !math.IsNaN(float64(x)) && !math.IsInf(float64(x), 0) {
			return maskedScalar(float64(x))
		}
	case starlark.String:
		return iolib.MaskString(string(x))
	}
	return iolib.MaskString(v.String())
}

// maskedScalar checks the same native rendering used when scalar secrets are
// registered, preserving structured types unless redaction changes the value.
func maskedScalar(value any) any {
	text := fmt.Sprint(value)
	if masked := iolib.MaskString(text); masked != text {
		return masked
	}
	return value
}

// taskPath turns a task output prefix such as "[outer] [inner[0]] " into "outer/inner[0]".
func taskPath(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	prefix = strings.TrimSuffix(strings.TrimPrefix(prefix, "["), "]")
	return strings.ReplaceAll(prefix, "] [", "/")
}
