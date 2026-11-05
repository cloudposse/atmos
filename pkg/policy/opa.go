package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage/inmem"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// OPAEngine evaluates embedded, pure Rego policies.
const OPAEngine = "opa"

const opaTimeout = 20 * time.Second

// OPAConfig contains materialized module source, never paths to load at evaluation time.
// Version 0 supports existing Rego; version 1 opts into Rego v1 syntax.
type OPAConfig struct {
	Query       string            `json:"query"`
	Modules     map[string]string `json:"modules"`
	RegoVersion int               `json:"rego_version"`
}

// OPA accepts a boolean predicate or an array/set of violation strings or objects.
type OPA struct{}

// Evaluate implements the boolean adapter interface.
func (o *OPA) Evaluate(ctx context.Context, evaluation Evaluation) (bool, error) {
	defer perf.Track(nil, "policy.OPA.Evaluate")()
	result, err := o.EvaluateDetailed(ctx, evaluation)
	return result.Matched, err
}

// EvaluateDetailed preserves violation messages and structured Rego values.
func (o *OPA) EvaluateDetailed(ctx context.Context, evaluation Evaluation) (EvaluationResult, error) {
	defer perf.Track(nil, "policy.OPA.EvaluateDetailed")()
	if err := validateRequest(ctx, evaluation.Request); err != nil {
		return EvaluationResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, opaTimeout)
	defer cancel()
	activation, err := evaluationActivation(evaluation)
	if err != nil {
		return EvaluationResult{}, err
	}
	input := activation["input"]
	delete(activation, "input")
	query, err := prepareOPA(ctx, evaluation.OPA, activation)
	if err != nil {
		return EvaluationResult{}, err
	}
	results, err := query.Eval(ctx, rego.EvalInput(input))
	if contextErr := ctx.Err(); contextErr != nil {
		return EvaluationResult{}, fmt.Errorf("%w: %w", errUtils.ErrPolicyEvaluation, contextErr)
	}
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("%w: OPA evaluation failed (input details withheld)", errUtils.ErrPolicyEvaluation)
	}
	if len(results) != 1 || len(results[0].Expressions) != 1 || len(results[0].Bindings) != 0 {
		return EvaluationResult{}, fmt.Errorf("%w: OPA query must return one defined boolean or violation collection", errUtils.ErrPolicyEvaluation)
	}
	return opaResult(results[0].Expressions[0].Value)
}

func prepareOPA(ctx context.Context, config *OPAConfig, metadata map[string]any) (rego.PreparedEvalQuery, error) {
	if err := validateOPAConfig(config); err != nil {
		return rego.PreparedEvalQuery{}, err
	}
	version := ast.RegoV0
	if config.RegoVersion == 1 {
		version = ast.RegoV1
	}
	capabilities := opaCapabilities(version)
	options := []func(*rego.Rego){rego.Query(config.Query), rego.SetRegoVersion(version), rego.Capabilities(capabilities), rego.StrictBuiltinErrors(true), rego.EnablePrintStatements(true), rego.Store(inmem.NewFromObject(map[string]any{"atmos_context": metadata}))}
	names := make([]string, 0, len(config.Modules))
	for name := range config.Modules {
		names = append(names, name)
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		source := config.Modules[name]
		total += len(source)
		if total > maxPolicyDocumentBytes {
			return rego.PreparedEvalQuery{}, fmt.Errorf("%w: OPA modules exceed size limit", errUtils.ErrPolicyInvalid)
		}
		options = append(options, rego.Module(name, source))
	}
	prepared, err := rego.New(options...).PrepareForEval(ctx)
	if err != nil {
		return prepared, fmt.Errorf("%w: compile OPA: %w", errUtils.ErrPolicyInvalid, err)
	}
	return prepared, nil
}

func opaResult(value any) (EvaluationResult, error) {
	if matched, ok := value.(bool); ok {
		return EvaluationResult{Matched: matched}, nil
	}
	items, ok := value.([]any)
	if !ok {
		return EvaluationResult{}, fmt.Errorf("%w: OPA result must be boolean or a violation array/set", errUtils.ErrPolicyEvaluation)
	}
	result := EvaluationResult{Matched: len(items) > 0}
	for _, item := range items {
		diagnostic := Diagnostic{}
		switch value := item.(type) {
		case string:
			diagnostic.Message = value
		case map[string]any:
			raw, err := json.Marshal(value)
			if err != nil {
				return EvaluationResult{}, err
			}
			diagnostic.Details = raw
			diagnostic.Message, _ = value["message"].(string)
			if diagnostic.Message == "" {
				diagnostic.Message, _ = value["msg"].(string)
			}
			if diagnostic.Message == "" {
				diagnostic.Message = string(raw)
			}
			diagnostic.Path, _ = value["path"].(string)
		default:
			return EvaluationResult{}, fmt.Errorf("%w: OPA violations must be strings or objects", errUtils.ErrPolicyEvaluation)
		}
		result.Diagnostics = append(result.Diagnostics, diagnostic)
	}
	// Sets and arrays share a stable presentation independent of Rego iteration order.
	sort.Slice(result.Diagnostics, func(i, j int) bool {
		a, _ := json.Marshal(result.Diagnostics[i])
		b, _ := json.Marshal(result.Diagnostics[j])
		return string(a) < string(b)
	})
	return result, nil
}

func opaCapabilities(version ast.RegoVersion) *ast.Capabilities {
	capabilities := ast.CapabilitiesForThisVersion(ast.CapabilitiesRegoVersion(version))
	// Exclude network, clock, randomness, environment and logging capabilities.
	builtins := capabilities.Builtins[:0]
	for _, builtin := range capabilities.Builtins {
		if !builtin.Nondeterministic && builtin.Name != "trace" && builtin.Name != "print" && builtin.Name != "internal.print" && builtin.Name != "opa.runtime" {
			builtins = append(builtins, builtin)
		}
	}
	capabilities.Builtins = builtins
	capabilities.AllowNet = []string{}
	return capabilities
}

func validateOPAConfig(config *OPAConfig) error {
	if config == nil || strings.TrimSpace(config.Query) == "" || len(config.Query) > maxExpressionBytes || len(config.Modules) == 0 || (config.RegoVersion != 0 && config.RegoVersion != 1) {
		return fmt.Errorf("%w: OPA requires query, modules, and rego_version 0 or 1", errUtils.ErrPolicyInvalid)
	}
	return nil
}
