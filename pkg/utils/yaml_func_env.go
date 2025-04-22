package utils

import (
	"fmt"
	"os"
	"strings"

	log "github.com/charmbracelet/log"
)

var ErrInvalidAtmosYAMLFunction = fmt.Errorf("invalid Atmos YAML function")

func ProcessTagEnv(
	input string,
) (string, error) {
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

	if partsLen == 2 {
		envVarName = strings.TrimSpace(parts[0])
		envVarDefault = strings.TrimSpace(parts[1])
	} else if partsLen == 1 {
		envVarName = strings.TrimSpace(parts[0])
	} else {
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
	str := strings.TrimPrefix(input, tag)
	str = strings.TrimSpace(str)

	if str == "" {
		err := fmt.Errorf("%w: %s", ErrInvalidAtmosYAMLFunction, input)
		return "", err
	}

	return str, nil
}
