package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Engine owns an instance-local language registry; it has no implicit evaluators.
// Its zero value is usable. Register adapters before evaluation for reproducibility.
type Engine struct {
	contextProvider ContextProvider
	factCollector   FactCollector
	mu              sync.RWMutex
	evaluators      map[string]Evaluator
}

// NewEngine returns an isolated registry with the supported pure evaluators.
func NewEngine(options ...EngineOption) *Engine {
	defer perf.Track(nil, "policy.NewEngine")()

	engine := &Engine{evaluators: map[string]Evaluator{CELEngine: &CEL{}, StarlarkEngine: &Starlark{}, OPAEngine: &OPA{}, JSONSchemaEngine: &JSONSchema{}}}
	for _, option := range options {
		option(engine)
	}
	return engine
}

// Register adds a language adapter without permitting replacement of an existing one.
func (e *Engine) Register(name string, evaluator Evaluator) error {
	defer perf.Track(nil, "policy.Engine.Register")()

	if strings.TrimSpace(name) == "" || evaluator == nil {
		return fmt.Errorf("%w: engine name and evaluator are required", errUtils.ErrPolicyInvalid)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.evaluators[name]; exists {
		return fmt.Errorf("%w: %s", errUtils.ErrPolicyEngineRegistered, name)
	}
	if e.evaluators == nil {
		e.evaluators = make(map[string]Evaluator)
	}
	e.evaluators[name] = evaluator
	return nil
}

// Evaluate evaluates only rules for the explicit request kind, retaining all outcomes.
// Missing required data and unknown engines fail closed even for a false predicate.
// Approvals are outstanding here; the CI adapter verifies review evidence separately.
func (e *Engine) Evaluate(ctx context.Context, request *Request, rules []Rule) (Decision, error) {
	defer perf.Track(nil, "policy.Engine.Evaluate")()

	decision := Decision{Status: NotApplicable, Findings: []Finding{}}
	if request != nil {
		Explain(request.Kind, "", "gate reached")
	}
	if err := validateRequest(ctx, request); err != nil {
		decision.Status = EvaluationError
		return decision, err
	}
	if err := validateRuleIdentities(rules); err != nil {
		decision.Status = EvaluationError
		return decision, err
	}
	if !hasMatchingKind(request.Kind, rules) {
		return decision, nil
	}
	request, err := e.prepareContext(ctx, request)
	if err != nil {
		decision.Status = EvaluationError
		return decision, err
	}
	request, err = cloneJSON(request)
	if err == nil && e.factCollector != nil {
		err = e.factCollector.CollectFacts(ctx, request, rules)
	}
	if err != nil {
		decision.Status = EvaluationError
		return decision, err
	}
	var evaluationErrors []error
	for index := range rules {
		rule := &rules[index]
		if rule.On != request.Kind {
			continue
		}
		finding, err := e.evaluateRule(ctx, request, rule)
		decision.Findings = append(decision.Findings, finding)
		decision.Status = dominantStatus(decision.Status, finding.Status)
		if err != nil {
			evaluationErrors = append(evaluationErrors, err)
		}
	}
	return decision, errors.Join(evaluationErrors...)
}

func validateRequest(ctx context.Context, request *Request) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPolicyEvaluation, err)
	}
	if request == nil || request.Kind == "" {
		return fmt.Errorf("%w: kind is required", errUtils.ErrPolicyInputInvalid)
	}
	var err error
	if request.Kind == File {
		err = validateDocument(request.Input)
	} else {
		_, err = decodeObject(request.Input)
	}
	if err == nil && request.Kind == AtmosAffected {
		err = validateAffectedInput(request.Input)
	}
	if err == nil && request.Kind == TerraformPlan {
		err = ValidateTerraformPlan(request.Input)
	}
	return err
}

func (e *Engine) evaluateRule(ctx context.Context, request *Request, rule *Rule) (Finding, error) {
	finding := Finding{RuleID: rule.ID, Status: NotApplicable}
	result, effects, err := e.match(ctx, request, rule)
	finding.Diagnostics = result.Diagnostics
	if err != nil {
		finding.Status = EvaluationError
		finding.Error = err.Error()
		Explain(request.Kind, rule.ID, "evaluation error; required input may be unavailable")
		return finding, fmt.Errorf("%w: rule %q: %w", errUtils.ErrPolicyEvaluation, rule.ID, err)
	}
	if result.Matched {
		finding.Effects = effects
		finding.Status = Allow
		if len(effects.Approvals) > 0 {
			finding.Status = AwaitingApproval
		}
		if effects.Deny {
			finding.Status = Deny
		}
	}
	Explain(request.Kind, rule.ID, string(finding.Status))
	for _, hook := range finding.Effects.Hooks {
		Explain(request.Kind, rule.ID, fmt.Sprintf("required hook %s at %s (require_success=%t)", hook.Ref, hook.Event, hook.RequireSuccess))
	}
	for _, approval := range finding.Effects.Approvals {
		Explain(request.Kind, rule.ID, fmt.Sprintf("required approval %s from %s", approval.ID, approval.Provider))
	}
	return finding, nil
}

