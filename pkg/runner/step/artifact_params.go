package step

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

// artifactFieldResolver renders the templated string fields of one artifact step. It remembers the
// first failure so a long run of field renders reads as straight-line code.
type artifactFieldResolver struct {
	handler *ArtifactHandler
	step    *schema.WorkflowStep
	vars    *Variables
	err     error
}

// text renders a templated field. An empty value stays empty.
func (r *artifactFieldResolver) text(field, value string) string {
	if r.err != nil || value == "" {
		return ""
	}
	resolved, err := r.vars.ResolveStepField(r.step, field, value)
	if err != nil {
		r.err = r.vars.TemplateFieldError(r.step, field, err)
		return ""
	}
	return resolved
}

// path renders a templated path field and anchors a relative result to the step's working
// directory, using the same helper as the archive step, so a relative source or destination means
// the same thing in a workflow, a custom command, and a hook.
func (r *artifactFieldResolver) path(field, value string) string {
	if r.err != nil || value == "" {
		return ""
	}
	resolved, err := r.handler.ResolveInWorkingDirectory(r.step, r.vars, value, field)
	if err != nil {
		r.err = err
		return ""
	}
	return resolved
}

// artifactParams resolves the authored step into the transport-neutral operation parameters that
// the CLI also builds. Templates are rendered here, `source` and `destination` are anchored to the
// step's working directory, and the document paths stay relative to the publish source, exactly as
// `--readme` and `--changelog` do on the command line.
func (h *ArtifactHandler) artifactParams(step *schema.WorkflowStep, vars *Variables) (*invoke.Params, error) {
	source, err := artifactSourceString(step)
	if err != nil {
		return nil, err
	}
	resolver := &artifactFieldResolver{handler: h, step: step, vars: vars}
	params := &invoke.Params{
		Action:      artifact.Action(step.Action),
		Source:      resolver.path("source", source),
		Name:        resolver.text("artifact_name", step.ArtifactName),
		Version:     resolver.text("version", step.Version),
		Kind:        resolver.text("artifact_kind", step.ArtifactKind),
		Force:       step.Force,
		DryRun:      step.ArtifactDryRun,
		Refresh:     step.Refresh,
		Readme:      resolver.text("readme", step.Readme),
		Changelog:   resolver.text("changelog", step.Changelog),
		Ref:         resolver.text("ref", step.Ref),
		Destination: resolver.path("destination", step.Destination),
		From:        resolver.text("from", step.From),
		To:          resolver.text("to", step.To),
		Document:    resolver.text("document", step.Document),
		Query:       resolver.text("query", step.SearchQuery),
	}
	repository := resolver.text("repository", step.Repository)
	if resolver.err != nil {
		return nil, resolver.err
	}
	if params.Action == artifact.ActionList || params.Action == artifact.ActionSearch {
		params.Repositories = splitRepositories(repository)
	} else {
		params.Repository = repository
	}
	if params.Labels, err = resolveArtifactLabels(step, resolver); err != nil {
		return nil, err
	}
	return params, resolver.err
}

// splitRepositories splits the comma-separated repository selector of list and search, the same
// way the repeatable `--repository` flag splits its values.
func splitRepositories(value string) []string {
	var repositories []string
	for _, alias := range strings.Split(value, ",") {
		if alias = strings.TrimSpace(alias); alias != "" {
			repositories = append(repositories, alias)
		}
	}
	return repositories
}

// resolveArtifactLabels returns the publish metadata labels, or the list and search selectors. The
// action's field matrix guarantees that at most one of the two forms is set.
func resolveArtifactLabels(step *schema.WorkflowStep, resolver *artifactFieldResolver) (map[string]string, error) {
	if step.Metadata != nil && len(step.Metadata.Labels) > 0 {
		labels := make(map[string]string, len(step.Metadata.Labels))
		for key, value := range step.Metadata.Labels {
			labels[key] = resolver.text("metadata.labels."+key, value)
		}
		return labels, resolver.err
	}
	if step.Labels == nil {
		return nil, nil
	}
	return parseArtifactSelectors(step, resolver)
}

// parseArtifactSelectors accepts the three shapes of `labels`: a comma-separated key=value string,
// a list of them, or a mapping. The string forms use the CLI's exact key/value parser.
func parseArtifactSelectors(step *schema.WorkflowStep, resolver *artifactFieldResolver) (map[string]string, error) {
	value := reflect.ValueOf(step.Labels)
	if value.Kind() == reflect.Map {
		labels := resolveLabelMapping(value, resolver)
		if resolver.err != nil {
			return nil, resolver.err
		}
		return labels, artifact.ValidateLabels(labels)
	}
	var pairs []string
	switch value.Kind() {
	case reflect.String:
		pairs = append(pairs, value.String())
	case reflect.Slice:
		pairs = labelListItems(value)
	default:
		return nil, artifactInvalidField(step, "labels", "use a key=value string, a list of them, or a mapping")
	}
	rendered := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		rendered = append(rendered, resolver.text("labels", pair))
	}
	if resolver.err != nil {
		return nil, resolver.err
	}
	return tags.ParseLabelsFlagFrom(strings.Join(rendered, ","), "the `labels` field of artifact step "+step.Name)
}

// labelListItems returns the items of a label list as strings.
func labelListItems(list reflect.Value) []string {
	items := make([]string, 0, list.Len())
	for index := range list.Len() {
		items = append(items, unwrapInterface(list.Index(index)).String())
	}
	return items
}

// resolveLabelMapping renders the values of a label mapping as templates.
func resolveLabelMapping(mapping reflect.Value, resolver *artifactFieldResolver) map[string]string {
	labels := make(map[string]string, mapping.Len())
	for _, key := range mapping.MapKeys() {
		entry := unwrapInterface(mapping.MapIndex(key))
		labels[fmt.Sprint(key.Interface())] = resolver.text("labels", fmt.Sprint(entry.Interface()))
	}
	return labels
}

// unwrapInterface returns the concrete value held by an interface-typed element.
func unwrapInterface(value reflect.Value) reflect.Value {
	if value.Kind() == reflect.Interface {
		return value.Elem()
	}
	return value
}

// checkResolvedArtifactParams verifies, after templates are rendered, that every required input is
// still present. A template that renders to an empty string must fail here, with the field named,
// rather than deep inside the service.
func checkResolvedArtifactParams(step *schema.WorkflowStep, params *invoke.Params) error {
	spec, err := artifactSpec(step)
	if err != nil {
		return err
	}
	present := map[invoke.Field]bool{
		invoke.FieldSource:      params.Source != "",
		invoke.FieldRepository:  params.Repository != "",
		invoke.FieldRef:         params.Ref != "",
		invoke.FieldDestination: params.Destination != "",
		invoke.FieldFrom:        params.From != "",
		invoke.FieldTo:          params.To != "",
		invoke.FieldDocument:    params.Document != "",
	}
	var missing []string
	for _, field := range spec.Required {
		if known, tracked := present[field]; tracked && !known {
			missing = append(missing, string(field))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return errUtils.Build(fmt.Errorf("%w: %s", errUtils.ErrArtifactStepFieldRequired, strings.Join(missing, ", "))).
		WithContext("step", step.Name).
		WithContext("action", step.Action).
		WithContext("fields", strings.Join(missing, ", ")).
		WithExplanationf("Step `%s` (action `%s`) rendered an empty value for required field(s): %s", step.Name, step.Action, strings.Join(missing, ", ")).
		WithHint("Check the templates in those fields; each must render to a non-empty value").
		Err()
}
