package starlark

import (
	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/perf"
	logmodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/log"
)

// Logger is the diagnostics sink behind the log module.
type Logger = logmodule.Logger

// WithLogger supplies the logger behind log.trace/debug/info/warn/error.
// A nil logger resolves Atmos's current default logger at each call.
func WithLogger(logger Logger) Option {
	defer perf.Track(nil, "starlark.WithLogger")()
	return func(e *Engine) { e.logger = logger }
}

func (s *session) logMembers() starlark.StringDict {
	return logmodule.Members(s.spec.Name, threadPrefix, func() Logger { return s.engine.logger })
}
