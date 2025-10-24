package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// processTagTerraformState processes `!terraform.state` YAML tag.
func processTagTerraformState(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	return processTagTerraformStateWithContext(atmosConfig, input, currentStack, nil)
}

// processTagTerraformStateWithContext processes `!terraform.state` YAML tag with cycle detection.
func processTagTerraformStateWithContext(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	resolutionCtx *ResolutionContext,
) any {
	defer perf.Track(atmosConfig, "exec.processTagTerraformStateWithContext")()

	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTerraformState)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	var component string
	var stack string
	var output string

	// Split the string into slices based on any whitespace (one or more spaces, tabs, or newlines),
	// while also ignoring leading and trailing whitespace.
	// SplitStringByDelimiter splits a string by the delimiter, not splitting inside quotes.
	parts, err := u.SplitStringByDelimiter(str, ' ')
	errUtils.CheckErrorPrintAndExit(err, "", "")

	partsLen := len(parts)

	switch partsLen {
	case 3:
		component = strings.TrimSpace(parts[0])
		stack = strings.TrimSpace(parts[1])
		output = strings.TrimSpace(parts[2])
	case 2:
		component = strings.TrimSpace(parts[0])
		stack = currentStack
		output = strings.TrimSpace(parts[1])
		log.Debug("Executing Atmos YAML function with component and output parameters; using current stack",
			"function", input,
			"stack", currentStack,
		)
	default:
		er := fmt.Errorf("%w %s", errUtils.ErrYamlFuncInvalidArguments, input)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// Check for circular dependencies if resolution context is provided.
	if resolutionCtx != nil {
		node := DependencyNode{
			Component:    component,
			Stack:        stack,
			FunctionType: "terraform.state",
			FunctionCall: input,
		}

		// Check and record this dependency.
		if err := resolutionCtx.Push(atmosConfig, node); err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}

		// Defer pop to ensure we clean up even if there's an error.
		defer resolutionCtx.Pop(atmosConfig)
	}

	value, err := GetTerraformState(atmosConfig, input, stack, component, output, false, nil)
	errUtils.CheckErrorPrintAndExit(err, "", "")
	return value
}
