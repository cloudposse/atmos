package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// This marker is emitted by the Terragrunt provider for optional TFLint variable
// files. Only the host knows the provisioned module directory at hook execution.
const tflintOptionalVarFilePrefix = "--atmos-optional-var-file="

const (
	tflintConfigSearchPrefix = "--atmos-tflint-config-search="
	tflintConfigName         = ".tflint.hcl"
	tflintMaxParents         = 100
)

func resolveTflintHookArguments(arguments []string, moduleDir string) ([]string, error) {
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
