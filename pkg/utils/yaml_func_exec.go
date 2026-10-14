package utils

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/admission"
)

func ProcessTagExec(
	input string,
) (any, error) {
	defer perf.Track(nil, "utils.ProcessTagExec")()
	if err := admission.Function(context.Background(), "exec"); err != nil {
		return nil, err
	}

	log.Debug("Executing Atmos YAML function", "function", input)
	str, err := getStringAfterTag(input, AtmosYamlFuncExec)
	if err != nil {
		return nil, err
	}

	res, err := ExecuteShellAndReturnOutput(str, input, ".", os.Environ(), false)
	if err != nil {
		return nil, err
	}

	// Strip trailing newlines to match shell command substitution behavior.
	// In shell, $(cmd) strips trailing newlines from the output.
	res = strings.TrimRight(res, "\n")

	var decoded any
	if err = json.Unmarshal([]byte(res), &decoded); err != nil {
		log.Debug("Unmarshalling error", "error", err)
		decoded = res
	}

	return decoded, nil
}
