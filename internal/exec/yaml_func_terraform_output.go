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

// processTagTerraformOutput processes `!terraform.output` YAML tag.
//
//nolint:unparam // stackInfo is used via processTagTerraformOutputWithContext
func processTagTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	return processTagTerraformOutputWithContext(atmosConfig, input, currentStack, nil, stackInfo)
}

// trackOutputDependency records the dependency in the resolution context and returns a cleanup function.
func trackOutputDependency(
	atmosConfig *schema.AtmosConfiguration,
	resolutionCtx *ResolutionContext,
	component string,
	stack string,
	input string,
) func() {
	if resolutionCtx == nil {
		return func() {}
	}

	node := DependencyNode{
		Component:    component,
		Stack:        stack,
		FunctionType: "terraform.output",
		FunctionCall: input,
	}

	// Check and record this dependency.
	if err := resolutionCtx.Push(atmosConfig, node); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	// Return cleanup function.
	return func() { resolutionCtx.Pop(atmosConfig) }
}

// processTagTerraformOutputWithContext processes `!terraform.output` YAML tag with cycle detection.
func processTagTerraformOutputWithContext(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	defer perf.Track(atmosConfig, "exec.processTagTerraformOutputWithContext")()

	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTerraformOutput)
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

	// Track dependency and defer cleanup.
	defer trackOutputDependency(atmosConfig, resolutionCtx, component, stack, input)()

	// Extract authContext from stackInfo if available.
	var authContext *schema.AuthContext
	if stackInfo != nil {
		authContext = stackInfo.AuthContext
	}

	value, exists, err := outputGetter.GetOutput(atmosConfig, stack, component, output, false, authContext)
	if err != nil {
		er := fmt.Errorf("failed to get terraform output for component %s in stack %s, output %s: %w", component, stack, output, err)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// If the output doesn't exist, return nil (backward compatible).
	// This allows YAML functions to reference outputs that don't exist yet.
	// Use yq fallback syntax (.output // "default") for default values.
	if !exists {
		return nil
	}

	// value may be nil here if the terraform output is legitimately null, which is valid.
	return value
}
