package step

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func validateAutomationStepFields(node *yaml.Node) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: step configuration must be a dictionary", errUtils.ErrAutomation)
	}
	fields := automationStepFields()
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		if !fields[key] {
			return fmt.Errorf("%w: unknown step field %q", errUtils.ErrAutomation, key)
		}
		if key == "steps" {
			for _, child := range value.Content {
				if err := validateAutomationStepFields(child); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateAutomationStep(step *schema.WorkflowStep, parallel bool) error {
	handler, ok := Get(step.Type)
	if !ok {
		return fmt.Errorf("%w: %s", errUtils.ErrUnknownStepType, step.Type)
	}
	// Scheduler policies require the enclosing workflow runner. Never accept them
	// silently in a synchronous function call where they would be ignored.
	if hasWorkflowPolicies(step) {
		return fmt.Errorf("%w: needs, when, continue, identity, background execution, and freshness policies belong on enclosing YAML steps", errUtils.ErrAutomation)
	}
	if step.Type != containerStepType && step.Container.IsEnabled() {
		return fmt.Errorf("%w: container overrides belong on enclosing YAML steps; use steps.container for container operations", errUtils.ErrAutomation)
	}
	if parallel && requiresExclusiveStep(step, handler) {
		return fmt.Errorf("%w: step %q cannot run in a parallel script task because it requires exclusive terminal or process access", errUtils.ErrAutomation, step.Type)
	}
	return handler.Validate(step)
}

func automationStepFields() map[string]bool {
	fields := map[string]bool{"with": true, "background": true, "for": true}
	kind := reflect.TypeFor[schema.WorkflowStep]()
	for i := range kind.NumField() {
		name := strings.Split(kind.Field(i).Tag.Get("yaml"), ",")[0]
		if name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}

func hasWorkflowPolicies(step *schema.WorkflowStep) bool {
	return len(step.Needs) > 0 || !step.When.IsZero() || !step.Continue.IsZero() || step.Identity != "" || step.BackgroundAsync || step.Inputs != nil || step.Artifacts != nil || step.Preconditions != nil
}

func requiresExclusiveStep(step *schema.WorkflowStep, handler StepHandler) bool {
	switch handler.GetName() {
	case schema.TaskTypeExec, "cast", "clear":
		return true
	}
	return handler.RequiresTTY() || step.Interactive || step.Tty || step.Output == "viewport"
}
