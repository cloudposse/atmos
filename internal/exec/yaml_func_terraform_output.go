package exec

import (
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTerraformOutput(
	atmosConfig schema.AtmosConfiguration,
	input string,
	currentStack string,
) any {
	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTerraformOutput)
	if err != nil {
		log.Fatal(err)
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
		log.Debug("Calling Atmos YAML function with component and output parameters; using current stack",
    "function", input,
    "stack", currentStack,
)
	} else {
		err := fmt.Errorf("invalid number of arguments in the Atmos YAML function: %s", input)
		log.Fatal(err)
	}

	value := GetTerraformOutput(&atmosConfig, stack, component, output, false)
	return value
}
