package exec

import (
	"fmt"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	terraformOutputFuncSyncMap = sync.Map{}
)

func processTagTerraformOutput(
	atmosConfig schema.AtmosConfiguration,
	input string,
	currentStack string,
) any {
	u.LogTrace(atmosConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(input, config.AtmosYamlFuncTerraformOutput)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	var component string
	var stack string
	var output string

	// Split the string into slices based on any whitespace (one or more spaces, tabs, or newlines),
	// while also ignoring leading and trailing whitespace
	parts := strings.Fields(str)
	partsLen := len(parts)

	if partsLen == 3 {
		component = strings.TrimSpace(parts[0])
		stack = strings.TrimSpace(parts[1])
		output = strings.TrimSpace(parts[2])
	} else if partsLen == 2 {
		component = strings.TrimSpace(parts[0])
		stack = currentStack
		output = strings.TrimSpace(parts[1])
		u.LogTrace(atmosConfig, fmt.Sprintf("Atmos YAML function `%s` is called with two parameters 'component' and 'output'. "+
			"Using the current stack '%s' as the 'stack' parameter", input, currentStack))
	} else {
		err := fmt.Errorf("invalid number of arguments in the Atmos YAML function: %s", input)
		u.LogErrorAndExit(atmosConfig, err)
	}

	value := GetTerraformOutput(&atmosConfig, stack, component, output, false)
	return value
}
