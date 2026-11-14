package terraform

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// This marker is emitted by the Terragrunt provider for optional TFLint variable
// files. Only the host knows the provisioned module directory at hook execution.
const tflintOptionalVarFilePrefix = "--atmos-optional-var-file="

// This marker stands for one --var per input of the final prepared configuration. The provider cannot
// know those inputs: the host merges the stack's variables over them after preparation.
const tflintInputVariablesMarker = "--atmos-tflint-input-variables"

const (
	tflintConfigSearchPrefix = "--atmos-tflint-config-search="
	tflintConfigName         = ".tflint.hcl"
	tflintMaxParents         = 100
)

func resolveTflintHookArguments(arguments []string, moduleDir string, inputs map[string]any) ([]string, error) {
	arguments, err := resolveTflintInputVariables(arguments, inputs)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(arguments))
	for _, argument := range resolveOptionalTflintVarFiles(arguments, moduleDir) {
		unitDir, search := strings.CutPrefix(argument, tflintConfigSearchPrefix)
		if !search {
			result = append(result, argument)
			continue
		}
		config, err := findTflintConfig(unitDir)
		if err != nil {
			return nil, fmt.Errorf("%w: no .tflint.hcl found beside the unit or in a parent directory; pass --config to select one: %w", errUtils.ErrComponentHookFailed, err)
		}
		result = append(result, config)
	}
	return result, nil
}

// resolveTflintInputVariables replaces the input marker with a --var for every input, sorted so that
// repeated runs build the same command.
func resolveTflintInputVariables(arguments []string, inputs map[string]any) ([]string, error) {
	result := make([]string, 0, len(arguments)+len(inputs))
	for _, argument := range arguments {
		if argument != tflintInputVariablesMarker {
			result = append(result, argument)
			continue
		}
		for _, name := range slices.Sorted(maps.Keys(inputs)) {
			value, err := tflintVariableValue(inputs[name])
			if err != nil {
				return nil, fmt.Errorf("%w: input %q cannot be passed to tflint: %w", errUtils.ErrComponentHookFailed, name, err)
			}
			result = append(result, "--var="+name+"="+value)
		}
	}
	return result, nil
}

// tflintVariableValue renders a string as itself and any other value as JSON, escaping HCL templates.
func tflintVariableValue(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return strings.NewReplacer("${", "$${", "%{", "%%{").Replace(string(data)), nil
}

func findTflintConfig(unitDir string) (string, error) {
	directory := unitDir
	for range tflintMaxParents {
		candidate := filepath.Join(directory, tflintConfigName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	return "", os.ErrNotExist
}

func resolveOptionalTflintVarFiles(arguments []string, moduleDir string) []string {
	result := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		file, optional := strings.CutPrefix(argument, tflintOptionalVarFilePrefix)
		if !optional {
			result = append(result, argument)
			continue
		}
		candidate := file
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(moduleDir, candidate)
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			result = append(result, "--var-file="+file)
		}
	}
	return result
}
