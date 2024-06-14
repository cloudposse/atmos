package exec

import (
	"context"
	"text/template"

	"github.com/samber/lo"
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

func componentFunc(component string, stack string) (any, error) {
	sections, err := ExecuteDescribeComponent(component, stack)
	if err != nil {
		return nil, err
	}

	outputs := map[string]any{
		"outputs": map[string]any{
			"id": stack,
		},
	}

	sections = lo.Assign(sections, outputs)

	return sections, nil
}
