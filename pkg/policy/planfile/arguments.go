package planfile

import (
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

type artifactSelection struct {
	path   string
	index  int
	joined bool
}

func (a artifactSelection) replace(args []string, path string) []string {
	result := append([]string{}, args...)
	if a.joined {
		path = "-out=" + path
	}
	result[a.index] = path
	return result
}

func selectArtifact(invocation *Invocation) (artifactSelection, error) {
	if err := validateArgumentEnvironment(invocation.Env); err != nil {
		return artifactSelection{}, err
	}
	if len(invocation.Args) < 2 {
		return artifactSelection{}, fmt.Errorf("%w: saved plan arguments required", errUtils.ErrPolicyPlan)
	}
	var selection artifactSelection
	var err error
	switch invocation.Args[0] {
	case "plan":
		selection, err = planSelection(invocation.Args)
	case "apply":
		selection, err = applySelection(invocation.Args)
	default:
		err = fmt.Errorf("%w: only plan and saved-plan apply are supported", errUtils.ErrPolicyPlan)
	}
	if err != nil {
		return artifactSelection{}, err
	}
	if !filepath.IsAbs(selection.path) {
		selection.path = filepath.Join(invocation.Dir, selection.path)
	}
	return selection, nil
}

func validateArgumentEnvironment(env []string) error {
	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		if value == "" {
			continue
		}
		switch name {
		case "TF_CLI_ARGS", "TF_CLI_ARGS_plan", "TF_CLI_ARGS_apply", "TF_CLI_ARGS_show":
			return fmt.Errorf("%w: set explicit command flags instead of %s", errUtils.ErrPolicyPlan, name)
		}
	}
	return nil
}

func planSelection(args []string) (artifactSelection, error) {
	selection := artifactSelection{index: -1}
	for index := 1; index < len(args); index++ {
		arg := args[index]
		if arg != "-out" && !strings.HasPrefix(arg, "-out=") {
			continue
		}
		if selection.index != -1 {
			return selection, fmt.Errorf("%w: multiple -out arguments", errUtils.ErrPolicyPlan)
		}
		selection.joined = arg != "-out"
		if selection.joined {
			selection.path = strings.TrimPrefix(arg, "-out=")
		} else {
			index++
			if index >= len(args) {
				return selection, fmt.Errorf("%w: missing -out value", errUtils.ErrPolicyPlan)
			}
			selection.path = args[index]
		}
		selection.index = index
	}
	if selection.index == -1 || selection.path == "" || strings.HasPrefix(selection.path, "-") {
		return selection, fmt.Errorf("%w: plan policies require a saved plan; enable planfile generation", errUtils.ErrPolicyPlan)
	}
	return selection, nil
}

func applySelection(args []string) (artifactSelection, error) {
	last := len(args) - 1
	selection := artifactSelection{index: last, path: args[last]}
	if selection.path == "" || strings.HasPrefix(selection.path, "-") {
		return selection, fmt.Errorf("%w: apply requires a saved plan", errUtils.ErrPolicyPlan)
	}
	// Fail closed on positional arguments, planning flags and future unknown flags.
	// Only flags that do not choose or regenerate a plan are accepted here.
	for index := 1; index < last; index++ {
		name, _, joined := strings.Cut(args[index], "=")
		switch name {
		case "-auto-approve", "-input", "-lock", "-no-color", "-compact-warnings", "-json":
		case "-lock-timeout", "-parallelism":
			if !joined {
				index++
				if index >= last || strings.HasPrefix(args[index], "-") {
					return selection, fmt.Errorf("%w: missing %s value", errUtils.ErrPolicyPlan, name)
				}
			}
		default:
			return selection, fmt.Errorf("%w: unsupported saved-plan apply argument %q", errUtils.ErrPolicyPlan, name)
		}
	}
	return selection, nil
}
