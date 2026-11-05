package secret

import (
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// newSelectorEvaluator returns the evaluator that lazily resolves a declaration's backend selector
// (for example `store: !aws.cloudformation.output <producer> <stack> <Output>`) for one declared
// secret. Each call describes the component with evaluation narrowed to exactly the requested
// field, authenticated like every other secret command, so a selector is evaluated only when a
// command actually uses its declaration and an unresolvable one never affects its siblings.
func newSelectorEvaluator(atmosConfig *schema.AtmosConfiguration, authManager auth.AuthManager, scope secretScope) secrets.SelectorEvaluator {
	return func(path []string, _ any) (any, error) {
		defer perf.Track(atmosConfig, "secret.selectorEvaluator")()

		section, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
			AtmosConfig:          atmosConfig,
			Component:            scope.Component,
			Stack:                scope.Stack,
			ComponentType:        scope.ComponentType,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			// Only CloudFormation output selectors are evaluated; secret values and unrelated
			// state/store reads stay deferred.
			Skip:         authenticatedSecretSkip(),
			AuthManager:  authManager,
			ErrorOptions: e.DescribeStacksErrorOptions{EvaluationPaths: [][]string{path}},
		})
		if err != nil {
			return nil, err
		}
		return valueAtPath(section, path)
	}
}

// sectionComponentType returns the component type recorded in a described component section, or
// an empty string when the section does not carry one.
func sectionComponentType(section map[string]any) string {
	if t, ok := section[cfg.ComponentTypeSectionName].(string); ok && t != "" {
		return t
	}
	if info, ok := section["component_info"].(map[string]any); ok {
		if t, ok := info[cfg.ComponentTypeSectionName].(string); ok {
			return t
		}
	}
	return ""
}

// valueAtPath walks a described section along path and returns the value found there.
func valueAtPath(section map[string]any, path []string) (any, error) {
	var current any = section
	for _, key := range path {
		node, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: %v", errUtils.ErrInvalidConfig, path)
		}
		current, ok = node[key]
		if !ok {
			return nil, fmt.Errorf("%w: %v", errUtils.ErrInvalidConfig, path)
		}
	}
	return current, nil
}

// newLazySelectorEvaluator is the evaluator for a scope whose configuration and authentication have
// not been loaded yet (the stack-wide SOPS collision check enumerates instances credential-free).
// Config and auth are built on the first selector evaluation, so instances without selectors never
// authenticate.
func newLazySelectorEvaluator(scope secretScope) secrets.SelectorEvaluator {
	var (
		once    sync.Once
		inner   secrets.SelectorEvaluator
		initErr error
	)
	return func(path []string, raw any) (any, error) {
		once.Do(func() {
			atmosConfig, authManager, err := loadConfigAndAuth(scope)
			if err != nil {
				initErr = err
				return
			}
			inner = newSelectorEvaluator(atmosConfig, authManager, scope)
		})
		if initErr != nil {
			return nil, initErr
		}
		return inner(path, raw)
	}
}

// loadConfigAndAuth initializes config and the authenticated manager for a scope and wires the
// identity-aware store/SOPS resolvers, exactly as loadServiceAndConfig does.
func loadConfigAndAuth(scope secretScope) (*schema.AtmosConfiguration, auth.AuthManager, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: scope.Component,
		Stack:            scope.Stack,
	}, true)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitConfig, err)
	}
	authManager, err := buildAuthManager(&atmosConfig, scope)
	if err != nil {
		return nil, nil, err
	}
	injectSecretStoreAuthResolver(&atmosConfig, authManager, scope)
	return &atmosConfig, authManager, nil
}

// entrySelectorEvaluatorFn is a seam so tests can supply a fake selector evaluator for the
// stack-wide collision check without real stack processing or authentication.
var entrySelectorEvaluatorFn = newLazySelectorEvaluator
