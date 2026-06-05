package flags

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// globalDefaultArgsKey is the atmos.yaml key holding default args applied to every command.
const globalDefaultArgsKey = "args"

// InjectDefaultArgs returns args with config-sourced default arguments inserted for the
// command that args resolve to.
//
// Default args come from the global top-level `args:` list and the path-derived
// `<command>.args` lists in rawConfig (the fully merged atmos.yaml, profiles applied),
// with the most-specific command path applied last. For example, running
// `atmos describe component` pulls from `args`, then `describe.args`, then
// `describe.component.args`.
//
// A default arg is skipped when its flag is already present on the command line or when its
// environment variable is set, preserving precedence: CLI > ENV > config default. Surviving
// defaults are spliced in right after the resolved command path (before the user's args), so
// the command parses them as if typed, and an explicit CLI flag still wins.
func InjectDefaultArgs(rootCmd *cobra.Command, rawConfig map[string]any, args []string) []string {
	defer perf.Track(nil, "flags.InjectDefaultArgs")()

	if rootCmd == nil || len(rawConfig) == 0 || len(args) == 0 {
		return args
	}

	targetCmd, rest, err := rootCmd.Find(args)
	if err != nil || targetCmd == nil || targetCmd == rootCmd {
		return args
	}

	cmdPath := commandPathNames(targetCmd, rootCmd)
	if skipDefaultArgsForCommand(cmdPath, rest) {
		return args
	}

	inject := filterDefaultArgs(collectDefaultArgs(rawConfig, cmdPath), rest)
	return spliceDefaultArgs(args, rest, inject)
}

// spliceDefaultArgs inserts inject right after the resolved command path (which is args with
// the trailing rest removed) and before the user's args. Returns args unchanged when there is
// nothing to inject.
func spliceDefaultArgs(args, rest, inject []string) []string {
	pathLen := len(args) - len(rest)
	if len(inject) == 0 || pathLen < 0 {
		return args
	}

	out := make([]string, 0, len(args)+len(inject))
	out = append(out, args[:pathLen]...) // resolved command path tokens.
	out = append(out, inject...)         // injected default args.
	out = append(out, rest...)           // the user's args (flags/positionals/pass-through).
	return out
}

// commandPathNames returns the canonical command names from the first command below the
// root down to target (e.g. ["describe", "component"]).
func commandPathNames(target, root *cobra.Command) []string {
	var names []string
	for c := target; c != nil && c != root; c = c.Parent() {
		names = append([]string{c.Name()}, names...)
	}
	return names
}

// skipDefaultArgsForCommand reports whether default-arg injection should be skipped for this
// command. Help and shell-completion paths must not be mutated.
func skipDefaultArgsForCommand(cmdPath, rest []string) bool {
	if len(cmdPath) == 0 {
		return true
	}
	switch cmdPath[0] {
	case "help", "completion":
		return true
	}
	for _, c := range cmdPath {
		if strings.HasPrefix(c, "__") { // cobra completion commands (__complete, __completeNoDesc).
			return true
		}
	}
	for _, a := range rest {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// collectDefaultArgs gathers default-arg tokens for the command path, least-specific first
// (global `args`, then each successive path segment's `args`), so the most-specific wins on
// cobra's last-flag-wins parsing.
func collectDefaultArgs(rawConfig map[string]any, cmdPath []string) []string {
	var out []string
	out = append(out, toStringList(rawConfig[globalDefaultArgsKey])...)

	node := rawConfig
	for _, name := range cmdPath {
		sub, ok := node[name].(map[string]any)
		if !ok {
			break
		}
		out = append(out, toStringList(sub[globalDefaultArgsKey])...)
		node = sub
	}
	return out
}

// filterDefaultArgs drops default args whose flag is already set on the command line or via
// its environment variable.
func filterDefaultArgs(defaults, userArgs []string) []string {
	var out []string
	for _, arg := range defaults {
		name := flagNameFromArg(arg)
		if name != "" && (flagPresentInArgs(name, userArgs) || flagEnvIsSet(name)) {
			continue
		}
		out = append(out, arg)
	}
	return out
}

// flagNameFromArg extracts the flag name from a default-arg token (e.g. "--skip=x" → "skip",
// "--process-functions" → "process-functions"). Returns "" for non-flag (positional) tokens.
func flagNameFromArg(arg string) string {
	if !strings.HasPrefix(arg, "-") {
		return ""
	}
	name := strings.TrimLeft(arg, "-")
	if idx := strings.IndexByte(name, '='); idx >= 0 {
		name = name[:idx]
	}
	return name
}

// flagPresentInArgs reports whether the named flag (long or its `--name=` form) already
// appears in the user's args, up to a `--` pass-through separator.
func flagPresentInArgs(name string, userArgs []string) bool {
	long := "--" + name
	for _, a := range userArgs {
		if a == "--" {
			return false
		}
		if a == long || strings.HasPrefix(a, long+"=") {
			return true
		}
	}
	return false
}

// flagEnvIsSet reports whether an Atmos environment variable for the flag is set. Atmos flags
// follow the ATMOS_<UPPER_SNAKE> convention; global flags may declare additional env vars,
// which are also honored.
func flagEnvIsSet(name string) bool {
	if _, ok := os.LookupEnv(conventionalEnvVar(name)); ok {
		return true
	}
	if flag := GlobalFlagsRegistry().Get(name); flag != nil {
		for _, env := range flag.GetEnvVars() {
			if _, ok := os.LookupEnv(env); ok {
				return true
			}
		}
	}
	return false
}

// conventionalEnvVar returns the conventional Atmos env var name for a flag (e.g.
// "process-functions" → "ATMOS_PROCESS_FUNCTIONS").
func conventionalEnvVar(name string) string {
	return "ATMOS_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// toStringList converts a config value (a YAML list, a single string, or nil) to []string.
func toStringList(v any) []string {
	switch vv := v.(type) {
	case nil:
		return nil
	case []string:
		return vv
	case []any:
		out := make([]string, 0, len(vv))
		for _, e := range vv {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{vv}
	default:
		return nil
	}
}
