package template_funcs

func componentFunc(component string, stack string) (any, error) {
	outputs := map[string]any{
		"outputs": map[string]any{
			"id": stack,
		},
	}

	return outputs, nil
}
