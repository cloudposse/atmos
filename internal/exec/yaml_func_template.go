package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(cliConfig schema.CliConfiguration, input string) any {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	part := strings.TrimPrefix(input, config.AtmosYamlFuncTemplate)
	part = strings.TrimSpace(part)

	if part == "" {
		err := errors.New(fmt.Sprintf("invalid Atmos YAML function: %s", input))
		u.LogErrorAndExit(cliConfig, err)
	}

	var decoded any
	if err := json.Unmarshal([]byte(part), &decoded); err != nil {
		return part
	}

	return decoded
}
