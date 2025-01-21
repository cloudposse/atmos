package exec

import (
	"encoding/json"
	"fmt"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(
	atmosConfig schema.AtmosConfiguration,
	input string,
	currentStack string,
) any {
	u.LogTrace(fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(input, config.AtmosYamlFuncTemplate)

	if err != nil {
		u.LogErrorAndExit(err)
	}

	var decoded any
	if err = json.Unmarshal([]byte(str), &decoded); err != nil {
		return str
	}

	return decoded
}
