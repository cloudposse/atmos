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
)

// FuncMap creates and returns a map of template functions
func FuncMap(ctx context.Context, gomplateData *data.Data) template.FuncMap {
	atmosFuncs := &AtmosFuncs{ctx, gomplateData}

	return map[string]any{
		"atmos": func() any { return atmosFuncs },
	}
}

type AtmosFuncs struct {
	ctx          context.Context
	gomplateData *data.Data
}

func (AtmosFuncs) Component(component string, stack string) (any, error) {
	return componentFunc(component, stack)
}

func (f AtmosFuncs) GomplateDatasource(alias string, args ...string) (any, error) {
	return gomplateDatasourceFunc(alias, f.gomplateData, args...)
}
