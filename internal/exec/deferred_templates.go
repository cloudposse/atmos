package exec

import (
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosYaml "github.com/cloudposse/atmos/pkg/yaml"
)

// Retry a failed list/describe template at scalar boundaries, retaining the original
// component context. Successful siblings survive a credential failure in one value.
type deferredTemplateOptions struct {
	config          *schema.AtmosConfiguration
	info            *schema.ConfigAndStacksInfo
	settings        *schema.Settings
	templateContext map[string]any
	onWarning       func(DegradationWarning)
}

func canDegradeValue(ac *schema.AtmosConfiguration, err error) bool {
	return deferred.CanRecover(err, ac.DeferredAuth != nil, isRecoverableInWarnMode)
}

func renderDeferredTemplateValues(input map[string]any, opts *deferredTemplateOptions) (map[string]any, error) {
	return deferred.RenderValues(input, func(value string) (any, error) {
		return renderDeferredTemplateValue(value, opts)
	})
}

func renderDeferredTemplateValue(value string, opts *deferredTemplateOptions) (any, error) {
	input, err := atmosYaml.ConvertToYAMLPreservingDelimiters(map[string]any{"value": value}, opts.config.Templates.Settings.Delimiters)
	if err != nil {
		return nil, err
	}
	rendered, err := ProcessTmplWithDatasources(opts.config, opts.info, *opts.settings, "deferred-value", input, opts.templateContext, true)
	if err != nil {
		if canDegradeValue(opts.config, err) {
			opts.onWarning(DegradationWarning{Stack: opts.info.Stack, Component: opts.info.Component, Function: value, Reason: err.Error()})
			return degradation.AtmosComputedValue{}, nil
		}
		return nil, err
	}
	result, err := u.UnmarshalYAML[map[string]any](rendered)
	if err != nil {
		return nil, err
	}
	return result["value"], nil
}
