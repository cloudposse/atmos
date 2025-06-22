package selector

// ExtractStackLabels returns a map[string]string of labels defined under metadata.labels in a stack manifest map.
// If no labels defined, returns empty map.
func ExtractStackLabels(stack map[string]any) map[string]string {
	meta, _ := stack["metadata"].(map[string]any)
	if meta == nil {
		return map[string]string{}
	}
	if lbl, ok := meta["labels"].(map[string]any); ok {
		return toStringMap(lbl)
	}
	return map[string]string{}
}

// ExtractComponentLabels returns component-level labels under metadata.labels for a component map.
func ExtractComponentLabels(component map[string]any) map[string]string {
	meta, _ := component["metadata"].(map[string]any)
	if meta == nil {
		return map[string]string{}
	}
	if lbl, ok := meta["labels"].(map[string]any); ok {
		return toStringMap(lbl)
	}
	return map[string]string{}
}

func toStringMap(m map[string]any) map[string]string {
	res := make(map[string]string)
	for k, v := range m {
		if s, ok := v.(string); ok {
			res[k] = s
		}
	}
	return res
}

// MergedLabels returns stack labels overlaid with component-level labels (component overrides stack).
// If component not found or has no labels, returns stack labels.
func MergedLabels(stack map[string]any, componentName string) map[string]string {
	merged := ExtractStackLabels(stack)

	if componentName == "" {
		return merged
	}

	// Look into terraform and helmfile sections
	compsSection, ok := stack["components"].(map[string]any)
	if !ok {
		return merged
	}
	for _, ctype := range []string{"terraform", "helmfile"} {
		if tsec, ok := compsSection[ctype].(map[string]any); ok {
			if comp, ok := tsec[componentName].(map[string]any); ok {
				cl := ExtractComponentLabels(comp)
				for k, v := range cl {
					merged[k] = v
				}
				return merged
			}
		}
	}
	return merged
}
