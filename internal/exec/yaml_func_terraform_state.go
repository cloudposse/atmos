package exec

import (
	"errors"
	"fmt"
	"slices"
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

// isRecoverableTerraformError checks whether a lookup is recoverable because state or an
// output genuinely does not exist yet. Authentication, credential-refresh, network, and
// backend API failures must remain visible rather than silently using a fallback value.
func isRecoverableTerraformError(err error) bool {
	return errors.Is(err, errUtils.ErrTerraformStateNotProvisioned) ||
		errors.Is(err, errUtils.ErrTerraformOutputNotFound)
}

// isRecoverableInWarnMode is the classification processNodesWithContext uses when the
// caller selected --error-mode=warn/silent. It is deliberately wider than
// isRecoverableTerraformError: warn mode degrades a value it could not read to
// `(computed)` and keeps going, rather than failing the whole command.
//
// Besides a genuinely unprovisioned state/output, that includes a backend read that failed —
// most importantly a cross-account `AccessDenied`. In a multi-account repository each stage
// keeps its Terraform state in its own account, so a command that walks every stack
// (`atmos list stacks`, an unfiltered `atmos describe stacks`) will always hit backends the
// current identity cannot read. Those are expected in that topology, not defects, and one of
// them must not abort the command. See https://github.com/cloudposse/atmos/issues/2566.
//
// Three deliberate limits:
//
//   - Warn/silent mode only. `--error-mode=strict` still surfaces every one of these.
//   - isRecoverableTerraformError — which gates the YQ `//` default operator — is unchanged,
//     so `!terraform.state … // "fallback"` still refuses to paper over a credential failure
//     with its literal default.
//   - Environmental failures only. ErrReadTerraformState is the wrapper GetTerraformState puts
//     around *every* backend failure, including several that are defects in the repository's
//     own manifests rather than conditions of the environment. Those are filtered out by
//     isTerraformStateManifestDefect below and stay fatal in every error mode.
//
// Scope: this covers `!terraform.state`, which reads the backend directly and wraps every
// failure in ErrReadTerraformState. `!terraform.output` shells out to `terraform output` and
// wraps any subprocess failure in ErrTerraformOutputFailed — far broader (missing binary, HCL
// error, timeout) — so degrading it wholesale would hide real defects. It is left strict;
// narrowing that error enough to degrade only access failures is tracked as follow-up in
// docs/fixes/2026-07-28-inventory-commands-read-remote-tfstate.md.
func isRecoverableInWarnMode(err error) bool {
	if isTerraformStateManifestDefect(err) {
		return false
	}

	return isRecoverableTerraformError(err) ||
		errors.Is(err, errUtils.ErrReadTerraformState)
}

// terraformStateManifestDefects are the failures that reach the classifier wrapped in
// ErrReadTerraformState but describe a mistake in the stack manifests rather than a condition
// of the environment. Degrading these to `(computed)` would turn a typo into plausible-looking
// output that exits 0 — the user would have no signal at all — so warn and silent mode must
// keep failing on them just as strict mode does.
//
// The distinction only became expressible once GetTerraformState started wrapping its cause
// with %w instead of %v (see terraform_state_utils.go); before that every backend failure
// arrived as one flat, unmatchable string.
var terraformStateManifestDefects = []error{
	// `backend_type` names a backend Atmos does not implement — a typo in the manifest.
	errUtils.ErrUnsupportedBackendType,
	// The state file was retrieved but is not parseable as Terraform state (corrupt or
	// truncated). Substituting `(computed)` here would hide state corruption.
	errUtils.ErrProcessTerraformStateFile,
	// A `static` remote-state backend does not declare the requested output key.
	errUtils.ErrStaticRemoteStateOutputMissing,
	// The YQ expression failed against state Atmos successfully retrieved.
	errUtils.ErrEvaluateTerraformBackendVariable,
}

// isTerraformStateManifestDefect reports whether err describes a manifest defect that must stay
// fatal even in warn/silent mode.
func isTerraformStateManifestDefect(err error) bool {
	return slices.ContainsFunc(terraformStateManifestDefects, func(sentinel error) bool {
		return errors.Is(err, sentinel)
	})
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
