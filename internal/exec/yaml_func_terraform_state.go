package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	fn "github.com/cloudposse/atmos/pkg/function"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// processTagTerraformState processes `!terraform.state` YAML tag.
//
//nolint:unparam // stackInfo is used via processTagTerraformStateWithContext
func processTagTerraformState(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, error) {
	return processTagTerraformStateWithContext(atmosConfig, input, currentStack, nil, stackInfo)
}

// processTagTerraformStateWithContext processes `!terraform.state` YAML tag with cycle detection.
func processTagTerraformStateWithContext(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagTerraformStateWithContext")()

	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTerraformState)
	if err != nil {
		return nil, err
	}

	// Parse function arguments using the purpose-built parser.
	// Format: component [stack] expression
	// Stack is optional - if not provided, uses currentStack.
	component, stack, output := fn.ParseArgs(str)

	if component == "" {
		return nil, fmt.Errorf("%w: missing component: %s", errUtils.ErrYamlFuncInvalidArguments, input)
	}

	if output == "" {
		return nil, fmt.Errorf("%w: missing output expression: %s", errUtils.ErrYamlFuncInvalidArguments, input)
	}

	// If no stack was specified, use the current stack.
	if stack == "" {
		stack = currentStack
		log.Debug("Executing Atmos YAML function with component and output parameters; using current stack",
			"function", input,
			"stack", currentStack,
		)
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
			return nil, err
		}

		// Defer pop to ensure we clean up even if there's an error.
		defer resolutionCtx.Pop(atmosConfig)
	}

	// Extract authContext and authManager from stackInfo if available.
	var authContext *schema.AuthContext
	var authManager any
	if stackInfo != nil {
		authContext = stackInfo.AuthContext
		authManager = stackInfo.AuthManager
	}

	value, err := stateGetter.GetState(atmosConfig, input, stack, component, output, false, authContext, authManager)
	if err != nil {
		return nil, err
	}
	return value, nil
}
