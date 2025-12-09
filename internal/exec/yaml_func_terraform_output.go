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

// processTagTerraformOutput processes `!terraform.output` YAML tag.
//
//nolint:unparam // stackInfo is used via processTagTerraformOutputWithContext
func processTagTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, error) {
	return processTagTerraformOutputWithContext(atmosConfig, input, currentStack, nil, stackInfo)
}

// processTagTerraformOutputWithContext processes `!terraform.output` YAML tag with cycle detection.
func processTagTerraformOutputWithContext(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagTerraformOutputWithContext")()

	log.Debug("Executing Atmos YAML function", log.FieldFunction, input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTerraformOutput)
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
			log.FieldFunction, input,
			"stack", currentStack,
		)
	}

	// Check for circular dependencies if resolution context is provided.
	if resolutionCtx != nil {
		node := DependencyNode{
			Component:    component,
			Stack:        stack,
			FunctionType: "terraform.output",
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

	value, exists, err := outputGetter.GetOutput(atmosConfig, stack, component, output, false, authContext, authManager)
	if err != nil {
		// Only use YQ defaults for recoverable terraform errors (state not provisioned, output not found).
		// Non-recoverable errors (API failures, auth errors, infrastructure issues) should fail hard.
		if isRecoverableTerraformError(err) && hasYqDefault(output) {
			log.Debug("Evaluating YQ default for recoverable error",
				log.FieldFunction, input,
				"error", err.Error(),
			)
			// Evaluate YQ against an empty map to get the default value.
			defaultValue, yqErr := evaluateYqDefault(atmosConfig, output)
			if yqErr != nil {
				// If YQ evaluation fails, return the original error.
				return nil, fmt.Errorf("failed to get terraform output for component %s in stack %s, output %s: %w", component, stack, output, err)
			}
			return defaultValue, nil
		}
		return nil, fmt.Errorf("failed to get terraform output for component %s in stack %s, output %s: %w", component, stack, output, err)
	}

	// If the output doesn't exist, check if we can use YQ default.
	if !exists {
		if hasYqDefault(output) {
			log.Debug("Evaluating YQ default for non-existent output",
				log.FieldFunction, input,
				"component", component,
				"stack", stack,
				"output", output,
			)
			// Evaluate YQ against an empty map to get the default value.
			defaultValue, yqErr := evaluateYqDefault(atmosConfig, output)
			if yqErr != nil {
				// If YQ evaluation fails, return nil (backward compatible).
				log.Debug("YQ default evaluation failed, returning nil",
					log.FieldFunction, input,
					"error", yqErr.Error(),
				)
				return nil, nil
			}
			return defaultValue, nil
		}
		// No default available, return nil (backward compatible).
		return nil, nil
	}

	// value may be nil here if the terraform output is legitimately null, which is valid.
	return value, nil
}
