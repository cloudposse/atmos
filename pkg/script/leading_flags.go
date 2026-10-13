package script

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// usageExitCode is the exit status for command lines Atmos cannot interpret, matching the
// convention of Atmos's own usage errors.
const usageExitCode = 2

// flagPrefix opens every flag word.
const flagPrefix = "-"

// DiagnoseLeadingFlags explains a command line that looks like a script invocation but whose
// leading flags Atmos cannot place. It returns nil when the arguments are not such an invocation,
// so the caller continues with normal command handling. Two mistakes are recognized:
//
//   - A flag with an optional value written with a space, as in `--profile dev ./tool.star`. The
//     flag cannot take the next word, so the word would be read as a command name.
//   - An unknown flag before the script path, as in `--bogus ./tool.star`.
//
// The takesValue callback is the same one SplitGlobalFlags uses. The optionalValue callback
// reports whether a known flag accepts an optional value (a non-boolean flag with a bare form).
func DiagnoseLeadingFlags(args []string, takesValue func(name string, short bool) (takes, found bool), optionalValue func(name string, short bool) bool) error {
	defer perf.Track(nil, "script.DiagnoseLeadingFlags")()

	globals, rest, ok := SplitGlobalFlags(args, takesValue)
	if ok {
		return ambiguousOptionalValue(globals, rest, optionalValue)
	}
	return unknownLeadingFlag(args, takesValue)
}

// ambiguousOptionalValue detects `--flag value ./script` where --flag takes only `--flag=value`.
func ambiguousOptionalValue(globals, rest []string, optionalValue func(string, bool) bool) error {
	if len(globals) == 0 || !wordBeforeScript(rest) {
		return nil
	}
	last := globals[len(globals)-1]
	name, short := strings.TrimLeft(last, flagPrefix), !strings.HasPrefix(last, "--")
	if !strings.HasPrefix(last, flagPrefix) || strings.Contains(last, "=") || (short && len(name) != 1) || !optionalValue(name, short) {
		return nil
	}
	return usageError(fmt.Errorf("%w: %q before a script path is ambiguous", errUtils.ErrScriptUsage, last+" "+rest[0]), fmt.Sprintf("Write %s=%s.", last, rest[0]))
}

// wordBeforeScript reports whether args start with one bare word that is not itself a script,
// followed by a script path.
func wordBeforeScript(args []string) bool {
	return len(args) >= 2 && !strings.HasPrefix(args[0], flagPrefix) && !MayBeFile(args[:1]) && MayBeFile(args[1:])
}

// unknownLeadingFlag names the first unrecognized flag when a script path follows it.
func unknownLeadingFlag(args []string, takesValue func(string, bool) (bool, bool)) error {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" || !strings.HasPrefix(arg, flagPrefix) || isStdinSelection(arg) {
			return nil
		}
		consumed, known := globalFlagWidth(args, index, takesValue)
		if known {
			index += consumed
			continue
		}
		if !flagIsUnknown(arg, takesValue) || !scriptFollows(args[index+1:]) {
			return nil
		}
		return usageError(fmt.Errorf("%w: unknown flag %q before a script path", errUtils.ErrScriptUsage, arg),
			"Atmos global flags go before the script path; flags after the script path belong to the script.")
	}
	return nil
}

// scriptFollows reports whether a script path comes next, allowing for one word that may be the
// preceding unknown flag's value.
func scriptFollows(args []string) bool {
	return MayBeFile(args) || wordBeforeScript(args)
}

// flagIsUnknown separates a flag Atmos does not define from a known flag that lacks its value.
func flagIsUnknown(arg string, takesValue func(string, bool) (bool, bool)) bool {
	if strings.HasPrefix(arg, "--") {
		name, _, _ := strings.Cut(arg[2:], "=")
		_, found := takesValue(name, false)
		return !found
	}
	_, found := takesValue(arg[1:2], true)
	return !found
}

func usageError(cause error, hint string) error {
	return errUtils.Build(cause).WithHint(hint).WithExitCode(usageExitCode).Err()
}
