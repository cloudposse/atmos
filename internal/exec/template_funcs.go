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

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
	templatepolicy "github.com/cloudposse/atmos/pkg/template/policy"
	"github.com/cloudposse/atmos/pkg/template/render"
	"github.com/cloudposse/atmos/pkg/templatefuncs"
)

// FuncMap creates and returns a map of template functions. The gomplateData
// datasources serve `atmos.GomplateDatasource`; pass the same Data the
// template engine renders with so both share the stack's definitions.
func FuncMap(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	ctx context.Context,
	gomplateData *render.Data,
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
	gomplateData *render.Data,
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
	gomplateData *render.Data,
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
	gomplateData        *render.Data
	// manifestLoadFile is the stack manifest being rendered while stacks are loading.
	// Empty outside manifest loading.
	manifestLoadFile string
	sourceFile       string
}

// Policy wrapping binds the `atmos` namespace to each template file through
// this interface; without it, calls would be attributed to no file.
var _ templatepolicy.SourceFileBinder = (*AtmosFuncs)(nil)

// BindSourceFile returns a copy of the namespace that attributes its policy
// checks to file, the template the calls come from.
func (f *AtmosFuncs) BindSourceFile(file string) any {
	bound := *f
	bound.sourceFile = file
	return &bound
}

// Component returns component configuration for the given component and stack.
func (f AtmosFuncs) Component(component string, stack string) (any, error) {
	if err := guard.Function(f.ctx, "atmos.Component", templatepolicy.Engine, guard.FileOrigin(f.sourceFile)); err != nil {
		return nil, err
	}
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
	return componentFunc(f.ctx, f.atmosConfig, f.configAndStacksInfo, component, stack)
}

// GomplateDatasource returns data for a gomplate datasource alias. Without
// source policies, results are cached for the lifetime of the process, keyed
// by the alias's effective definition, the arguments and the environment, so
// a datasource referenced from many stacks and components is fetched once per
// environment.
func (f AtmosFuncs) GomplateDatasource(alias string, args ...string) (any, error) {
	if err := guard.Function(f.ctx, "atmos.GomplateDatasource", templatepolicy.Engine, guard.FileOrigin(f.sourceFile)); err != nil {
		return nil, err
	}
	sourceRules, err := guard.Enabled("source")
	if err != nil {
		return nil, err
	}
	if !sourceRules {
		// Nothing guards the read, so keep the cache that avoids fetching a datasource again.
		// The cache key includes the alias's URL and headers and the environment, so two
		// stack manifests that define the same alias differently, or that set different
		// templates.settings.env values, never see each other's value.
		log.Debug("atmos.GomplateDatasource(): processing datasource", "alias", alias)
		return render.CachedDatasource(f.gomplateData, alias, args...)
	}
	return templatepolicy.Datasource(f.gomplateData, f.sourceFile, alias, false, args...)
}

// Store reads a value from a named store for the given stack, component, and key.
func (f AtmosFuncs) Store(store string, stack string, component string, key string) (any, error) {
	if err := guard.Function(f.ctx, "atmos.Store", templatepolicy.Engine, guard.FileOrigin(f.sourceFile)); err != nil {
		return nil, err
	}
	defer perf.Track(nil, "exec.AtmosFuncs.Store")()

	if authdeferred.IsDeferred(f.atmosConfig.AuthManager) {
		return deferredStoreFunc(f.atmosConfig, f.configAndStacksInfo, storedeferred.StoreOptions{Name: store, Stack: stack, Component: component, Key: key})
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
	if err := guard.Function(f.ctx, "atmos.Resolve", templatepolicy.Engine, guard.FileOrigin(f.sourceFile)); err != nil {
		return nil, err
	}
	defer perf.Track(f.atmosConfig, "exec.AtmosFuncs.Resolve")()

	guard.RememberValue(input, guard.FileOrigin(f.sourceFile))
	var stack string
	if f.configAndStacksInfo != nil {
		stack = f.configAndStacksInfo.Stack
	}
	return processCustomTags(f.atmosConfig, input, stack, nil, f.configAndStacksInfo)
}
