package utils

import (
	"encoding/json"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

func ProcessTagExec(
	input string,
) (any, error) {
	defer perf.Track(nil, "utils.ProcessTagExec")()

	log.Debug("Executing Atmos YAML function", "function", input)
	str, err := getStringAfterTag(input, AtmosYamlFuncExec)
	if err != nil {
		return nil, err
	}

	res, err := ExecuteShellAndReturnOutput(str, input, ".", nil, false)
	if err != nil {
		return nil, err
	}

	var decoded any
	if err = json.Unmarshal([]byte(res), &decoded); err != nil {
		log.Debug("Unmarshalling error", "error", err)
		decoded = res
	}

	return decoded, nil
}
