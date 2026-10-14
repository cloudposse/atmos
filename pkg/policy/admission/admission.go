// Package admission enforces policy before execution, outside pure evaluators.
// Loading its input never expands Atmos configuration or invokes hooks.
package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
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
		return err
	}
	rules := bundle.RulesFor(request.Kind)
	if len(rules) == 0 {
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
	name = strings.TrimPrefix(strings.ToLower(name), "!")
	switch name {
	case "repo-root":
		name = "git.root"
	case "git.ref":
		name = "git.sha"
	}
	data, err := json.Marshal(map[string]any{"name": name})
	if err != nil {
		return err
	}
	return Check(ctx, &policy.Request{Kind: policy.Function, Input: data})
}
