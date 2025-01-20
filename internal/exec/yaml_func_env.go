package exec

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagEnv(
	atmosConfig schema.AtmosConfiguration,
	input string,
	currentStack string,
) any {
	u.LogTrace(atmosConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(input, config.AtmosYamlFuncEnv)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	var envVarName string
	envVarDefault := ""
	var envVarExists bool

	parts, err := u.SplitStringByDelimiter(str, ' ')
	if err != nil {
		e := fmt.Errorf("error executing the YAML function: %s\n%v", input, err)
		u.LogErrorAndExit(atmosConfig, e)
	}

	partsLen := len(parts)

	if partsLen == 2 {
		envVarName = strings.TrimSpace(parts[0])
		envVarDefault = strings.TrimSpace(parts[1])
	} else if partsLen == 1 {
		envVarName = strings.TrimSpace(parts[0])
	} else {
		err = fmt.Errorf("invalid number of arguments in the Atmos YAML function: %s. The function accepts 1 or 2 arguments", input)
		u.LogErrorAndExit(atmosConfig, err)
	}

	res, envVarExists := os.LookupEnv(envVarName)

	if envVarExists {
		return res
	}

	if envVarDefault != "" {
		return envVarDefault
	}

	return nil
}
