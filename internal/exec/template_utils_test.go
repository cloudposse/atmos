package exec

import "testing"

// TestProcessTmplWithDatasourcesGomplate tests that a static template is rendered correctly.
func TestProcessTmplWithDatasourcesGomplate(t *testing.T) {
	t.Parallel()

	// Test case 1: Static content with no interpolation
	mergedData := map[string]interface{}{
		// No variables to interpolate.
	}
	tmpl := "Static Content"
	result, err := ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, false)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	// For a static template with no interpolation, the output should equal the template.
	if result != tmpl {
		t.Errorf("Expected result to be %q, got %q", tmpl, result)
	}

	// Test case 2: Template with variable interpolation
	mergedData = map[string]interface{}{
		"name": "Atmos",
	}
	tmpl = "Hello {{ .name }}!"

	result, err = ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, false)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	expected := "Hello Atmos!"
	if result != expected {
		t.Errorf("Expected result to be %q, got %q", expected, result)
	}

	// Test case 3: Template with config datasource interpolation
	tmpl = " {{- $data := (ds \"config\") -}}\n\nHello {{ $data.name | default \"Project Title\" }}!"

	result, err = ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, false)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	expected = "Hello Atmos!"
	if result != expected {
		t.Errorf("Expected result to be %q, got %q", expected, result)
	}

	// Test case 4: Nested variable access
	mergedData = map[string]interface{}{
		"config": map[string]interface{}{
			"version": "1.0.0",
		},
	}
	tmpl = "{{- $data := (ds \"config\") -}}\n\nVersion: {{ $data.config.version }}"

	result, err = ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, false)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	expected = "Version: 1.0.0"
	if result != expected {
		t.Errorf("Expected result to be %q, got %q", expected, result)
	}

	// Test case 4: Root data fields remain available for fallback templates.
	mergedData = map[string]interface{}{
		"name": "Atmos",
	}
	tmpl = "Project: {{ .name }}"

	result, err = ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, true)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	expected = "Project: Atmos"
	if result != expected {
		t.Errorf("Expected result to be %q, got %q", expected, result)
	}

	// Test case 5: Legacy Cloud Posse README templates define their own config
	// datasource from .Env.README_YAML.
	mergedData = map[string]interface{}{
		"name": "Atmos",
	}
	tmpl = "{{ defineDatasource \"config\" .Env.README_YAML }}Project: {{ (ds \"config\").name }}"

	result, err = ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, true)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	expected = "Project: Atmos"
	if result != expected {
		t.Errorf("Expected result to be %q, got %q", expected, result)
	}
}
