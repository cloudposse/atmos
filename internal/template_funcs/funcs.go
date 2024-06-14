package template_funcs

import (
	"context"
	"text/template"
)

// FuncMap creates and returns a map of template functions
func FuncMap(ctx context.Context) template.FuncMap {
	atmosFuncs := &AtmosFuncs{ctx}

	return map[string]any{
		"atmos": func() any { return atmosFuncs },
	}
}

type AtmosFuncs struct {
	ctx context.Context
}

func (AtmosFuncs) Component(component string, stack string) (any, error) {
	return componentFunc(component, stack)
}
