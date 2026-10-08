// https://forum.golangbridge.org/t/html-template-optional-argument-in-function/6080
// https://lkumarjain.blogspot.com/2020/11/deep-dive-into-go-template.html
// https://echorand.me/posts/golang-templates/
// https://www.practical-go-lessons.com/chap-32-templates
// https://docs.gofiber.io/template/next/html/TEMPLATES_CHEATSHEET/
// https://engineering.01cloud.com/2023/04/13/optional-function-parameter-pattern/

package exec

import (
	"context"
	"fmt"
	"text/template"

	"github.com/hairyhenderson/gomplate/v3/data"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
	"github.com/cloudposse/atmos/pkg/templatefuncs"
)

// FuncMap creates and returns a map of template functions.
func FuncMap(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	ctx context.Context,
	gomplateData *data.Data,
) template.FuncMap {
	defer perf.Track(atmosConfig, "exec.FuncMap")()

	return newFuncMap(atmosConfig, configAndStacksInfo, ctx, gomplateData, "")
}

// manifestLoadFuncMap creates the template functions used while a stack manifest is being loaded
// (before all manifests are merged). It is identical to FuncMap except that atmos.Component fails
// with a clear error instead of describing the target component, which would load every stack
// manifest again, including the manifest currently being rendered.
func manifestLoadFuncMap(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	ctx context.Context,
	gomplateData *data.Data,
	manifestFile string,
) template.FuncMap {
	defer perf.Track(atmosConfig, "exec.manifestLoadFuncMap")()

	return newFuncMap(atmosConfig, configAndStacksInfo, ctx, gomplateData, manifestFile)
}

// newFuncMap builds the template function map. A non-empty manifestFile marks the functions as
// running during stack manifest loading.
func newFuncMap(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	ctx context.Context,
	gomplateData *data.Data,
	manifestFile string,
) template.FuncMap {
	atmosFuncs := &AtmosFuncs{
		atmosConfig:         atmosConfig,
		configAndStacksInfo: configAndStacksInfo,
		ctx:                 ctx,
		gomplateData:        gomplateData,
		manifestLoadFile:    manifestFile,
	}

	funcs := templatefuncs.FuncMap()
	funcs["atmos"] = func() any { return atmosFuncs }

	return funcs
}

// AtmosFuncs exposes functions available in templates via the "atmos" namespace.
type AtmosFuncs struct {
	atmosConfig         *schema.AtmosConfiguration
	configAndStacksInfo *schema.ConfigAndStacksInfo
	ctx                 context.Context
	gomplateData        *data.Data
	// manifestLoadFile is the stack manifest being rendered while stacks are loading.
	// Empty outside manifest loading.
	manifestLoadFile string
}

// Component returns component configuration for the given component and stack.
func (f AtmosFuncs) Component(component string, stack string) (any, error) {
	// Re-entry guard: describing a component loads and renders every stack manifest, including the
	// manifest that is being rendered right now, which would call atmos.Component again without limit.
	if f.manifestLoadFile != "" {
		return nil, errUtils.Build(fmt.Errorf("%w: atmos.Component(%q, %q) in stack manifest '%s'",
			errUtils.ErrComponentFuncDuringManifestLoad, component, stack, f.manifestLoadFile)).
			WithContext("file", f.manifestLoadFile).
			WithContext("component", component).
			WithContext("stack", stack).
			WithHint("Use atmos.Component in a component section (vars, settings, env) of a regular stack manifest: Atmos evaluates those after all manifests are loaded.").
			WithHint("Manifests that are fully rendered while loading (files ending in .tmpl and imports with a context) cannot use atmos.Component.").
			Err()
	}
	return componentFunc(f.atmosConfig, f.configAndStacksInfo, component, stack)
}

// GomplateDatasource returns data for a gomplate datasource alias.
func (f AtmosFuncs) GomplateDatasource(alias string, args ...string) (any, error) {
	return gomplateDatasourceFunc(alias, f.gomplateData, args...)
}

// Store reads a value from a named store for the given stack, component, and key.
func (f AtmosFuncs) Store(store string, stack string, component string, key string) (any, error) {
	defer perf.Track(nil, "exec.AtmosFuncs.Store")()

	if authdeferred.IsDeferred(f.atmosConfig.AuthManager) {
		return storedeferred.LookupStore(f.atmosConfig, f.configAndStacksInfo, storedeferred.StoreOptions{Name: store, Stack: stack, Component: component, Key: key})
	}
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
