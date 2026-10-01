package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosYaml "github.com/cloudposse/atmos/pkg/yaml"
)

// ComponentYAMLProcessor processes YAML functions for a specific component.
// It implements the merge.YAMLFunctionProcessor interface.
type ComponentYAMLProcessor struct {
	atmosConfig   *schema.AtmosConfiguration
	currentStack  string
	skip          []string
	resolutionCtx *ResolutionContext
	stackInfo     *schema.ConfigAndStacksInfo
}

// NewComponentYAMLProcessor creates a new YAML processor for component merging.
func NewComponentYAMLProcessor(
	atmosConfig *schema.AtmosConfiguration,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) m.YAMLFunctionProcessor {
	defer perf.Track(atmosConfig, "exec.NewComponentYAMLProcessor")()

	return &ComponentYAMLProcessor{
		atmosConfig:   atmosConfig,
		currentStack:  currentStack,
		skip:          skip,
		resolutionCtx: resolutionCtx,
		stackInfo:     stackInfo,
	}
}

// ProcessYAMLFunctionString processes a YAML function string and returns the processed value.
// This method recursively processes the string to handle all YAML function types:
// - !terraform.output: Terraform output from other components
// - !terraform.state: Terraform state queries
// - !store.get, !store: Store lookups
// - !exec: Command execution
// - !env: Environment variable expansion
// - !labels, !tags, !labels.keys, !labels.values: component metadata lookups.
//
// It does NOT Go-template-render a deferred !template string's {{ }} expressions — see
// TemplateAwareYAMLProcessor for that. This type is also used outside merge-time deferred
// resolution (e.g. pkg/hooks resolves hook-execution strings through it), where that whole-document
// template pass has already run.
func (p *ComponentYAMLProcessor) ProcessYAMLFunctionString(value string) (any, error) {
	defer perf.Track(p.atmosConfig, "exec.ComponentYAMLProcessor.ProcessYAMLFunctionString")()

	// Process the YAML function using existing function processors.
	// processCustomTagsWithContext handles all YAML function types and skip list.
	return processCustomTagsWithContext(
		p.atmosConfig,
		value,
		p.currentStack,
		p.skip,
		p.resolutionCtx,
		p.stackInfo,
	)
}

// TemplateAwareYAMLProcessor wraps ComponentYAMLProcessor to Go-template-render a deferred YAML
// function string's {{ }} expressions before delegating to the normal tag dispatch.
//
// This isn't only about !template's own argument — ANY deferred function's argument can contain a
// {{ }} placeholder (e.g. `!terraform.state component-1 {{ .stack }} baz`, parametrizing which
// stack to query). In the normal, non-deferred path, the whole-document ProcessTmplWithDatasources
// pass renders {{ }} everywhere in the document BEFORE any YAML tag is dispatched, so by the time
// a function tag like !terraform.state runs, its {{ .stack }} has already become a literal value.
// WalkAndDeferYAMLFunctions pulls a deferred value out of the document (replacing it with a
// placeholder) before that whole-document pass ever sees it, so nothing else ever renders its
// {{ }} — this type exists to do that rendering, generically, for whichever function the value
// turns out to be for, not just !template.
type TemplateAwareYAMLProcessor struct {
	inner                    m.YAMLFunctionProcessor
	atmosConfig              *schema.AtmosConfiguration
	configAndStacksInfo      *schema.ConfigAndStacksInfo
	settingsSection          schema.Settings
	componentTemplateContext map[string]any
}

// TemplateAwareYAMLProcessorOptions configures NewTemplateAwareYAMLProcessor.
//
// ComponentTemplateContext must be the same context built for the whole-document template pass
// (internal/exec/utils.go, around the ProcessTmplWithDatasources call in ProcessStacks) — it may
// be nil if template processing was disabled for this invocation (processTemplates=false); in that
// case, resolving a deferred value containing {{ }} fails loudly
// (errUtils.ErrDeferredTemplateContextMissing) rather than silently skipping it.
type TemplateAwareYAMLProcessorOptions struct {
	AtmosConfig              *schema.AtmosConfiguration
	ConfigAndStacksInfo      *schema.ConfigAndStacksInfo
	SettingsSection          schema.Settings
	ComponentTemplateContext map[string]any
	Skip                     []string
	ResolutionCtx            *ResolutionContext
}

