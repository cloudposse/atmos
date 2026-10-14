package hooks

import (
	"os"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// HookDepthEnvVar counts how many hooks deep the running command is. Every hook runs with the
	// value one higher than the command that triggered it, so a hook that runs the Atmos command
	// it is attached to (directly, or through `atmos.terraform` in a script) stops after a bounded
	// number of levels instead of recursing until the machine runs out of processes.
	HookDepthEnvVar = "ATMOS_HOOK_DEPTH"

	// The deepest level of nested hooks allowed.
	maxHookDepth = 8
)

// currentHookDepth reads the nesting level from the environment. A missing or malformed value
// counts as zero: the guard only works from values Atmos itself wrote.
func currentHookDepth() int {
	raw, ok := os.LookupEnv(HookDepthEnvVar)
	if !ok {
		return 0
	}
	depth, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || depth < 0 {
		return 0
	}
	return depth
}

// enterHook returns the nesting level a hook's own commands and steps run at. It fails when the
// command is already maxHookDepth hooks deep.
func enterHook(name string, event HookEvent) (int, error) {
	depth := currentHookDepth()
	if depth >= maxHookDepth {
		return 0, errUtils.Build(errUtils.ErrHookRecursionLimit).
			WithExplanationf("Hook %q on event %q is already %d hooks deep (limit %d). A hook that runs the Atmos command it is attached to starts itself again.", name, event, depth, maxHookDepth).
			WithHint("Scope the hook to the event that needs it, or guard the command it runs, for example by checking ATMOS_HOOK_DEPTH.").
			WithContext("hook", name).
			WithContext("event", string(event)).
			WithContext("depth", depth).
			Err()
	}
	return depth + 1, nil
}
