package exec

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
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

// isRecoverableTerraformError checks if an error is recoverable (can use YQ default).
func isRecoverableTerraformError(err error) bool {
	return errors.Is(err, errUtils.ErrTerraformStateNotProvisioned) ||
		errors.Is(err, errUtils.ErrTerraformOutputNotFound)
}

// hasYqDefault checks if a YQ expression contains a default (fallback) operator.
func hasYqDefault(yqExpr string) bool {
	return strings.Contains(yqExpr, "//")
}

// evaluateYqDefault evaluates a YQ expression against an empty map to get the default value.
func evaluateYqDefault(atmosConfig *schema.AtmosConfiguration, yqExpr string) (any, error) {
	return tb.GetTerraformBackendVariable(atmosConfig, map[string]any{}, yqExpr)
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
			"function", input,
			"stack", currentStack,
		)
	default:
		return nil, fmt.Errorf("%w %s", errUtils.ErrYamlFuncInvalidArguments, input)
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
		// Check if this is a recoverable error AND the expression has a YQ default.
		if isRecoverableTerraformError(err) && hasYqDefault(output) {
			log.Debug("Evaluating YQ default for recoverable error",
				"function", input,
				"error", err.Error(),
			)
			// Evaluate YQ against an empty map to get the default value.
			defaultValue, yqErr := evaluateYqDefault(atmosConfig, output)
			if yqErr != nil {
				// If YQ evaluation fails, return the original error.
				return nil, fmt.Errorf("%w: failed to evaluate YQ default: %w", err, yqErr)
			}
			return defaultValue, nil
		}
		// Non-recoverable error or no default available.
		return nil, err
	}

	return value, nil
}
