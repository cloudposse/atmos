package reexec

import (
	"os"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// invocation is the command line a process recorded before rewriting os.Args for its own parsing.
var invocation = struct {
	sync.RWMutex
	args       []string
	scriptArgs int
}{}

// SetOriginalArgs records the argv the user actually typed when the process has rewritten os.Args
// for its own parsing (a standalone script run isolates the script path and its arguments).
// The scriptArgs count says how many trailing arguments (the script path and its arguments)
// belong to the script, so re-exec helpers leave them untouched. Re-exec call sites read Args so
// the new process receives the whole command line. It returns a function that restores the
// previous value.
func SetOriginalArgs(args []string, scriptArgs int) (restore func()) {
	defer perf.Track(nil, "reexec.SetOriginalArgs")()

	invocation.Lock()
	defer invocation.Unlock()
	previousArgs, previousCount := invocation.args, invocation.scriptArgs
	invocation.args, invocation.scriptArgs = append([]string(nil), args...), scriptArgs
	return func() {
		invocation.Lock()
		defer invocation.Unlock()
		invocation.args, invocation.scriptArgs = previousArgs, previousCount
	}
}

// Args returns the argv to forward to a re-executed Atmos: the original command line when
// SetOriginalArgs recorded one, otherwise os.Args.
func Args() []string {
	defer perf.Track(nil, "reexec.Args")()

	invocation.RLock()
	defer invocation.RUnlock()
	if invocation.args != nil {
		return append([]string(nil), invocation.args...)
	}
	return os.Args
}

// ScriptArgs returns how many trailing arguments of Args belong to a standalone script (its path
// and its arguments), or 0 when the command line does not run a script.
func ScriptArgs() int {
	defer perf.Track(nil, "reexec.ScriptArgs")()

	invocation.RLock()
	defer invocation.RUnlock()
	return invocation.scriptArgs
}

// StripBeforeScript applies strip to args except the last scriptArgs entries, so flags that belong
// to a standalone script (for example a script's own --chdir) are forwarded unchanged. A count of
// zero, or one that does not fit args, means there is no script and strip applies to all of args.
func StripBeforeScript(args []string, scriptArgs int, strip func([]string) []string) []string {
	defer perf.Track(nil, "reexec.StripBeforeScript")()

	if scriptArgs <= 0 || scriptArgs > len(args) {
		return strip(args)
	}
	split := len(args) - scriptArgs
	stripped := strip(args[:split])
	return append(stripped, args[split:]...)
}
