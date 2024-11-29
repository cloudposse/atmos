package exec

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagExec(cliConfig schema.CliConfiguration, input string) any {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	part := strings.TrimPrefix(input, config.AtmosYamlFuncExec)
	part = strings.TrimSpace(part)

	res, err := ExecuteShellAndReturnOutput(cliConfig, part, input, ".", nil, false)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	var decoded any
	if err := json.Unmarshal([]byte(res), &decoded); err != nil {
		return res
	}
	return decoded
}
