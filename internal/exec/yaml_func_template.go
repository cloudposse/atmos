package exec

import (
	"encoding/json"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(
	input string,
) any {
	defer perf.Track(nil, "exec.processTagTemplate")()

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
