package utils

import (
	"fmt"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

var ErrInvalidAtmosYAMLFunction = fmt.Errorf("invalid Atmos YAML function")

func ProcessTagEnv(
	input string,
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
