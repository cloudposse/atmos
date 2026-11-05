// Package recovery executes retry obligations at individual component operation boundaries.
package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Result is the output of one attempt. Capture must be reset for each attempt.
type Result struct {
	Err      error
	Stdout   string
	Stderr   string
	Output   string
	ExitCode int
}

// Runner pins an event's policies and instance identity. Each Run has its own budget.
// It is safe to share a runner between independent API calls and polling iterations.
type Runner struct {
	event   policy.InputKind
	input   policy.ErrorInput
	subject *policy.Subject
	rules   []policy.Rule
}

// New prepares the concrete component/subcommand event before executing any attempt.
// A nil runner means no error policies apply; Call then invokes its function directly.
func New(info *schema.ConfigAndStacksInfo, componentType, command string) (*Runner, error) {
	defer perf.Track(nil, "recovery.New")()
	if info != nil && info.DryRun {
		return nil, nil
	}
	event := policy.ErrorEvent(componentType, command)
	if !policy.IsErrorEvent(event) || strings.Contains(string(event), "*") {
		return nil, fmt.Errorf("%w: invalid execution event %q", errUtils.ErrPolicyInputInvalid, event)
	}
	bundle, err := admission.LoadBundle()
	if err != nil || bundle == nil {
		return nil, err
	}
	rules := bundle.RulesFor(event)
	if len(rules) == 0 {
		return nil, nil
	}
	runner := &Runner{event: event, rules: rules, input: policy.ErrorInput{Event: string(event), ComponentType: componentType, Command: strings.TrimSuffix(strings.TrimPrefix(string(event), componentType+"."), ".error")}}
	if err := runner.setIdentity(info); err != nil {
		return nil, err
	}
	return runner, nil
}

func (r *Runner) setIdentity(info *schema.ConfigAndStacksInfo) error {
	if info == nil {
		return nil
	}
	r.input.Component, r.input.Stack = info.ComponentFromArg, info.Stack
	if r.input.Stack == "" {
		r.input.Stack = info.StackFromArg
	}
	subject, err := policy.NewSubject(policy.ObjectIdentity{Name: r.input.Component, Stack: r.input.Stack, ComponentType: r.input.ComponentType}, info.ComponentSection["metadata"])
	r.subject = subject
	return err
}

// Run retries only the supplied invocation. Fallback is the existing component retry
// configuration, used only if no retry policy matches the first failure.
func (r *Runner) Run(ctx context.Context, stage string, fallback *schema.RetryConfig, invoke func() Result) error {
	defer perf.Track(nil, "recovery.Run")()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateFallback(fallback); err != nil {
		return err
	}
	started := time.Now()
	result := invoke()
	if !eligible(ctx, result) {
		return result.Err
	}
	selected, err := r.selectRetry(ctx, stage, 1, result)
	if err != nil {
		return err
	}
	budget, patterns, err := prepareBudget(selected, fallback, result, time.Since(started))
	if err != nil {
		return err
	}
	if budget == nil {
		return result.Err
	}
	session := retrySession{runner: r, stage: stage, selected: selected, patterns: patterns, result: result, invoke: invoke}
	err = retry.WithPredicate(ctx, budget, session.next, func(_ error) bool { return session.shouldRetry(ctx) })
	if session.policyErr != nil {
		return session.policyErr
	}
	return err
}

func validateFallback(config *schema.RetryConfig) error {
	if config == nil || len(config.Conditions) == 0 {
		return nil
	}
	if err := retry.Validate(config); err != nil {
		return errors.Join(errUtils.ErrInvalidConfig, err)
	}
	_, err := retry.CompileConditions(config.Conditions)
	if err != nil {
		return errors.Join(errUtils.ErrInvalidConfig, err)
	}
	return nil
}

func prepareBudget(selected *policy.Rule, fallback *schema.RetryConfig, result Result, elapsed time.Duration) (*schema.RetryConfig, []*regexp.Regexp, error) {
	config := fallback
	if selected != nil {
		config = selected.Effects.Retry
	}
	if config == nil {
		return nil, nil, nil
	}
	patterns, err := retry.CompileConditions(config.Conditions)
	if err != nil {
		return nil, nil, err
	}
	if selected == nil && !retry.MatchesAny(patterns, result.Output+result.Stdout+result.Stderr) {
		return nil, nil, nil
	}
	copyConfig := *config
	if config.MaxElapsedTime != nil {
		remaining := *config.MaxElapsedTime - elapsed
		if remaining <= 0 {
			return nil, nil, nil
		}
		copyConfig.MaxElapsedTime = &remaining
	}
	return &copyConfig, patterns, nil
}

