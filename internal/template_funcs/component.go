package template_funcs

import "github.com/pkg/errors"

func componentFunc(component string, stackSelectors ...map[string]any) (any, error) {
	if len(stackSelectors) != 1 {
		return nil, errors.New("'atmos.Component' template function accepts two parameters: component and optional stack selector map")
	}

	stackSelector := stackSelectors[0]

	res := map[string]any{
		"outputs": map[string]any{
			"id": stackSelector["stage"],
		},
	}

	return res, nil
}
