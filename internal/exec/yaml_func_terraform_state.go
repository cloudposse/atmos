package exec

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
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

// isRecoverableTerraformError checks if an error is recoverable (can use YQ default). Used
// by the always-on local `//`-default evaluation regardless of --error-mode, so this stays
// deliberately narrow: only "state genuinely doesn't exist yet" — an infrastructure/API
// failure (e.g. an S3 credential or network error) must NOT silently fall back to a literal
// default outside of an explicit --error-mode=warn/silent opt-in (see
// TestTerraformOutput_APIErrorWithDefaultReturnsError). Warn-mode's broader tolerance lives
// in isRecoverableInWarnMode below, consumed only by processNodesWithContext's onWarning path.
func isRecoverableTerraformError(err error) bool {
	return errors.Is(err, errUtils.ErrTerraformStateNotProvisioned) ||
		errors.Is(err, errUtils.ErrTerraformOutputNotFound)
}

// isRecoverableInWarnMode is the classification processNodesWithContext uses when the
// caller explicitly opted into --error-mode=warn/silent (onWarning != nil). It's broader
// than isRecoverableTerraformError: on top of "state not yet provisioned", it also tolerates
// ErrGetObjectFromS3 — a backend read that failed for any other reason (credential refresh,
// network, permissions) — since the user has already said "continue past values that can't
// resolve right now" by choosing warn/silent. This must NOT be used by the always-on local
// `//`-default path (see isRecoverableTerraformError's doc comment for why).
func isRecoverableInWarnMode(err error) bool {
	return isRecoverableTerraformError(err) || errors.Is(err, errUtils.ErrGetObjectFromS3)
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

	parsed, err := fnparser.ParseTerraform(str)
	if err != nil {
		return nil, err
	}
	component = parsed.Component
	stack = parsed.Stack
	output = parsed.Expression
	if stack == "" {
		stack = currentStack
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
