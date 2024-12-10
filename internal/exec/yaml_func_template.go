package exec

import (
	"encoding/json"
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(cliConfig schema.CliConfiguration, input string) any {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(cliConfig, input, u.AtmosYamlFuncTemplate)

	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	var decoded any
	if err = json.Unmarshal([]byte(str), &decoded); err != nil {
		return str
	}

	return decoded
}
