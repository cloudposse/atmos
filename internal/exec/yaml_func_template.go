package exec

import (
	"encoding/json"
	log "github.com/charmbracelet/log"

	atmoserr "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(
	atmosConfig schema.AtmosConfiguration,
	input string,
	currentStack string,
) any {
	log.Debug("Executing", "Atmos YAML function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTemplate)
	if err != nil {
		atmoserr.CheckErrorPrintAndExit(err, "", "")
	}

	var decoded any
	if err = json.Unmarshal([]byte(str), &decoded); err != nil {
		return str
	}

	return decoded
}
