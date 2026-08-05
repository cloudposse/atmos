package step

import (
	"context"
	"fmt"

	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	pstore "github.com/cloudposse/atmos/pkg/store"
)

const storeStepType = "store"

// StoreHandler writes a value to a configured Atmos store backend (see pkg/store), making it
// available to later steps via {{ .steps.<name>.value }}/{{ .steps.<name>.metadata.* }} and to
// other workflow or component runs through the existing read-only `!store`/`!store.get` YAML
// functions -- e.g. a build step writes an image tag here, and a later (or entirely separate)
// deploy reads it back. Available as a component lifecycle hook via the existing `kind: step`
// bridge (`kind: step` / `type: store` / `with: {...}`); the pre-existing `kind: store` hook
// (terraform-output-specific) is unrelated and unaffected.
type StoreHandler struct {
	BaseHandler
}

// storeStepConfig is the `with:` shape for a store step, decoded the same way tflint's
// step-specific fields are (see decodeTFLintWith): a generic map round-tripped through YAML into
// a typed struct, rather than adding new flat fields to the already-large schema.WorkflowStep.
type storeStepConfig struct {
	Store     string `yaml:"store"`
	Key       string `yaml:"key"`
	Value     string `yaml:"value"`
	Stack     string `yaml:"stack"`
	Component string `yaml:"component"`
}

// storeValidActions lists the actions a store step accepts. Only "write" exists today; an empty
// action defaults to it, following the archive step's action-defaulting convention.
var storeValidActions = map[string]bool{
	"":      true,
	"write": true,
}

func init() {
	Register(&StoreHandler{
		BaseHandler: NewBaseHandler(storeStepType, CategoryCommand, false),
	})
}

// Validate checks the store step configuration before execution.
func (h *StoreHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.StoreHandler.Validate")()

	if !storeValidActions[step.Action] {
		return errUtils.Build(errUtils.ErrStoreStepInvalidAction).
			WithContext("step", step.Name).
			WithContext("action", step.Action).
			WithHint("Use action: write (the only supported action today)").
			Err()
	}

	stepCfg := storeConfig(step)
	if err := h.ValidateRequired(step, "store", stepCfg.Store); err != nil {
		return err
	}
	if err := h.ValidateRequired(step, "key", stepCfg.Key); err != nil {
		return err
	}
	return h.ValidateRequired(step, "value", stepCfg.Value)
}

// Execute resolves the step's `with:` fields and writes the value to the named store.
func (h *StoreHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.StoreHandler.Execute")()

	if err := h.Validate(step); err != nil {
		return nil, err
	}

	stepCfg := storeConfig(step)

	storeName, err := vars.Resolve(stepCfg.Store)
	if err != nil {
		return nil, fmt.Errorf("step %q: failed to resolve store: %w", step.Name, err)
	}
	key, err := vars.Resolve(stepCfg.Key)
	if err != nil {
		return nil, fmt.Errorf("step %q: failed to resolve key: %w", step.Name, err)
	}
	value, err := vars.Resolve(stepCfg.Value)
	if err != nil {
		return nil, fmt.Errorf("step %q: failed to resolve value: %w", step.Name, err)
	}
	stack, err := resolveStoreStack(&stepCfg, vars)
	if err != nil {
		return nil, fmt.Errorf("step %q: failed to resolve stack: %w", step.Name, err)
	}
	component, err := resolveStoreComponent(&stepCfg, vars)
	if err != nil {
		return nil, fmt.Errorf("step %q: failed to resolve component: %w", step.Name, err)
	}

	s, ok := vars.AtmosConfig.Stores[storeName]
	if !ok {
		return nil, fmt.Errorf("step %q: %w: %s", step.Name, pstore.ErrStoreNotConfigured, storeName)
	}

	if err := s.Set(stack, component, key, value); err != nil {
		return nil, errUtils.Build(errUtils.ErrStoreStepWriteFailed).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("store", storeName).
			Err()
	}

	return NewStepResult(value).
		WithMetadata("store", storeName).
		WithMetadata("key", key).
		WithMetadata("stack", stack).
		WithMetadata("component", component), nil
}

// storeConfig decodes the step's `with:` block and lets top-level `stack`/`component` fields
// (shared with other step types, e.g. tflint) override the `with:` values when set.
func storeConfig(step *schema.WorkflowStep) storeStepConfig {
	if step == nil {
		return storeStepConfig{}
	}
	cfg := decodeStoreWith(step.With)
	if step.Stack != "" {
		cfg.Stack = step.Stack
	}
	if step.Component != "" {
		cfg.Component = step.Component
	}
	return cfg
}

func decodeStoreWith(with map[string]any) storeStepConfig {
	if len(with) == 0 {
		return storeStepConfig{}
	}
	data, err := yaml.Marshal(with)
	if err != nil {
		log.Debug("Failed to marshal store step with: block", "error", err)
		return storeStepConfig{}
	}
	var cfg storeStepConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Debug("Failed to unmarshal store step with: block", "error", err)
		return storeStepConfig{}
	}
	return cfg
}

// resolveStoreStack resolves the target stack: the step's own `stack` (with: or top-level),
// falling back to the workflow's --stack flag/ATMOS_STACK, then empty (a store-global key --
// see pkg/store/providers.getKey, which collapses an empty stack/component cleanly).
func resolveStoreStack(cfg *storeStepConfig, vars *Variables) (string, error) {
	stack := cfg.Stack
	if stack == "" {
		stack = vars.Flags["stack"]
	}
	if stack == "" {
		stack = vars.Env["ATMOS_STACK"]
	}
	return vars.Resolve(stack)
}

// resolveStoreComponent resolves the target component the same way resolveStoreStack resolves
// the stack: step config, then the workflow's --component flag/ATMOS_COMPONENT, then empty.
func resolveStoreComponent(cfg *storeStepConfig, vars *Variables) (string, error) {
	component := cfg.Component
	if component == "" {
		component = vars.Flags["component"]
	}
	if component == "" {
		component = vars.Env["ATMOS_COMPONENT"]
	}
	return vars.Resolve(component)
}