// NewTemplateAwareYAMLProcessor creates a new YAML processor for resolving deferred YAML
// functions, correctly Go-template-rendering any {{ }} expression in their argument along the way.
func NewTemplateAwareYAMLProcessor(opts *TemplateAwareYAMLProcessorOptions) m.YAMLFunctionProcessor {
	defer perf.Track(opts.AtmosConfig, "exec.NewTemplateAwareYAMLProcessor")()

	inner := NewComponentYAMLProcessor(opts.AtmosConfig, opts.ConfigAndStacksInfo.Stack, opts.Skip, opts.ResolutionCtx, opts.ConfigAndStacksInfo)

	return &TemplateAwareYAMLProcessor{
		inner:                    inner,
		atmosConfig:              opts.AtmosConfig,
		configAndStacksInfo:      opts.ConfigAndStacksInfo,
		settingsSection:          opts.SettingsSection,
		componentTemplateContext: opts.ComponentTemplateContext,
	}
}

// ProcessYAMLFunctionString implements merge.YAMLFunctionProcessor.
func (p *TemplateAwareYAMLProcessor) ProcessYAMLFunctionString(value string) (any, error) {
	defer perf.Track(p.atmosConfig, "exec.TemplateAwareYAMLProcessor.ProcessYAMLFunctionString")()

	// Fast path: skip the template engine call entirely when there's no Go-template expression to
	// render (the common case: most deferred function arguments are static). This runs once per
	// deferred string rather than once per whole document, so avoiding the render call when
	// it can't possibly change anything matters for large stacks.
	leftDelimiter := "{{"
	if d := p.atmosConfig.Templates.Settings.Delimiters; len(d) == 2 && d[0] != "" {
		leftDelimiter = d[0]
	}
	if !strings.Contains(value, leftDelimiter) {
		return p.inner.ProcessYAMLFunctionString(value)
	}

	if p.componentTemplateContext == nil {
		return nil, fmt.Errorf("%w: %q", errUtils.ErrDeferredTemplateContextMissing, value)
	}

	// ProcessTmplWithDatasources is built for the whole-document pass: after rendering, it
	// unmarshals its own output as YAML expecting a map (to extract a possible
	// settings.templates.settings override for the next evaluation pass) and errors if that
	// unmarshal fails. A deferred value on its own is a scalar (e.g. `!template '{{ .foo }}'`),
	// which fails that internal map-unmarshal. Wrap it in a single-key map for the call, then
	// unwrap the rendered result ourselves — mirroring how the whole-document pass encodes its
	// input (utils.go's ProcessStacks, componentSectionStr) to survive the same round trip.
	const deferredValueWrapperKey = "value"
	wrapped, err := atmosYaml.ConvertToYAMLPreservingDelimiters(
		map[string]any{deferredValueWrapperKey: value},
		p.atmosConfig.Templates.Settings.Delimiters,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to prepare deferred YAML function value for rendering: %w", errUtils.ErrRenderTemplate, err)
	}

	renderedWrapped, err := ProcessTmplWithDatasources(
		p.atmosConfig,
		p.configAndStacksInfo,
		p.settingsSection,
		"deferred-yaml-function-value",
		wrapped,
		p.componentTemplateContext,
		false, // Fail loudly on a render error instead of silently returning unrendered text.
	)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to render deferred YAML function value: %w", errUtils.ErrRenderTemplate, err)
	}

	renderedMap, err := u.UnmarshalYAML[schema.AtmosSectionMapType](renderedWrapped)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse rendered deferred YAML function value: %w", errUtils.ErrRenderTemplate, err)
	}

	rendered, ok := renderedMap[deferredValueWrapperKey].(string)
	if !ok {
		return nil, fmt.Errorf("%w: rendering %q did not produce a string value", errUtils.ErrRenderTemplate, value)
	}

	return p.inner.ProcessYAMLFunctionString(rendered)
}