func (e *Engine) match(ctx context.Context, request *Request, rule *Rule) (EvaluationResult, Effects, error) {
	selected, err := selectRule(ctx, request, rule)
	if err != nil || !selected {
		return EvaluationResult{}, Effects{}, err
	}
	e.mu.RLock()
	evaluator, exists := e.evaluators[rule.Engine]
	e.mu.RUnlock()
	if !exists {
		return EvaluationResult{}, Effects{}, fmt.Errorf("%w: %s", errUtils.ErrPolicyEngineUnknown, rule.Engine)
	}
	if request.Kind == Source {
		var err error
		request, err = sourceRequest(request, rule.AllowedRoots)
		if err != nil {
			return EvaluationResult{}, Effects{}, err
		}
	}
	// JSON cloning rejects unsupported raw payloads and isolates each adapter and result.
	input, err := prepareEvaluation(request, rule)
	if err != nil {
		return EvaluationResult{}, Effects{}, err
	}
	effects, err := cloneJSON(rule.Effects)
	if err != nil {
		return EvaluationResult{}, Effects{}, err
	}
	if detailed, ok := evaluator.(DetailedEvaluator); ok {
		result, err := detailed.EvaluateDetailed(ctx, input)
		return result, effects, err
	}
	matched, err := evaluator.Evaluate(ctx, input)
	return EvaluationResult{Matched: matched}, effects, err
}

func validateRuleIdentities(rules []Rule) error {
	seen := make(map[string]bool, len(rules))
	for index := range rules {
		rule := &rules[index]
		if strings.TrimSpace(rule.ID) == "" || rule.On == "" || seen[rule.ID] {
			return fmt.Errorf("%w: each rule needs a unique id and input kind", errUtils.ErrPolicyInvalid)
		}
		seen[rule.ID] = true
	}
	return nil
}

func validateRule(rule *Rule) error {
	if rule.ID == "" || rule.Engine == "" || (strings.TrimSpace(rule.Expression) == "" && rule.OPA == nil && rule.Schema == nil) {
		return fmt.Errorf("%w: id, engine, and expression are required", errUtils.ErrPolicyInvalid)
	}
	if len(rule.Parameters) > 0 {
		if _, err := decodeObject(rule.Parameters); err != nil {
			return fmt.Errorf("%w: parameters: %w", errUtils.ErrPolicyInvalid, err)
		}
	}
	if err := validateChecks(rule.Checks); err != nil {
		return err
	}
	return validateEffects(rule.Effects)
}

func validateEffects(effects Effects) error {
	for _, approval := range effects.Approvals {
		if approval.ID == "" || approval.Provider == "" {
			return fmt.Errorf("%w: approval id and provider are required", errUtils.ErrPolicyInvalid)
		}
		if _, err := decodeObject(approval.Spec); err != nil {
			return fmt.Errorf("%w: approval spec: %w", errUtils.ErrPolicyInvalid, err)
		}
	}
	for _, hook := range effects.Hooks {
		if hook.Ref == "" || hook.Event == "" {
			return fmt.Errorf("%w: hook ref and event are required", errUtils.ErrPolicyInvalid)
		}
	}
	return nil
}

func cloneJSON[T any](value T) (T, error) {
	var result T
	data, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(data, &result)
	}
	if err != nil {
		return result, fmt.Errorf("%w: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	return result, nil
}

func dominantStatus(left, right DecisionStatus) DecisionStatus {
	rank := map[DecisionStatus]int{NotApplicable: 0, Allow: 1, AwaitingApproval: 2, Deny: 3, EvaluationError: 4}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func selectRule(ctx context.Context, request *Request, rule *Rule) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrPolicyEvaluation, err)
	}
	if err := validateRule(rule); err != nil {
		return false, err
	}
	if selected, err := rule.Match.Matches(request.Subject); err != nil || !selected {
		if err == nil {
			Explain(request.Kind, rule.ID, "excluded by selector")
		}
		return false, err
	}
	for _, requirement := range rule.RequiredCI {
		if err := requireCI(request.CI, requirement); err != nil {
			return false, err
		}
	}
	return true, nil
}

func hasMatchingKind(kind InputKind, rules []Rule) bool {
	for i := range rules {
		if rules[i].On == kind {
			return true
		}
	}
	return false
}

func prepareEvaluation(request *Request, rule *Rule) (Evaluation, error) {
	input, err := cloneJSON(Evaluation{Expression: rule.Expression, Parameters: rule.Parameters, Request: request, OPA: rule.OPA, Schema: rule.Schema})
	if err != nil || len(rule.Checks) == 0 {
		return input, err
	}
	// Version/calendar checks (including timezone loading) belong to the host.
	// Language adapters consume only detached data and precomputed boolean results.
	activation, err := evaluationActivation(input)
	if err != nil {
		return input, err
	}
	input.Checks, err = evaluateChecks(rule.Checks, activation)
	return input, err
}
