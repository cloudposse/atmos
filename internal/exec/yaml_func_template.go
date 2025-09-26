package exec

import (
	"encoding/json"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(
	input string,
) any {
	log.Debug("Executing", "Atmos YAML function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTemplate)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	var decoded any
	if err = json.Unmarshal([]byte(str), &decoded); err != nil {
		return str
	}

	return decoded
}
