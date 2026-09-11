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
//
// The functionType parameter identifies the calling YAML function (e.g. "terraform.output",
// "aws.cloudformation.output") for the pushed DependencyNode — shared by every
// output-fetching YAML function that reuses this "component [stack] output"
// grammar, so it must be threaded through rather than hardcoded, or cycle
// diagnostics for non-terraform.output callers would misreport their origin.
func trackOutputDependency(
	atmosConfig *schema.AtmosConfiguration,
	resolutionCtx *ResolutionContext,
	component string,
	stack string,
	functionType string,
	input string,
) (func(), error) {
	if resolutionCtx == nil {
		return func() {}, nil
	}

	node := DependencyNode{
		Component:    component,
		Stack:        stack,
		FunctionType: functionType,
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
	cleanup, err := trackOutputDependency(atmosConfig, resolutionCtx, component, stack, "terraform.output", input)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if value, mocked, mockErr := resolveTerraformMockOutput(atmosConfig, stackInfo, stack, component, output); mocked {
		return value, mockErr
	}

	return lookupTerraformOutput(atmosConfig, stackInfo, &terraformStateLookup{yamlFunc: input, stack: stack, component: component, output: output})
}

// lookupTerraformOutput reads the real Terraform output. When `--use-mocks` runs in fallback mode
// and the component declares mocks, the whole real output map is merged over the mocks and the
// expression is evaluated against the result (see lookupTerraformOutputWithMocks). Otherwise the
// plain real lookup runs, with a YQ `//` default rescuing a recoverable error or a missing output.
func lookupTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
) (any, error) {
	mocks, err := declaredFallbackMocks(atmosConfig, stackInfo, lookup)
	if err != nil {
		return nil, err
	}
	if mocks != nil {
		return lookupTerraformOutputWithMocks(atmosConfig, stackInfo, lookup, mocks)
	}

	authContext, authManager := terraformLookupAuth(atmosConfig, stackInfo)

	value, exists, err := outputGetter.GetOutput(atmosConfig, lookup.stack, lookup.component, lookup.output, false, authContext, authManager, terraformLookupOptions(stackInfo)...)
	if err != nil {
		return handleTerraformOutputError(atmosConfig, lookup, err)
	}

	// If the output doesn't exist, try the YQ default.
	if !exists {
		return handleMissingTerraformOutput(atmosConfig, lookup)
	}

	// value may be nil here if the terraform output is legitimately null, which is valid.
	return value, nil
}

// wrapTerraformOutputError adds the component, stack, and output context to a failed output lookup.
func wrapTerraformOutputError(lookup *terraformStateLookup, err error) error {
	return fmt.Errorf("failed to get terraform output for component %s in stack %s, output %s: %w", lookup.component, lookup.stack, lookup.output, err)
}

// lookupTerraformOutputWithMocks fetches the whole real output map and evaluates the expression
// against it overlaid on the component mocks. A recoverable error (state not provisioned, output
// not found) is treated as an empty real map and kept (wrapped) as the error to report when
// nothing resolves; anything else (auth, network, backend) is returned unchanged so mocks never
// hide it.
func lookupTerraformOutputWithMocks(
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
	lookup *terraformStateLookup,
	mocks map[string]any,
) (any, error) {
	authContext, authManager := terraformLookupAuth(atmosConfig, stackInfo)

	whole, exists, err := outputGetter.GetOutput(atmosConfig, lookup.stack, lookup.component, terraformAllOutputsExpression, false, authContext, authManager, terraformLookupOptions(stackInfo)...)
	if err != nil {
		wrapped := wrapTerraformOutputError(lookup, err)
		if !isRecoverableTerraformError(err) {
			return nil, withFallbackModeHint(wrapped)
		}
		return resolveTerraformOutputWithMocks(atmosConfig, lookup, mocks, nil, wrapped)
	}

	var realOutputs map[string]any
	if exists {
		realOutputs, _ = whole.(map[string]any)
	}
	return resolveTerraformOutputWithMocks(atmosConfig, lookup, mocks, realOutputs, nil)
}

// handleTerraformOutputError resolves a failed output lookup. Only recoverable terraform errors
// (state not provisioned, output not found) may be rescued by a YQ default; non-recoverable errors
// (API failures, auth errors, infrastructure issues) fail hard.
func handleTerraformOutputError(
	atmosConfig *schema.AtmosConfiguration,
	lookup *terraformStateLookup,
	err error,
) (any, error) {
	wrapped := wrapTerraformOutputError(lookup, err)
	if !isRecoverableTerraformError(err) {
		return nil, wrapped
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

// handleMissingTerraformOutput resolves an output that does not exist in state: a YQ default wins,
// and otherwise nil (backward compatible).
func handleMissingTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	lookup *terraformStateLookup,
) (any, error) {
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
