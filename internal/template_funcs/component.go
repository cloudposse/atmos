package template_funcs

import (
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/internal/exec"
)

func componentFunc(component string, stack string) (any, error) {
	sections, err := exec.ExecuteDescribeComponent(component, stack)
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
