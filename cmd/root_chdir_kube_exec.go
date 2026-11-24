package cmd

import (
	"fmt"
	"os"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/reexec"
	"github.com/cloudposse/atmos/pkg/ui"
)

// The kubeExecTokenCommands are the commands that the kubeconfig exec entries
// written by the aws/eks, azure/aks and gcp/gke integrations run.
// The kubectl client invokes them with ATMOS_CHDIR set to the checkout that wrote the entry.
var kubeExecTokenCommands = [][]string{
	{"aws", "eks", "token"},
	{"azure", "aks", "token"},
	{"gcp", "gke", "token"},
}

// warnMissingKubeExecChdir tells the user, on stderr, that the ATMOS_CHDIR
// directory of a kubeconfig exec entry is gone. Overridable in tests.
var warnMissingKubeExecChdir = func(dir string) {
	ui.Warning(missingKubeExecChdirMessage(dir))
}

// missingKubeExecChdirMessage is the warning shown when the ATMOS_CHDIR
// directory of a kubeconfig exec entry does not exist.
func missingKubeExecChdirMessage(dir string) string {
	return fmt.Sprintf("ATMOS_CHDIR directory %s does not exist, so Atmos runs from the current directory. "+
		"Re-run `atmos auth login` (or the kubeconfig integration) from the repository to update the kubeconfig.", dir)
}

// checkChdirTarget verifies that absPath is an existing directory. It returns
// skip=true, and no error, only when the path came from ATMOS_CHDIR (fromEnv),
// does not exist, and this process is a kubeconfig exec token command: the
// directory of a removed checkout must not break kubectl authentication, so
// the command runs from the current directory instead. An explicit --chdir and
// every other command still fail as before.
func checkChdirTarget(absPath string, fromEnv bool) (skip bool, err error) {
	stat, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			if fromEnv && len(os.Args) > 1 && isKubeExecTokenInvocation(os.Args[1:]) {
				warnMissingKubeExecChdir(absPath)
				// Keep the stale directory away from later chdir handling and child processes.
				_ = os.Unsetenv(reexec.AtmosChdirEnvVar)
				return true, nil
			}
			return false, fmt.Errorf("%w: directory does not exist: %s", errUtils.ErrWorkdirNotExist, absPath)
		}
		return false, fmt.Errorf("%w: failed to access directory: %s", errUtils.ErrStatFile, absPath)
	}

	if !stat.IsDir() {
		return false, fmt.Errorf("%w: not a directory: %s", errUtils.ErrWorkdirNotExist, absPath)
	}

	return false, nil
}

// isKubeExecTokenInvocation reports whether args (os.Args without the program
// name) start with one of kubeExecTokenCommands, ignoring flags before and
// between the command words. A global flag registered on the root command that
// takes a value (no NoOptDefVal) in a separate argument, such as
// "--logs-level Debug", skips that value too. Parsing stops at "--". A flag
// that is not registered is skipped alone, so an invocation that relies on
// such a flag's separate value is treated as another command, which keeps the
// strict behavior.
func isKubeExecTokenInvocation(args []string) bool {
	words := make([]string, 0, len(kubeExecTokenCommands[0]))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if len(arg) > 0 && arg[0] == '-' {
			if flagConsumesNext(arg, args, i+1, words) {
				i++ // Skip the flag's value.
			}
			continue
		}
		words = append(words, arg)
		if len(words) == cap(words) {
			break
		}
	}
	return slices.ContainsFunc(kubeExecTokenCommands, func(command []string) bool {
		return slices.Equal(command, words)
	})
}

// flagConsumesNext reports whether the argument at args[next] is the separate
// value of the flag arg. A flag that requires a value always consumes it. A
// NoOptDefVal flag such as --identity consumes it too, as the optional-value
// normalization does at startup, unless it is the next word of a token
// command, so bare "--identity aws eks token" keeps "aws" as a command word.
func flagConsumesNext(arg string, args []string, next int, words []string) bool {
	if next >= len(args) {
		return false
	}
	kind := flagValueKind(arg)
	switch kind {
	case flagValueRequired:
		return true
	case flagValueOptional:
		value := args[next]
		if len(value) == 0 || value[0] == '-' {
			return false
		}
		return !isNextTokenCommandWord(words, value)
	default:
		return false
	}
}

// isNextTokenCommandWord reports whether word would extend words toward one of
// kubeExecTokenCommands.
func isNextTokenCommandWord(words []string, word string) bool {
	return slices.ContainsFunc(kubeExecTokenCommands, func(command []string) bool {
		return len(command) > len(words) && slices.Equal(command[:len(words)], words) && command[len(words)] == word
	})
}

// Kinds of value a global flag takes in the next argument.
const (
	flagValueNone = iota
	flagValueRequired
	flagValueOptional
)

// flagValueKind classifies arg: it has no inline value ("--name=value"), and
// is a non-boolean global flag that either requires a value (no NoOptDefVal)
// or accepts an optional one (NoOptDefVal consuming the next argument). It reads the global flag registry
// rather than RootCmd, which cannot be referenced here without an
// initialization cycle (RootCmd -> processChdirFlag -> checkChdirTarget).
func flagValueKind(arg string) int {
	if strings.Contains(arg, "=") {
		return flagValueNone
	}
	name := strings.TrimLeft(arg, "-")
	isShorthand := !strings.HasPrefix(arg, "--") && len(name) == 1
	if !strings.HasPrefix(arg, "--") && !isShorthand {
		return flagValueNone
	}
	for _, flag := range flags.GlobalFlagsRegistry().All() {
		if globalFlagMatches(flag, name, isShorthand) {
			return classifyFlagValue(flag)
		}
	}
	return flagValueNone
}

// classifyFlagValue returns the kind of separate value flag takes.
func classifyFlagValue(flag flags.Flag) int {
	if _, isBool := flag.(*flags.BoolFlag); isBool {
		return flagValueNone
	}
	if flag.GetNoOptDefVal() == "" {
		return flagValueRequired
	}
	if flag.GetNoOptDefValConsumesNextArg() {
		return flagValueOptional
	}
	return flagValueNone
}

// globalFlagMatches reports whether flag is the one called name, compared by
// shorthand when isShorthand is set and by long name otherwise.
func globalFlagMatches(flag flags.Flag, name string, isShorthand bool) bool {
	if isShorthand {
		return flag.GetShorthand() == name
	}
	return flag.GetName() == name
}
