// Package admission enforces policy before execution, outside pure evaluators.
// Loading its input never expands Atmos configuration or invokes hooks.
package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

var invocation struct {
	sync.RWMutex
	active bool
	bundle *policy.Bundle
}

// BeginInvocation pins governing definitions before configuration can change the
// working directory or environment. The CLI serializes top-level invocations.
func BeginInvocation() (func(), error) {
	defer perf.Track(nil, "admission.BeginInvocation")()
	guard.ResetOrigins()
	bundle, err := loadDirectory()
	if err != nil {
		return nil, err
	}
	invocation.Lock()
	previous, active := invocation.bundle, invocation.active
	invocation.bundle, invocation.active = bundle, true
	invocation.Unlock()
	return func() {
		invocation.Lock()
		invocation.bundle, invocation.active = previous, active
		invocation.Unlock()
	}, nil
}

func loadDirectory() (*policy.Bundle, error) {
	directory, err := policy.DirectoryFromEnvironment()
	if err != nil || directory == "" {
		return nil, err
	}
	return policy.LoadDirectory(directory)
}

// LoadBundle returns governing definitions pinned for this invocation. Callers
// must treat them as immutable. Outside the CLI, it loads the configured directory.
func LoadBundle() (*policy.Bundle, error) {
	defer perf.Track(nil, "admission.LoadBundle")()
	invocation.RLock()
	bundle, active := invocation.bundle, invocation.active
	invocation.RUnlock()
	if active {
		return bundle, nil
	}
	return loadDirectory()
}

// HasRules avoids collecting instance metadata for an unconfigured capability.
func HasRules(kind policy.InputKind) (bool, error) {
	defer perf.Track(nil, "admission.HasRules")()
	bundle, err := LoadBundle()
	return bundle != nil && len(bundle.RulesFor(kind)) > 0, err
}

// Check loads protected local rules and evaluates one explicitly available capability.
// An absent directory is opt-in behavior, not evidence of a trusted configuration.
func Check(ctx context.Context, request *policy.Request) error {
	defer perf.Track(nil, "admission.Check")()
	if request == nil {
		return errUtils.ErrPolicyInputInvalid
	}
	bundle, err := LoadBundle()
	if err != nil || bundle == nil {
		if err == nil {
			policy.Explain(request.Kind, "", "gate reached; no configured rules")
		}
		return err
	}
	rules := bundle.RulesFor(request.Kind)
	if len(rules) == 0 {
		policy.Explain(request.Kind, "", "gate reached; no configured rules")
		return nil
	}
	request.CI, err = ci.CollectPolicySnapshot(ctx)
	if err != nil {
		return err
	}
	decision, err := policy.NewEngine().Evaluate(ctx, request, rules)
	if err != nil {
		return fmt.Errorf("%w: admission evaluation failed (input details withheld)", errUtils.ErrPolicyEvaluation)
	}
	decision, err = ci.ResolvePolicyApprovals(ctx, request, decision)
	if err != nil {
		return err
	}
	return policy.EnforceDecision(decision)
}

// Function checks the canonical tag name, not its arguments (which can be secrets).
// No subject is fabricated before component metadata is available.
func Function(ctx context.Context, name string) error {
	defer perf.Track(nil, "admission.Function")()
	return guard.Function(ctx, guard.CanonicalFunction(name), "yaml", guard.Origin{Provenance: "unknown"})
}

func init() {
	guard.SetEnabled(func(kind string) (bool, error) { return HasRules(policy.InputKind(kind)) })
	guard.SetEvaluator(func(ctx context.Context, kind string, input json.RawMessage) error {
		return Check(ctx, &policy.Request{Kind: policy.InputKind(kind), Input: input})
	})
}

// FunctionAt checks a YAML function with its parser-provided source file.
func FunctionAt(ctx context.Context, name, file string) error {
	defer perf.Track(nil, "admission.FunctionAt")()

	return guard.FunctionAt(ctx, name, file)
}

// FunctionValue checks lazy YAML functions using conservative parse-time attribution.
func FunctionValue(ctx context.Context, name, value string) error {
	defer perf.Track(nil, "admission.FunctionValue")()

	return guard.FunctionValue(ctx, name, value)
}
