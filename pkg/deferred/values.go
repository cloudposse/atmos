package deferred

import "github.com/cloudposse/atmos/pkg/perf"

// RenderValues evaluates each scalar separately, preserving successful siblings.
func RenderValues(input map[string]any, evaluate func(string) (any, error)) (map[string]any, error) {
	defer perf.Track(nil, "deferred.RenderValues")()

	var walk func(any) (any, error)
	walk = func(value any) (any, error) {
		switch v := value.(type) {
		case map[string]any:
			result := make(map[string]any, len(v))
			for key, child := range v {
				converted, err := walk(child)
				if err != nil {
					return nil, err
				}
				result[key] = converted
			}
			return result, nil
		case []any:
			result := make([]any, len(v))
			for i, child := range v {
				converted, err := walk(child)
				if err != nil {
					return nil, err
				}
				result[i] = converted
			}
			return result, nil
		case string:
			return evaluate(v)
		default:
			return value, nil
		}
	}
	result, err := walk(input)
	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}
