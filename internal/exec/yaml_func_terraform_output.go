package exec

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTerraformOutput(cliConfig schema.CliConfiguration, input string) any {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	part := strings.TrimPrefix(input, config.AtmosYamlFuncTerraformOutput)
	part = strings.TrimSpace(part)
	return part
}
