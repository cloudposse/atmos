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
) (any, error) {
	return processTagTerraformOutputWithContext(atmosConfig, input, currentStack, nil, stackInfo)
}

// trackOutputDependency records the dependency in the resolution context and returns a cleanup function.
// It returns an error if cycle detection fails.
func trackOutputDependency(
	atmosConfig *schema.AtmosConfiguration,
	resolutionCtx *ResolutionContext,
	component string,
	stack string,
	input string,
) (func(), error) {
	if resolutionCtx == nil {
		return func() {}, nil
	}

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

	// Return cleanup function.
	return func() { resolutionCtx.Pop(atmosConfig) }, nil
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

	var component string
	var stack string
	var output string

	// Split the string into slices based on any whitespace (one or more spaces, tabs, or newlines),
	// while also ignoring leading and trailing whitespace.
	// SplitStringByDelimiter splits a string by the delimiter, not splitting inside quotes.
	parts, err := u.SplitStringByDelimiter(str, ' ')
	if err != nil {
		return nil, err
	}

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
			log.FieldFunction, input,
			"stack", currentStack,
		)
	default:
		return nil, fmt.Errorf("%w %s", errUtils.ErrYamlFuncInvalidArguments, input)
	}

	// Track dependency and get cleanup function.
	cleanup, err := trackOutputDependency(atmosConfig, resolutionCtx, component, stack, input)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Extract authContext and authManager from stackInfo if available.
	var authContext *schema.AuthContext
	var authManager any
	if stackInfo != nil {
		authContext = stackInfo.AuthContext
		authManager = stackInfo.AuthManager
	}

	value, exists, err := outputGetter.GetOutput(atmosConfig, stack, component, output, false, authContext, authManager)
	if err != nil {
		// For API/infrastructure errors, check if we can use YQ default.
		if hasYqDefault(output) {
			log.Debug("Evaluating YQ default for output error",
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
