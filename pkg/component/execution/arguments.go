package execution

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const varFileFlag = "-var-file="

// ArgumentsRequest describes one Terraform invocation for ApplyExtraArguments.
type ArgumentsRequest struct {
	// Command is the matching name from CommandName.
	Command string
	// Args is the full argument vector, starting with the command.
	Args []string
	// InsertAt is the index after the command and its subcommands where extra arguments go.
	InsertAt int
	// PlanFileArgument reports that the last argument is a saved plan. Terraform rejects -var
	// options beside a saved plan, so they are dropped.
	PlanFileArgument bool
	// WorkingDirectory resolves relative variable file paths. Terraform runs there.
	WorkingDirectory string
}

// ArgumentsResult is the rewritten invocation.
type ArgumentsResult struct {
	Args []string
	// Environment holds KEY=value entries sorted by key. Later blocks override earlier ones.
	Environment []string
}

// ApplyExtraArguments adds the arguments, variable files, and environment of every block that
// lists the command, in declaration order. The input arguments are never modified.
func ApplyExtraArguments(items []schema.ComponentExtraArguments, request *ArgumentsRequest) (ArgumentsResult, error) {
	defer perf.Track(nil, "execution.ApplyExtraArguments")()

	var inserted []string
	environment := map[string]string{}
	for i := range items {
		item := &items[i]
		if !slices.Contains(item.Commands, request.Command) {
			continue
		}
		maps.Copy(environment, item.Environment)
		arguments, err := blockArguments(item, request)
		if err != nil {
			return ArgumentsResult{}, err
		}
		inserted = append(inserted, arguments...)
	}
	position := min(max(request.InsertAt, 0), len(request.Args))
	args := slices.Concat(request.Args[:position], inserted, request.Args[position:])
	return ArgumentsResult{Args: args, Environment: sortedEnvironment(environment)}, nil
}

// CheckRequiredVarFiles verifies the required variable files of every block that lists the
// command, without building any arguments. It lets a caller reject a missing file before
// implicit init or workspace subprocesses, and their hooks, run for that command.
func CheckRequiredVarFiles(items []schema.ComponentExtraArguments, command, workingDirectory string) error {
	defer perf.Track(nil, "execution.CheckRequiredVarFiles")()

	for i := range items {
		if !slices.Contains(items[i].Commands, command) {
			continue
		}
		if _, err := requiredVarFiles(items[i].RequiredVarFiles, workingDirectory); err != nil {
			return err
		}
	}
	return nil
}

func blockArguments(item *schema.ComponentExtraArguments, request *ArgumentsRequest) ([]string, error) {
	arguments := slices.Clone(item.Arguments)
	if request.PlanFileArgument {
		return dropVariableArguments(arguments), nil
	}
	required, err := requiredVarFiles(item.RequiredVarFiles, request.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	for _, file := range required {
		arguments = append(arguments, varFileFlag+file)
	}
	for _, file := range keepLast(item.OptionalVarFiles) {
		if fileExists(resolve(request.WorkingDirectory, file)) {
			arguments = append(arguments, varFileFlag+file)
		}
	}
	return arguments, nil
}

// dropVariableArguments removes -var and -var-file options in either dash form (Terraform's
// flag parser accepts "--var-file" as well as "-var-file"). A bare option takes the next
// argument as its value ("-var", "name=value"), so that value goes with it; otherwise it
// would be left behind as an unexpected positional argument.
func dropVariableArguments(arguments []string) []string {
	var kept []string
	for i := 0; i < len(arguments); i++ {
		name, hasValue := variableOption(arguments[i])
		if name == "" {
			kept = append(kept, arguments[i])
			continue
		}
		if !hasValue {
			i++
		}
	}
	return kept
}

// variableOption reports whether argument is a -var or -var-file option, returning its name
// and whether the value is attached with "=". Other options, such as -variable-foo, are not
// variable options.
func variableOption(argument string) (name string, hasValue bool) {
	option, found := strings.CutPrefix(argument, "--")
	if !found {
		option, found = strings.CutPrefix(argument, "-")
	}
	if !found {
		return "", false
	}
	option, _, hasValue = strings.Cut(option, "=")
	if option != "var" && option != "var-file" {
		return "", false
	}
	return option, hasValue
}

func requiredVarFiles(files []string, directory string) ([]string, error) {
	files = keepLast(files)
	for _, file := range files {
		if !fileExists(resolve(directory, file)) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrComponentVarFileMissing, file)
		}
	}
	return files, nil
}

// keepLast removes duplicates and keeps the last occurrence of each item, as Terragrunt does.
func keepLast(items []string) []string {
	var result []string
	for i, item := range items {
		if !slices.Contains(items[i+1:], item) {
			result = append(result, item)
		}
	}
	return result
}

func resolve(directory, path string) string {
	if filepath.IsAbs(path) || directory == "" {
		return path
	}
	return filepath.Join(directory, path)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func sortedEnvironment(environment map[string]string) []string {
	var result []string
	for _, key := range slices.Sorted(maps.Keys(environment)) {
		result = append(result, key+"="+environment[key])
	}
	return result
}
