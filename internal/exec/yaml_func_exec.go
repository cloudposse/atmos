package exec

import (
	"encoding/json"
	"fmt"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagExec(
	atmosConfig schema.AtmosConfiguration,
	input string,
	currentStack string,
) any {
	u.LogTrace(atmosConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(input, config.AtmosYamlFuncExec)

	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	res, err := ExecuteShellAndReturnOutput(atmosConfig, str, input, ".", nil, false)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	var decoded any
	if err = json.Unmarshal([]byte(res), &decoded); err != nil {
		return res
	}

	return decoded
}
