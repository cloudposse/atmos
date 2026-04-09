package utils

import (
	"errors"
	"fmt"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

var ErrInvalidAtmosYAMLFunction = errors.New("invalid Atmos YAML function")

// EnvVarContext provides context for environment variable lookup.
// It allows !env to check stack manifest env sections before falling back to OS environment.
type EnvVarContext interface {
	// GetComponentEnvSection returns the component's env section map, or nil if not available.
	GetComponentEnvSection() map[string]any
}

func ProcessTagEnv(
	input string,
	envContext EnvVarContext,
) (string, error) {
	defer perf.Track(nil, "utils.ProcessTagEnv")()

	log.Debug("Executing Atmos YAML function", "input", input)

	str, err := getStringAfterTag(input, AtmosYamlFuncEnv)
	if err != nil {
		return "", err
	}

	var envVarName string
	envVarDefault := ""
	var envVarExists bool

	parts, err := SplitStringByDelimiter(str, ' ')
	if err != nil {
		e := fmt.Errorf("%w: %s", ErrInvalidAtmosYAMLFunction, input)
		return "", e
	}

	partsLen := len(parts)

	switch partsLen {
	case 2:
		envVarName = strings.TrimSpace(parts[0])
		envVarDefault = strings.TrimSpace(parts[1])
	case 1:
		envVarName = strings.TrimSpace(parts[0])
	default:
		err = fmt.Errorf("%w: invalid number of arguments. The function accepts 1 or 2 arguments: %s", ErrInvalidAtmosYAMLFunction, input)
		return "", err
	}

	// First, check the component's env section from stack manifests.
	if envContext != nil {
		if envSection := envContext.GetComponentEnvSection(); envSection != nil {
			if val, exists := envSection[envVarName]; exists {
				// Convert the value to string.
				return fmt.Sprintf("%v", val), nil
			}
		}
	}

	// Fall back to OS environment variables.
	res, envVarExists := os.LookupEnv(envVarName)

	if envVarExists {
		return res, nil
	}

	if envVarDefault != "" {
		return envVarDefault, nil
	}

	return "", nil
}

func getStringAfterTag(input string, tag string) (string, error) {
	defer perf.Track(nil, "utils.getStringAfterTag")()

	str := strings.TrimPrefix(input, tag)
	str = strings.TrimSpace(str)

	if str == "" {
		err := fmt.Errorf("%w: %s", ErrInvalidAtmosYAMLFunction, input)
		return "", err
	}

	return str, nil
}
