package exec

import (
	u "atmos/internal/utils"
	"os"
	"os/exec"
	"strings"
)

var (
	commonFlags = []string{
		"--stack",
		"-s",
		"--dry-run",
		"--kubeconfig-path",
		"--terraform-dir",
		"--helmfile-dir",
		"--config-dir",
	}

	// First arg is a terraform subcommand
	// Second arg is component
	commonArgsIndexes = []int{0, 1}
)

// removeCommonArgsAndFlags removes common args and flags from the provided list of arguments/flags
func removeCommonArgsAndFlags(argsAndFlags []string) []string {
	result := []string{}
	indexesToRemove := []int{}

	for i, arg := range argsAndFlags {
		for _, f := range commonFlags {
			if u.SliceContainsInt(commonArgsIndexes, i) {
				indexesToRemove = append(indexesToRemove, i)
			} else if arg == f {
				indexesToRemove = append(indexesToRemove, i)
				indexesToRemove = append(indexesToRemove, i+1)
			} else if strings.HasPrefix(arg, f+"=") {
				indexesToRemove = append(indexesToRemove, i)
			}
		}
	}

	for i, arg := range argsAndFlags {
		if !u.SliceContainsInt(indexesToRemove, i) {
			result = append(result, arg)
		}
	}

	return result
}

// https://medium.com/rungo/executing-shell-commands-script-files-and-executables-in-go-894814f1c0f7
func execCommand(command string, args []string, dir string) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	return cmd.Run()
}
