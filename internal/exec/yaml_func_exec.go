package exec

import (
	"encoding/json"
	"fmt"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagExec(
	cliConfig schema.CliConfiguration,
	input string,
	currentStack string,
) any {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(input, config.AtmosYamlFuncExec)

	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	res, err := ExecuteShellAndReturnOutput(cliConfig, str, input, ".", nil, false)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	var decoded any
	if err = json.Unmarshal([]byte(res), &decoded); err != nil {
		return res
	}

	return decoded
}
