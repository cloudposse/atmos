package template_funcs

func componentFunc(component string, stack ...interface{}) (interface{}, error) {
	res := map[string]interface{}{
		"outputs": map[string]interface{}{
			"id": "id1-test",
		},
	}
	return res, nil
}
