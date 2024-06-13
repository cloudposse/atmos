package template_funcs

import (
	"context"
	"text/template"

	"github.com/samber/lo"
)

// FuncMap creates and returns a map of template functions
func FuncMap(ctx context.Context) template.FuncMap {
	funcs := template.FuncMap{}
	funcs = lo.Assign(funcs, CreateAtmosFuncs(ctx))
	return funcs
}

func CreateAtmosFuncs(ctx context.Context) map[string]interface{} {
	atmosFuncs := &AtmosFuncs{ctx}

	return map[string]interface{}{
		"atmos": func() interface{} { return atmosFuncs },
	}
}

type AtmosFuncs struct {
	ctx context.Context
}

func (AtmosFuncs) Component(component string, stack ...interface{}) (string, error) {
	return componentFunc(component, stack)
}
