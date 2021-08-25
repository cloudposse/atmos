package exec

import (
	u "atmos/internal/utils"
	"errors"
	"fmt"
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
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	return cmd.Run()
}

// Check stack schema and return component info
func checkStackConfig(
	stack string,
	stacksMap map[string]interface{},
	component string,
) (map[interface{}]interface{}, string, string, error) {

	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var terraformSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var baseComponent string
	var command string
	var ok bool

	if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("Stack '%s' does not exist", stack))
	}
	if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("'components' section is missing in stack '%s'", stack))
	}
	if terraformSection, ok = componentsSection["terraform"].(map[string]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("'components/terraform' section is missing in stack '%s'", stack))
	}
	if componentSection, ok = terraformSection[component].(map[string]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("Invalid or missing configuration for component '%s' in stack '%s'", component, stack))
	}
	if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("Missing 'vars' section for component '%s' in stack '%s'", component, stack))
	}
	if baseComponent, ok = componentSection["component"].(string); !ok {
		baseComponent = ""
	}
	if command, ok = componentSection["command"].(string); !ok {
		command = ""
	}

	return componentVarsSection, baseComponent, command, nil
}
