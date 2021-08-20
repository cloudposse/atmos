package exec

import (
	u "atmos/internal/utils"
	"os"
	"os/exec"
	"strings"
)

var (
	commonFlags = []string{"--stack", "-s"}
)

// removeCommonFlags removes common CLI flags from the provided list of arguments/flags
func removeCommonFlags(args []string) []string {
	result := []string{}
	indexesToRemove := []int{}

	for i, arg := range args {
		for _, f := range commonFlags {
			if arg == f {
				indexesToRemove = append(indexesToRemove, i)
				indexesToRemove = append(indexesToRemove, i+1)
			} else if strings.HasPrefix(arg, f+"=") {
				indexesToRemove = append(indexesToRemove, i)
			}
		}
	}

	for i, arg := range args {
		if !u.SliceContainsInt(indexesToRemove, i) {
			result = append(result, arg)
		}
	}

	return result
}

// https://medium.com/rungo/executing-shell-commands-script-files-and-executables-in-go-894814f1c0f7
func execCommand(command string, args []string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	return cmd.Run()
}
