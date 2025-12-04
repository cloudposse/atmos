package exec

import (
	"fmt"

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

	// Parse function arguments using the purpose-built parser.
	// Format: component [stack] expression
	// Stack is optional - if not provided, uses currentStack.
	component, stack, output := u.ParseFunctionArgs(str)

	if component == "" {
		er := fmt.Errorf("%w: missing component: %s", errUtils.ErrYamlFuncInvalidArguments, input)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	if output == "" {
		er := fmt.Errorf("%w: missing output expression: %s", errUtils.ErrYamlFuncInvalidArguments, input)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// If no stack was specified, use the current stack.
	if stack == "" {
		stack = currentStack
		log.Debug("Executing Atmos YAML function with component and output parameters; using current stack",
			"function", input,
			"stack", currentStack,
		)
	}

	// Track dependency and defer cleanup.
	defer trackOutputDependency(atmosConfig, resolutionCtx, component, stack, input)()

	// Extract authContext and authManager from stackInfo if available.
	var authContext *schema.AuthContext
	var authManager any
	if stackInfo != nil {
		authContext = stackInfo.AuthContext
		authManager = stackInfo.AuthManager
	}

	value, exists, err := outputGetter.GetOutput(atmosConfig, stack, component, output, false, authContext, authManager)
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
