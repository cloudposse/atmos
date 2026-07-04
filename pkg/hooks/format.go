package hooks

import (
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Inline output formats a generic `kind: command` hook can declare via the
// hook's `format:` field.
const (
	// FormatSARIF parses the command's output as a SARIF document, giving a
	// custom hook the same findings summary / CI annotations / SARIF upload as
	// the built-in scanner kinds.
	FormatSARIF = "sarif"
)

// formatHandlers maps a hook `format:` value to the ResultHandler that parses
// that format. It is the seam that lets a custom `kind: command` hook reuse a
// structured parser (e.g. SARIF) without pkg/hooks importing the parser's
// package — the parser package registers here from its init(), the same
// inversion the kind registry uses. Concretely it avoids the pkg/hooks ↔
// pkg/hooks/sarif import cycle.
var (
	formatHandlersMu sync.RWMutex
	formatHandlers   = map[string]ResultHandler{}
)

// RegisterFormatHandler registers the ResultHandler used to parse a hook's
// declared output `format:` when its kind has no built-in ResultHandler (i.e.
// the generic command kind). Returns an error on empty format, nil handler, or
// duplicate registration. Parser packages call this from init().
func RegisterFormatHandler(format string, handler ResultHandler) error {
	defer perf.Track(nil, "hooks.RegisterFormatHandler")()

	if format == "" || handler == nil {
		return errUtils.ErrNilParam
	}
	formatHandlersMu.Lock()
	defer formatHandlersMu.Unlock()
	if _, exists := formatHandlers[format]; exists {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanationf("hook format handler %q already registered", format).
			Err()
	}
	formatHandlers[format] = handler
	return nil
}

// formatHandlerFor returns the registered ResultHandler for a format, if any.
func formatHandlerFor(format string) (ResultHandler, bool) {
	formatHandlersMu.RLock()
	defer formatHandlersMu.RUnlock()
	h, ok := formatHandlers[format]
	return h, ok
}

// resolveResultHandler picks the ResultHandler for a hook invocation: the
// kind's own handler when present (built-in scanner kinds), otherwise a
// format handler matching the hook's `format:` (generic command kind +
// `format: sarif`). Returns nil when neither applies.
func resolveResultHandler(ctx *ExecContext) ResultHandler {
	if ctx == nil || ctx.Kind == nil {
		return nil
	}
	if ctx.Kind.ResultHandler != nil {
		return ctx.Kind.ResultHandler
	}
	if ctx.Hook != nil && ctx.Hook.Format != "" {
		if h, ok := formatHandlerFor(ctx.Hook.Format); ok {
			return h
		}
	}
	return nil
}
