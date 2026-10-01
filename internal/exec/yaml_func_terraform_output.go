package exec

import (
	"fmt"

	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
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
			log.FieldFunction, input,
			"stack", currentStack,
		)
	}

	// Track dependency and get cleanup function.
	cleanup, err := trackOutputDependency(atmosConfig, resolutionCtx, component, stack, input)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if value, mocked, mockErr := resolveTerraformMockOutput(atmosConfig, stackInfo, stack, component, output); mocked {
		return value, mockErr
	}

	return lookupTerraformOutput(atmosConfig, stackInfo, &terraformStateLookup{yamlFunc: input, stack: stack, component: component, output: output})
}

// lookupTerraformOutput reads the real Terraform output and, when `--use-mocks` runs in fallback
// mode, falls back to the component mocks on a recoverable miss. Precedence: real value, then
// mock, then YQ `//` default, then the original error (or nil for a missing output).
func lookupTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
) (any, error) {
	authContext, authManager := terraformLookupAuth(atmosConfig, stackInfo)

	value, exists, err := outputGetter.GetOutput(atmosConfig, lookup.stack, lookup.component, lookup.output, false, authContext, authManager, terraformLookupOptions(stackInfo)...)
	if err != nil {
		return handleTerraformOutputError(atmosConfig, stackInfo, lookup, err)
	}

	// If the output doesn't exist, try the component mocks (fallback mode) and then a YQ default.
	if !exists {
		return handleMissingTerraformOutput(atmosConfig, stackInfo, lookup)
	}

	// value may be nil here if the terraform output is legitimately null, which is valid.
	return value, nil
}

// handleTerraformOutputError resolves a failed output lookup. Only recoverable terraform errors
// (state not provisioned, output not found) may be rescued by a component mock (fallback mode) or
// a YQ default; non-recoverable errors (API failures, auth errors, infrastructure issues) fail hard.
func handleTerraformOutputError(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
	err error,
) (any, error) {
	wrapped := fmt.Errorf("failed to get terraform output for component %s in stack %s, output %s: %w", lookup.component, lookup.stack, lookup.output, err)
	if !isRecoverableTerraformError(err) {
		return nil, wrapped
	}
	if mocked, handled, mockErr := resolveTerraformMockFallback(atmosConfig, stackInfo, lookup, wrapped); handled {
		return mocked, mockErr
	}
	if !hasYqDefault(lookup.output) {
		return nil, wrapped
	}
	log.Debug(
		"Evaluating YQ default for recoverable error",
		log.FieldFunction, lookup.yamlFunc,
		"error", err.Error(),
	)
	// Evaluate YQ against an empty map to get the default value.
	defaultValue, yqErr := evaluateYqDefault(atmosConfig, lookup.output)
	if yqErr != nil {
		// If YQ evaluation fails, return the original error.
		return nil, wrapped
	}
	return defaultValue, nil
}

// handleMissingTerraformOutput resolves an output that does not exist in state: a component mock
// (fallback mode) wins, then a YQ default, and otherwise nil (backward compatible).
func handleMissingTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
) (any, error) {
	if mocked, handled, mockErr := resolveTerraformMockFallback(atmosConfig, stackInfo, lookup, nil); handled {
		return mocked, mockErr
	}
	if !hasYqDefault(lookup.output) {
		// No default available, return nil (backward compatible).
		return nil, nil
	}
	log.Debug(
		"Evaluating YQ default for non-existent output",
		log.FieldFunction, lookup.yamlFunc,
		"component", lookup.component,
		"stack", lookup.stack,
		"output", lookup.output,
	)
	// Evaluate YQ against an empty map to get the default value.
	defaultValue, yqErr := evaluateYqDefault(atmosConfig, lookup.output)
	if yqErr != nil {
		// If YQ evaluation fails, return nil (backward compatible).
		log.Debug(
			"YQ default evaluation failed, returning nil",
			log.FieldFunction, lookup.yamlFunc,
			"error", yqErr.Error(),
		)
		return nil, nil
	}
	return defaultValue, nil
}
