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

	"github.com/hairyhenderson/gomplate/v3/data"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// FuncMap creates and returns a map of template functions.
func FuncMap(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	ctx context.Context,
	gomplateData *data.Data,
) template.FuncMap {
	defer perf.Track(atmosConfig, "exec.FuncMap")()

	atmosFuncs := &AtmosFuncs{atmosConfig, configAndStacksInfo, ctx, gomplateData}

	return map[string]any{
		"atmos": func() any { return atmosFuncs },
	}
}

// AtmosFuncs exposes functions available in templates via the "atmos" namespace.
type AtmosFuncs struct {
	atmosConfig         *schema.AtmosConfiguration
	configAndStacksInfo *schema.ConfigAndStacksInfo
	ctx                 context.Context
	gomplateData        *data.Data
}

// Component returns component configuration for the given component and stack.
func (f AtmosFuncs) Component(component string, stack string) (any, error) {
	return componentFunc(f.atmosConfig, f.configAndStacksInfo, component, stack)
}

// GomplateDatasource returns data for a gomplate datasource alias.
func (f AtmosFuncs) GomplateDatasource(alias string, args ...string) (any, error) {
	return gomplateDatasourceFunc(alias, f.gomplateData, args...)
}

// Store reads a value from a named store for the given stack, component, and key.
func (f AtmosFuncs) Store(store string, stack string, component string, key string) (any, error) {
	defer perf.Track(nil, "exec.AtmosFuncs.Store")()

	return storeFunc(f.atmosConfig, store, stack, component, key)
}
