// https://forum.golangbridge.org/t/html-template-optional-argument-in-function/6080
// https://lkumarjain.blogspot.com/2020/11/deep-dive-into-go-template.html
// https://echorand.me/posts/golang-templates/
// https://www.practical-go-lessons.com/chap-32-templates
// https://docs.gofiber.io/template/next/html/TEMPLATES_CHEATSHEET/
// https://engineering.01cloud.com/2023/04/13/optional-function-parameter-pattern/

package exec

import (
	"context"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/templatefuncs"
	"github.com/cloudposse/atmos/pkg/templating"
)

// FuncMap creates and returns a map of template functions. The datasources
// reader serves `atmos.GomplateDatasource`; it is normally the template engine
// rendering the template, and may be nil when no datasources are available.
func FuncMap(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	ctx context.Context,
	datasources templating.DatasourceReader,
) template.FuncMap {
	defer perf.Track(atmosConfig, "exec.FuncMap")()

	atmosFuncs := &AtmosFuncs{atmosConfig, configAndStacksInfo, ctx, datasources}

	funcs := templatefuncs.FuncMap()
	funcs["atmos"] = func() any { return atmosFuncs }

	return funcs
}

// AtmosFuncs exposes functions available in templates via the "atmos" namespace.
type AtmosFuncs struct {
	atmosConfig         *schema.AtmosConfiguration
	configAndStacksInfo *schema.ConfigAndStacksInfo
	ctx                 context.Context
	datasources         templating.DatasourceReader
}

// Component returns component configuration for the given component and stack.
func (f AtmosFuncs) Component(component string, stack string) (any, error) {
	return componentFunc(f.atmosConfig, f.configAndStacksInfo, component, stack)
}

// GomplateDatasource returns data for a gomplate datasource alias. Results are
// cached by the template engine for the lifetime of the process, so a
// datasource referenced from many stacks and components is fetched once.
func (f AtmosFuncs) GomplateDatasource(alias string, args ...string) (any, error) {
	defer perf.Track(f.atmosConfig, "exec.AtmosFuncs.GomplateDatasource")()

	log.Debug("atmos.GomplateDatasource(): processing datasource", "alias", alias)

	if f.datasources == nil {
		return nil, errUtils.ErrGomplateDatasourceUnavailable
	}

	result, err := f.datasources.Datasource(alias, args...)
	if err != nil {
		return nil, err
	}

	log.Debug("atmos.GomplateDatasource(): processed datasource", "alias", alias, "result", result)

	return result, nil
}

// Store reads a value from a named store for the given stack, component, and key.
func (f AtmosFuncs) Store(store string, stack string, component string, key string) (any, error) {
	defer perf.Track(nil, "exec.AtmosFuncs.Store")()

	return storeFunc(f.atmosConfig, store, stack, component, key)
}

// Resolve evaluates an Atmos YAML-function string (e.g. "!git.repository", "!exec ...",
// "!store ...") at template-render time and returns the resolved value.
//
// It enables composing a YAML-function result with other strings and template variables
// in a single value, which a bare YAML tag cannot do because a tag owns the entire scalar.
// For example:
//
//	settings:
//	  context:
//	    repo: "!git.repository"
//	  terraform:
//	    workspace_key_prefix: '{{ atmos.Resolve .settings.context.repo }}/{{ .atmos_component }}'
//
// Because it runs during template rendering (before the eager YAML-function pass), it has
// the same evaluation semantics as atmos.Component. A plain (untagged) string is returned
// unchanged.
func (f AtmosFuncs) Resolve(input string) (any, error) {
	defer perf.Track(f.atmosConfig, "exec.AtmosFuncs.Resolve")()

	var stack string
	if f.configAndStacksInfo != nil {
		stack = f.configAndStacksInfo.Stack
	}
	return processCustomTags(f.atmosConfig, input, stack, nil, f.configAndStacksInfo)
}
