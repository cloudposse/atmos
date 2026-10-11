package doctor

import "strings"

// templateMarkers returns opening delimiters that mark a value as an unevaluated template.
// The Go template default is always included, so configured delimiters only widen detection.
func (p *project) templateMarkers() []string {
	markers := []string{defaultTemplateMarker}
	templates, _ := p.config["templates"].(map[string]any)
	settings, _ := templates["settings"].(map[string]any)
	delimiters, _ := settings["delimiters"].([]any)
	if len(delimiters) > 0 {
		if left, ok := delimiters[0].(string); ok && left != "" && left != defaultTemplateMarker {
			markers = append(markers, left)
		}
	}
	return markers
}

func containsTemplate(value string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
