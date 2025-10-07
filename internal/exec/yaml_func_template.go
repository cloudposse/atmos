package exec

import (
	"encoding/json"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(
	input string,
) (any, error) {
	defer perf.Track(nil, "exec.processTagTemplate")()

	log.Debug("Executing", "Atmos YAML function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTemplate)
	if err != nil {
		return nil, err
	}

	var decoded any
	if err = json.Unmarshal([]byte(str), &decoded); err != nil {
		return str, nil
	}

	return decoded, nil
}
