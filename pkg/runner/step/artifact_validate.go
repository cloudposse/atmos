package step

import (
	"fmt"
	"reflect"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Validate checks the artifact step configuration before execution. It rejects an unknown action,
// a field the action does not accept, a missing required field, and a malformed `source` or
// `labels`, all from the authored step alone, so an invalid combination fails before any work.
func (h *ArtifactHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ArtifactHandler.Validate")()

	spec, err := artifactSpec(step)
	if err != nil {
		return err
	}
	if _, err := artifactSourceString(step); err != nil {
		return err
	}
	if len(step.Query) > 0 {
		return artifactInvalidField(step, "query", "the query mapping belongs to the http step; an artifact step takes the search text as a string")
	}
	if err := validateArtifactLabelShape(step); err != nil {
		return err
	}
	set := artifactFieldsSet(step)
	for _, field := range invoke.AllFields() {
		if set[field] && !spec.Accepts(field) {
			return errUtils.Build(fmt.Errorf("%w: `%s` for action `%s`", errUtils.ErrArtifactStepFieldForbidden, field, step.Action)).
				WithContext("step", step.Name).
				WithContext("action", step.Action).
				WithContext("field", string(field)).
				WithExplanationf("Field `%s` is not valid for artifact action `%s`", field, step.Action).
				WithHintf("Fields for `%s`: %s", step.Action, artifactFieldList(spec)).
				Err()
		}
	}
	for _, field := range spec.Required {
		if !set[field] {
			return errUtils.Build(fmt.Errorf("%w: `%s` for action `%s`", errUtils.ErrArtifactStepFieldRequired, field, step.Action)).
				WithContext("step", step.Name).
				WithContext("action", step.Action).
				WithContext("field", string(field)).
				WithExplanationf("Step `%s` (action `%s`) is missing required field `%s`", step.Name, step.Action, field).
				WithHintf("Fields for `%s`: %s", step.Action, artifactFieldList(spec)).
				Err()
		}
	}
	return nil
}

// artifactSpec returns the input contract of the step's action, or a typed error naming the valid
// actions when the action is missing or unknown.
func artifactSpec(step *schema.WorkflowStep) (invoke.Spec, error) {
	action := artifact.Action(step.Action)
	spec, ok := invoke.SpecFor(action)
	if ok {
		return spec, nil
	}
	valid := make([]string, 0, len(invoke.Actions()))
	for _, candidate := range invoke.Actions() {
		valid = append(valid, string(candidate))
	}
	return invoke.Spec{}, errUtils.Build(fmt.Errorf("%w: %q", errUtils.ErrArtifactStepInvalidAction, step.Action)).
		WithContext("step", step.Name).
		WithContext("action", step.Action).
		WithExplanationf("Artifact step `%s` has action %q, which is not an artifact operation", step.Name, step.Action).
		WithHintf("Use one of: %s", strings.Join(valid, ", ")).
		Err()
}

// artifactFieldList renders the fields an action accepts, required fields first.
func artifactFieldList(spec invoke.Spec) string {
	names := make([]string, 0, len(spec.Required)+len(spec.Optional))
	for _, field := range spec.Required {
		names = append(names, string(field)+" (required)")
	}
	for _, field := range spec.Optional {
		names = append(names, string(field))
	}
	return strings.Join(names, ", ")
}

// artifactSourceString reads step.Source as a plain string. The field is shared with the workdir
// step, which also accepts a source map; an artifact step requires a plain string path. An unset
// source returns the empty string and no error.
func artifactSourceString(step *schema.WorkflowStep) (string, error) {
	if step.Source == nil {
		return "", nil
	}
	source, ok := step.Source.(string)
	if !ok {
		return "", errUtils.Build(fmt.Errorf("%w, got %T", errUtils.ErrArtifactStepInvalidSource, step.Source)).
			WithContext("step", step.Name).
			WithContext("type", fmt.Sprintf("%T", step.Source)).
			WithHint("Set `source` to a plain string path").
			Err()
	}
	return source, nil
}

// artifactInvalidField builds the error for a field with the wrong shape or value.
func artifactInvalidField(step *schema.WorkflowStep, field, detail string) error {
	return errUtils.Build(fmt.Errorf("%w: `%s`", errUtils.ErrArtifactStepInvalidField, field)).
		WithContext("step", step.Name).
		WithContext("field", field).
		WithExplanationf("Artifact step `%s` field `%s` is invalid: %s", step.Name, field, detail).
		Err()
}

// validateArtifactLabelShape checks that `labels` is a string, a list of strings, or a mapping.
func validateArtifactLabelShape(step *schema.WorkflowStep) error {
	if step.Labels == nil {
		return nil
	}
	switch reflect.ValueOf(step.Labels).Kind() {
	case reflect.String, reflect.Map:
		return nil
	case reflect.Slice:
		for _, item := range reflect.ValueOf(step.Labels).Seq2() {
			if item.Kind() == reflect.Interface {
				item = item.Elem()
			}
			if item.Kind() != reflect.String {
				return artifactInvalidField(step, "labels", "list items must be key=value strings")
			}
		}
		return nil
	default:
		return artifactInvalidField(step, "labels", "use a key=value string, a list of them, or a mapping")
	}
}

// artifactFieldsSet reports which operation inputs the authored step sets. A boolean counts as set
// only when true, because an absent boolean cannot be told from `false`.
func artifactFieldsSet(step *schema.WorkflowStep) map[invoke.Field]bool {
	return map[invoke.Field]bool{
		invoke.FieldSource:       step.Source != nil,
		invoke.FieldRepository:   step.Repository != "",
		invoke.FieldArtifactName: step.ArtifactName != "",
		invoke.FieldVersion:      step.Version != "",
		invoke.FieldArtifactKind: step.ArtifactKind != "",
		invoke.FieldForce:        step.Force,
		invoke.FieldDryRun:       step.ArtifactDryRun,
		invoke.FieldMetadata:     step.Metadata != nil,
		invoke.FieldLabels:       step.Labels != nil,
		invoke.FieldReadme:       step.Readme != "",
		invoke.FieldChangelog:    step.Changelog != "",
		invoke.FieldRef:          step.Ref != "",
		invoke.FieldDestination:  step.Destination != "",
		invoke.FieldFrom:         step.From != "",
		invoke.FieldTo:           step.To != "",
		invoke.FieldDocument:     step.Document != "",
		invoke.FieldQuery:        step.SearchQuery != "",
		invoke.FieldRefresh:      step.Refresh,
	}
}
