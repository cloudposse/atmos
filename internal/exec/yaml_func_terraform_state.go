package exec

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
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

// isRecoverableTerraformError checks whether a lookup is recoverable because state or an
// output genuinely does not exist yet. Authentication, credential-refresh, network, and
// backend API failures must remain visible rather than silently using a fallback value.
func isRecoverableTerraformError(err error) bool {
	return errors.Is(err, errUtils.ErrTerraformStateNotProvisioned) ||
		errors.Is(err, errUtils.ErrTerraformOutputNotFound)
}

// isRecoverableInWarnMode is the classification processNodesWithContext uses when the
// caller selected --error-mode=warn/silent. Error mode only degrades a genuinely
// unprovisioned state/output; it never hides credential or backend failures.
func isRecoverableInWarnMode(err error) bool {
	return isRecoverableTerraformError(err)
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

	return lookupTerraformState(atmosConfig, stackInfo, &terraformStateLookup{yamlFunc: input, stack: stack, component: component, output: output})
}

// terraformLookupAuth extracts the auth context and auth manager a Terraform state/output lookup
// should use from stackInfo. AuthDisabled is propagated downstream even when no AuthManager was
// created: the wrapper's stack info tells the getter to skip resolving the target component's own
// auth section.
func terraformLookupAuth(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) (*schema.AuthContext, any) {
	if stackInfo == nil {
		return nil, nil
	}
	authContext := stackInfo.AuthContext
	authManager := stackInfo.AuthManager
	if authdeferred.IsDeferred(atmosConfig.AuthManager) || (authManager == nil && stackInfo.AuthDisabled) {
		authManager = &authContextWrapper{stackInfo: stackInfo}
	}
	return authContext, authManager
}

// lookupTerraformState reads the real Terraform state. When `--use-mocks` runs in fallback mode and
// the component declares mocks, the whole real output map is merged over the mocks and the
// expression is evaluated against the result (see lookupTerraformStateWithMocks). Otherwise the
// plain real lookup runs, with a YQ `//` default rescuing a recoverable error.
func lookupTerraformState(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
) (any, error) {
	mocks, err := declaredFallbackMocks(atmosConfig, stackInfo, lookup)
	if err != nil {
		return nil, err
	}
	if mocks != nil {
		return lookupTerraformStateWithMocks(atmosConfig, stackInfo, lookup, mocks)
	}

	authContext, authManager := terraformLookupAuth(atmosConfig, stackInfo)

	value, err := stateGetter.GetState(atmosConfig, lookup.yamlFunc, lookup.stack, lookup.component, lookup.output, false, authContext, authManager, terraformLookupOptions(stackInfo)...)
	if err != nil {
		return handleTerraformStateError(atmosConfig, lookup, err)
	}

	return value, nil
}

// lookupTerraformStateWithMocks fetches the whole real output map and evaluates the expression
// against it overlaid on the component mocks. A recoverable error (state not provisioned, output
// not found) is treated as an empty real map; anything else (auth, network, backend) is returned
// unchanged so mocks never hide it.
func lookupTerraformStateWithMocks(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
	mocks map[string]any,
) (any, error) {
	authContext, authManager := terraformLookupAuth(atmosConfig, stackInfo)

	whole, err := stateGetter.GetState(atmosConfig, lookup.yamlFunc, lookup.stack, lookup.component, terraformAllOutputsExpression, false, authContext, authManager, terraformLookupOptions(stackInfo)...)
	if err != nil && !isRecoverableTerraformError(err) {
		return nil, err
	}

	var realOutputs map[string]any
	if err == nil {
		realOutputs, _ = whole.(map[string]any)
	}
	return resolveTerraformOutputWithMocks(atmosConfig, lookup, mocks, realOutputs, err)
}

// handleTerraformStateError resolves a failed state lookup: a recoverable error may be rescued by
// a YQ default; anything else fails unchanged.
func handleTerraformStateError(
	atmosConfig *schema.AtmosConfiguration,
	lookup *terraformStateLookup,
	err error,
) (any, error) {
	if !isRecoverableTerraformError(err) {
		// Non-recoverable error: auth, network, or backend failures are never masked.
		return nil, err
	}
	if !hasYqDefault(lookup.output) {
		return nil, err
	}
	log.Debug(
		"Evaluating YQ default for recoverable error",
		"function", lookup.yamlFunc,
		"error", err.Error(),
	)
	// Evaluate YQ against an empty map to get the default value.
	defaultValue, yqErr := evaluateYqDefault(atmosConfig, lookup.output)
	if yqErr != nil {
		// If YQ evaluation fails, return the original error.
		return nil, fmt.Errorf("%w: failed to evaluate YQ default: %w", err, yqErr)
	}
	return defaultValue, nil
}
