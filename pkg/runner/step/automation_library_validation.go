package step

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// automationCommonFields are the step fields every step type accepts in a direct step call.
var automationCommonFields = []string{"name", "type", "output", "show", "env", "working_directory", "timeout", "retry", "outputs", "with"}

// automationPolicyFields are the scheduler-policy fields and the container override. Field validation lets them through so
// validateAutomationStep can reject them with a message that explains where they are supported.
var automationPolicyFields = []string{"needs", "when", "continue", "identity", "background", "inputs", "artifacts", "preconditions", "container"}

// Rejects fields the step type does not read. The stepType argument is the type of the step the
// node describes; when it is empty, the node's own `type` key names it. A handler that declares
// its fields (see knownFieldsHandler) is checked against the common fields plus its own; every
// other handler is checked against the fields of any step type.
func validateAutomationStepFields(node *yaml.Node, stepType string) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: step configuration must be a dictionary", errUtils.ErrAutomation)
	}
	if stepType == "" {
		stepType = mappingScalar(node, "type")
	}
	fields, scoped := automationFieldsFor(stepType)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		if !fields[key] && (!scoped || !slices.Contains(automationPolicyFields, key)) {
			return unknownAutomationFieldError(key, stepType, fields, scoped)
		}
		if err := validateAutomationChildFields(key, value); err != nil {
			return err
		}
	}
	return nil
}

// validateAutomationChildFields validates the fields of every nested step.
func validateAutomationChildFields(key string, node *yaml.Node) error {
	if key != "steps" {
		return nil
	}
	for _, child := range node.Content {
		if err := validateAutomationStepFields(child, ""); err != nil {
			return err
		}
	}
	return nil
}

// unknownAutomationFieldError reports a field the step type does not accept.
func unknownAutomationFieldError(key, stepType string, fields map[string]bool, scoped bool) error {
	if scoped {
		return fmt.Errorf("%w: unknown field %q for step type %q; valid fields: %s", errUtils.ErrAutomation, key, stepType, sortedKeys(fields))
	}
	return fmt.Errorf("%w: unknown step field %q", errUtils.ErrAutomation, key)
}

// automationFieldsFor returns the fields a direct step call of stepType may set, and whether they
// are scoped to the type (true) or the union of every step type's fields (false).
func automationFieldsFor(stepType string) (map[string]bool, bool) {
	if handler, ok := Get(stepType); ok {
		if declared, ok := handler.(knownFieldsHandler); ok {
			fields := make(map[string]bool)
			for _, name := range automationCommonFields {
				fields[name] = true
			}
			for _, name := range declared.KnownFields() {
				fields[name] = true
			}
			return fields, true
		}
	}
	return automationStepFields(), false
}

// mappingScalar returns the scalar value of key in a mapping node, or "".
func mappingScalar(node *yaml.Node, key string) string {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1].Value
		}
	}
	return ""
}

func sortedKeys(set map[string]bool) string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// withCaptureHint explains where output capture lives when a direct step call asks a step for an
// exec.run mode. Step `output` controls how a step shows what it produced (raw, log, viewport,
// none); capturing a command's output is exec.run(output="capture"), and every step result already
// carries the output in its value.
func withCaptureHint(step *schema.WorkflowStep, err error) error {
	if err == nil || !errors.Is(err, errUtils.ErrStepInvalidOutputMode) {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(step.Output)) {
	case "capture", "stream":
		return errUtils.Build(err).
			WithHintf("Use exec.run(argv, output=\"capture\") to capture a command's output; step output modes are %s.", strings.Join(schema.StepOutputModes(), ", ")).
			Err()
	default:
		return err
	}
}

func validateAutomationStep(step *schema.WorkflowStep, parallel bool) error {
	if step.LoadError != nil {
		return loadErrorWithValidation(step)
	}
	handler, ok := Get(step.Type)
	if !ok {
		return fmt.Errorf("%w: %s", errUtils.ErrUnknownStepType, step.Type)
	}
	// Scheduler policies require the enclosing workflow runner. Never accept them
	// silently in a synchronous function call where they would be ignored.
	if hasWorkflowPolicies(step) {
		return fmt.Errorf("%w: %s %s not supported in a direct step call or a Git hook; needs, when, continue, identity, background execution, and freshness policies (inputs, artifacts, preconditions) are supported on steps in workflows and lifecycle hooks",
			errUtils.ErrAutomation, strings.Join(presentWorkflowPolicies(step), ", "), policyVerb(step))
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

// presentWorkflowPolicies names the scheduler-policy fields the step actually sets.
func presentWorkflowPolicies(step *schema.WorkflowStep) []string {
	var present []string
	add := func(set bool, name string) {
		if set {
			present = append(present, name)
		}
	}
	add(len(step.Needs) > 0, "needs")
	add(!step.When.IsZero(), "when")
	add(!step.Continue.IsZero(), "continue")
	add(step.Identity != "", "identity")
	add(step.BackgroundAsync, "background")
	add(step.Inputs != nil, "inputs")
	add(step.Artifacts != nil, "artifacts")
	add(step.Preconditions != nil, "preconditions")
	return present
}

func policyVerb(step *schema.WorkflowStep) string {
	if len(presentWorkflowPolicies(step)) > 1 {
		return "are"
	}
	return "is"
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
