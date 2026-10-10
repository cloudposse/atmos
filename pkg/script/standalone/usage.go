package standalone

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// UsageExitCode is the exit status for command-line input a script's interface does not accept,
// matching the convention of Atmos's own usage errors.
const UsageExitCode = 2

// NewUsageError presents a failure to parse the user's command line as a usage error rather than a
// script failure. The invoked argument is the script as the user typed it, so the hint can be
// copied as-is.
func NewUsageError(cmd *cobra.Command, invoked string, cause error) error {
	defer perf.Track(nil, "standalone.NewUsageError")()

	base := cause
	if !errors.Is(cause, errUtils.ErrScriptUsage) {
		base = fmt.Errorf("%w: %w", errUtils.ErrScriptUsage, cause)
	}
	return errUtils.Build(base).
		WithExplanationf("Usage: `%s`", cmd.UseLine()).
		WithHintf("Run %s --help for usage.", invoked).
		WithExitCode(UsageExitCode).
		Err()
}

// CheckBoolPositional rejects a true/false word that directly follows a boolean flag. Boolean flags
// never consume the next word, so `--verbose false` would silently turn the flag on and pass
// "false" as a positional argument.
func CheckBoolPositional(set *pflag.FlagSet, argv []string) error {
	defer perf.Track(nil, "standalone.CheckBoolPositional")()

	for index := 0; index < len(argv); index++ {
		token := argv[index]
		if token == "--" {
			return nil
		}
		if !strings.HasPrefix(token, "-") || token == "-" {
			continue
		}
		name, takesNext := flagAt(set, token)
		if name == "" {
			if takesNext {
				index++
			}
			continue
		}
		if next := index + 1; next < len(argv) && isBoolWord(argv[next]) {
			return errUtils.Build(fmt.Errorf("%w: %q after --%s is read as a positional argument, not as the flag's value", errUtils.ErrScriptUsage, argv[next], name)).
				WithHintf("Use --%s=%s to set a boolean flag explicitly, or put -- before the argument to pass it literally.", name, strings.ToLower(argv[next])).
				Err()
		}
	}
	return nil
}

// flagAt classifies one flag token. It returns the name of a boolean flag that stands alone (so
// the following word is not its value), or whether a value-taking flag consumes the next word.
func flagAt(set *pflag.FlagSet, token string) (boolName string, takesNext bool) {
	if strings.HasPrefix(token, "--") {
		name := strings.TrimPrefix(token, "--")
		if strings.Contains(name, "=") {
			return "", false
		}
		return classify(set.Lookup(name))
	}
	cluster := token[1:]
	var last *pflag.Flag
	for position := 0; position < len(cluster); position++ {
		flag := set.ShorthandLookup(cluster[position : position+1])
		if flag == nil {
			return "", false
		}
		if flag.NoOptDefVal == "" {
			// A value-taking shorthand reads the rest of the token, or else the next word.
			return "", position == len(cluster)-1
		}
		last = flag
	}
	return classify(last)
}

func classify(flag *pflag.Flag) (boolName string, takesNext bool) {
	if flag == nil {
		return "", false
	}
	if flag.NoOptDefVal != "" {
		return flag.Name, false
	}
	return "", true
}

func isBoolWord(word string) bool {
	lower := strings.ToLower(word)
	return lower == "true" || lower == "false"
}

// AnnotateFlags appends what the help output would otherwise omit to each declared flag's
// description: whether it is required, its allowed values, and the environment variable bound to it.
func AnnotateFlags(set *pflag.FlagSet, declared []flags.Flag) {
	defer perf.Track(nil, "standalone.AnnotateFlags")()

	for _, definition := range declared {
		flag := set.Lookup(definition.GetName())
		if flag == nil {
			continue
		}
		var notes []string
		if definition.IsRequired() {
			notes = append(notes, "(required)")
		}
		if choices, ok := definition.(interface{ GetValidValues() []string }); ok && len(choices.GetValidValues()) > 0 {
			notes = append(notes, "(one of: "+strings.Join(choices.GetValidValues(), ", ")+")")
		}
		if env := definition.GetEnvVars(); len(env) > 0 {
			notes = append(notes, "[env: "+strings.Join(env, ", ")+"]")
		}
		flag.Usage = strings.TrimSpace(flag.Usage + " " + strings.Join(notes, " "))
	}
}