type retrySession struct {
	runner    *Runner
	stage     string
	selected  *policy.Rule
	patterns  []*regexp.Regexp
	result    Result
	invoke    func() Result
	attempt   int
	policyErr error
}

func (s *retrySession) next() error {
	s.attempt++
	if s.attempt > 1 {
		log.Warn("Retrying failed component operation", "event", eventName(s.runner), "stage", s.stage, "attempt", s.attempt)
		s.result = s.invoke()
	}
	return s.result.Err
}

func (s *retrySession) shouldRetry(ctx context.Context) bool {
	if !eligible(ctx, s.result) {
		return false
	}
	// The first failure was evaluated before choosing this invocation's budget.
	if s.attempt == 1 {
		return true
	}
	current, err := s.runner.selectRetry(ctx, s.stage, s.attempt, s.result)
	if err != nil {
		s.policyErr = err
		return false
	}
	if s.selected != nil {
		return current != nil && current.ID == s.selected.ID
	}
	// Never start a second policy budget after legacy retries have begun.
	return current == nil && retry.MatchesAny(s.patterns, s.result.Output+s.result.Stdout+s.result.Stderr)
}

func eligible(ctx context.Context, result Result) bool {
	return result.Err != nil && ctx.Err() == nil &&
		!errors.Is(result.Err, context.Canceled) && !errors.Is(result.Err, context.DeadlineExceeded) &&
		!guard.IsPolicyError(result.Err)
}

func (r *Runner) selectRetry(ctx context.Context, stage string, attempt int, result Result) (*policy.Rule, error) {
	if r == nil {
		return nil, nil
	}
	input := r.errorInput(stage, attempt, result)
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	snapshot, err := ci.CollectPolicySnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: error policy context unavailable for %s", errUtils.ErrPolicyEvaluation, r.event)
	}
	decision, err := admission.NewEngine().Evaluate(ctx, &policy.Request{Kind: r.event, Input: raw, Subject: r.subject, CI: snapshot}, r.rules)
	if err != nil {
		// Expressions may contain sensitive output; do not include evaluator details.
		return nil, fmt.Errorf("%w: error policy evaluation failed for %s", errUtils.ErrPolicyEvaluation, r.event)
	}
	if err := policy.EnforceDecision(decision); err != nil {
		return nil, err
	}
	return r.selectedRetry(&decision), nil
}

func (r *Runner) errorInput(stage string, attempt int, result Result) policy.ErrorInput {
	input := r.input
	input.Stage, input.Attempt, input.ExitCode = stage, attempt, result.ExitCode
	input.Stdout, input.Stderr, input.Output = result.Stdout, result.Stderr, result.Output
	if input.Output == "" {
		input.Output = result.Stdout + result.Stderr
	}
	input.Error.Message = result.Err.Error()
	var coded interface{ ErrorCode() string }
	if errors.As(result.Err, &coded) {
		input.Error.Code = coded.ErrorCode()
	}
	return input
}

func (r *Runner) selectedRetry(decision *policy.Decision) *policy.Rule {
	for i := range decision.Findings {
		finding := &decision.Findings[i]
		if finding.Effects.Retry == nil {
			continue
		}
		for j := range r.rules {
			if r.rules[j].ID == finding.RuleID {
				return &r.rules[j]
			}
		}
	}
	return nil
}

// Call applies operation policies to a single SDK call, retaining its typed result.
// Successful calls are never replayed when a later call or polling iteration fails.
func Call[T any](ctx context.Context, runner *Runner, stage string, fn func() (T, error)) (T, error) {
	defer perf.Track(nil, "recovery.Call")()
	if runner == nil {
		return fn()
	}
	var value T
	err := runner.Run(ctx, stage, nil, func() Result {
		var err error
		value, err = fn()
		return Result{Err: err}
	})
	return value, err
}

// Do is the error-only variant of Call for SDK methods without a result.
//
//nolint:lintroller // Delegates instrumentation to Call.
func Do(ctx context.Context, runner *Runner, stage string, fn func() error) error {
	_, err := Call(ctx, runner, stage, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

func eventName(runner *Runner) policy.InputKind {
	if runner == nil {
		return ""
	}
	return runner.event
}
