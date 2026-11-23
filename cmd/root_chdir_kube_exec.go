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
			if flagTakesSeparateValue(arg) {
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

// flagTakesSeparateValue reports whether arg is a global flag that requires
// its value in the next argument: it has no inline value ("--name=value"), is
// not a boolean, and has no NoOptDefVal. It reads the global flag registry
// rather than RootCmd, which cannot be referenced here without an
// initialization cycle (RootCmd -> processChdirFlag -> checkChdirTarget).
func flagTakesSeparateValue(arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	isShorthand := !strings.HasPrefix(arg, "--") && len(name) == 1
	if !strings.HasPrefix(arg, "--") && !isShorthand {
		return false
	}
	for _, flag := range flags.GlobalFlagsRegistry().All() {
		if (isShorthand && flag.GetShorthand() == name) || (!isShorthand && flag.GetName() == name) {
			_, isBool := flag.(*flags.BoolFlag)
			return !isBool && flag.GetNoOptDefVal() == ""
		}
	}
	return false
}
