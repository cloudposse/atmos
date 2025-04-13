package utils

import (
	"fmt"
	"os"
	"strings"
)

func ProcessTagEnv(
	input string,
) (string, error) {
	LogTrace(fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(input, AtmosYamlFuncEnv)
	if err != nil {
		return "", err
	}

	var envVarName string
	envVarDefault := ""
	var envVarExists bool

	parts, err := SplitStringByDelimiter(str, ' ')
	if err != nil {
		e := fmt.Errorf("error executing the YAML function: %s\n%v", input, err)
		return "", e

	}

	partsLen := len(parts)

	if partsLen == 2 {
		envVarName = strings.TrimSpace(parts[0])
		envVarDefault = strings.TrimSpace(parts[1])
	} else if partsLen == 1 {
		envVarName = strings.TrimSpace(parts[0])
	} else {
		err = fmt.Errorf("invalid number of arguments in the Atmos YAML function: %s. The function accepts 1 or 2 arguments", input)
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
		err := fmt.Errorf("invalid Atmos YAML function: %s", input)
		return "", err
	}

	return str, nil
}
