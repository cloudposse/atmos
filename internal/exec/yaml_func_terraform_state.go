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

	component, stack, output, err := parseTerraformStateArgs(str, currentStack)
	if err != nil {
		return nil, fmt.Errorf("%w %s", errUtils.ErrYamlFuncInvalidArguments, input)
	}

	if stack == currentStack {
		log.Debug(
			"Executing Atmos YAML function with component and output parameters; using current stack",
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

	if value, mocked, mockErr := resolveTerraformMockOutput(atmosConfig, stackInfo, stack, component, output); mocked {
		return value, mockErr
	}

	// Extract authContext and authManager from stackInfo if available.
	var authContext *schema.AuthContext
	var authManager any
	if stackInfo != nil {
		authContext = stackInfo.AuthContext
		authManager = stackInfo.AuthManager
		if authManager == nil && stackInfo.AuthDisabled {
			authManager = &authContextWrapper{stackInfo: stackInfo}
		}
	}

	value, err := stateGetter.GetState(atmosConfig, input, stack, component, output, false, authContext, authManager)
	if err != nil {
		// Check if this is a recoverable error AND the expression has a YQ default.
		if isRecoverableTerraformError(err) && hasYqDefault(output) {
			log.Debug(
				"Evaluating YQ default for recoverable error",
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

// parseTerraformStateArgs parses a terraform.state function invocation. It preserves the legacy
// CSV-style quoting parser, then accepts an unquoted YQ expression containing whitespace. The
// latter is what YAML supplies for whole-value quoted functions, for example:
//
//	kms_key_arn: !terraform.state kms-key '.key_arn // "mock-value"'
func parseTerraformStateArgs(args string, currentStack string) (component string, stack string, output string, err error) {
	parts, splitErr := u.SplitStringByDelimiter(args, ' ')
	if splitErr == nil {
		switch len(parts) {
		case 3:
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2]), nil
		case 2:
			return strings.TrimSpace(parts[0]), currentStack, strings.TrimSpace(parts[1]), nil
		}
	}

	args = strings.TrimSpace(args)
	componentEnd := strings.IndexAny(args, " \t\n\r")
	if componentEnd <= 0 {
		return "", "", "", errUtils.ErrYamlFuncInvalidArguments
	}

	component = args[:componentEnd]
	remainder := strings.TrimLeft(args[componentEnd:], " \t\n\r")
	if remainder == "" {
		return "", "", "", errUtils.ErrYamlFuncInvalidArguments
	}

	// A YQ expression begins with one of these characters. Treat the complete remainder as
	// the output expression so whitespace in `//` fallbacks and pipes remains intact.
	if isTerraformStateExpressionStart(remainder[0]) {
		return component, currentStack, trimTerraformStateExpressionQuotes(remainder), nil
	}

	stackEnd := strings.IndexAny(remainder, " \t\n\r")
	if stackEnd <= 0 {
		return component, currentStack, remainder, nil
	}

	stack = remainder[:stackEnd]
	output = strings.TrimLeft(remainder[stackEnd:], " \t\n\r")
	if output == "" {
		return "", "", "", errUtils.ErrYamlFuncInvalidArguments
	}

	return component, stack, trimTerraformStateExpressionQuotes(output), nil
}

func isTerraformStateExpressionStart(char byte) bool {
	switch char {
	case '.', '|', '[', '{', '"', '\'':
		return true
	default:
		return false
	}
}

func trimTerraformStateExpressionQuotes(expression string) string {
	expression = strings.TrimSpace(expression)
	if len(expression) < 2 {
		return expression
	}

	if expression[0] == '\'' && expression[len(expression)-1] == '\'' {
		return expression[1 : len(expression)-1]
	}

	return expression
}
