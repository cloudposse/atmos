package template_funcs

func componentFunc(component string, stack string) (any, error) {
	res := map[string]any{
		"outputs": map[string]any{
			"id": stack,
		},
	}

	return res, nil
}
