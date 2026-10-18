package exec

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.TemplatesSettingsGomplate{Experimental: new(bool)}

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

const experimentalTemplate = `ip: '{{ net.CIDRHost 1 "10.0.0.0/24" }}'`

// TestProcessTmpl_GomplateExperimental tests experimental Gomplate function activation.
func TestProcessTmpl_GomplateExperimental(t *testing.T) {
	t.Parallel()

	t.Run("unset disables experimental functions", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.AtmosConfiguration{}

		_, err := ProcessTmpl(cfg, "experimental", experimentalTemplate, nil, false)

		require.ErrorContains(t, err, "experimental mode not enabled")
	})

	t.Run("enabled permits experimental functions", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.AtmosConfiguration{}
		cfg.Templates.Settings.Gomplate.Experimental = lo.ToPtr(true)

		result, err := ProcessTmpl(cfg, "experimental", experimentalTemplate, nil, false)

		require.NoError(t, err)
		assert.Equal(t, "ip: '10.0.0.1'", result)
	})

	t.Run("explicit false disables experimental functions", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.AtmosConfiguration{}
		cfg.Templates.Settings.Gomplate.Experimental = lo.ToPtr(false)

		_, err := ProcessTmpl(cfg, "experimental", experimentalTemplate, nil, false)

		require.ErrorContains(t, err, "experimental mode not enabled")
	})

	t.Run("nil config disables experimental functions", func(t *testing.T) {
		t.Parallel()

		_, err := ProcessTmpl(nil, "experimental", experimentalTemplate, nil, false)

		require.ErrorContains(t, err, "experimental mode not enabled")
	})
}

// TestProcessTmplWithDatasources_GomplateExperimental tests experimental setting inheritance and stack overrides.
func TestProcessTmplWithDatasources_GomplateExperimental(t *testing.T) {
	t.Parallel()

	t.Run("unset stack value preserves global activation", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.AtmosConfiguration{}
		cfg.Templates.Settings.Enabled = true
		cfg.Templates.Settings.Gomplate.Enabled = true
		cfg.Templates.Settings.Gomplate.Experimental = lo.ToPtr(true)

		var settings schema.Settings

		result, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, settings,
			"experimental", experimentalTemplate, nil, false)

		require.NoError(t, err)
		assert.Equal(t, "ip: '10.0.0.1'", result)
	})

	t.Run("explicit false stack value disables global activation", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.AtmosConfiguration{}
		cfg.Templates.Settings.Enabled = true
		cfg.Templates.Settings.Gomplate.Enabled = true
		cfg.Templates.Settings.Gomplate.Experimental = lo.ToPtr(true)

		var settings schema.Settings
		settings.Templates.Settings.Gomplate.Experimental = lo.ToPtr(false)

		_, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, settings,
			"experimental", experimentalTemplate, nil, false)

		require.ErrorContains(t, err, "experimental mode not enabled")
	})
}

// TestProcessTmplWithDatasources_GomplateExperimentalEvaluations tests experimental settings across evaluation passes.
func TestProcessTmplWithDatasources_GomplateExperimentalEvaluations(t *testing.T) {
	t.Parallel()

	t.Run("preserves the setting across evaluations", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.AtmosConfiguration{}
		cfg.Templates.Settings.Enabled = true
		cfg.Templates.Settings.Gomplate.Enabled = true
		cfg.Templates.Settings.Gomplate.Experimental = lo.ToPtr(true)
		cfg.Templates.Settings.Evaluations = 2

		tmpl := "{{ .next }}"

		result, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, schema.Settings{},
			"experimental", tmpl, map[string]any{"next": experimentalTemplate}, false)

		require.NoError(t, err)
		assert.Contains(t, result, "ip: '10.0.0.1'")

		assert.True(t, *cfg.Templates.Settings.Gomplate.Experimental)
	})

	t.Run("enables experimental functions for the next evaluation", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.AtmosConfiguration{}
		cfg.Templates.Settings.Enabled = true
		cfg.Templates.Settings.Gomplate.Enabled = true
		cfg.Templates.Settings.Gomplate.Experimental = lo.ToPtr(false)
		cfg.Templates.Settings.Evaluations = 2

		tmpl := `{{ .next }}
settings:
  templates:
    settings:
      gomplate:
        experimental: true
`

		result, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, schema.Settings{},
			"experimental", tmpl, map[string]any{"next": experimentalTemplate}, false)

		require.NoError(t, err)
		assert.Contains(t, result, "ip: '10.0.0.1'")

		assert.False(t, *cfg.Templates.Settings.Gomplate.Experimental)
	})

	t.Run("disables experimental functions for the next evaluation", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.AtmosConfiguration{}
		cfg.Templates.Settings.Enabled = true
		cfg.Templates.Settings.Gomplate.Enabled = true
		cfg.Templates.Settings.Gomplate.Experimental = lo.ToPtr(true)
		cfg.Templates.Settings.Evaluations = 2

		tmpl := `{{ .next }}
settings:
  templates:
    settings:
      gomplate:
        experimental: false
`

		_, err := ProcessTmplWithDatasources(cfg, &schema.ConfigAndStacksInfo{}, schema.Settings{},
			"experimental", tmpl, map[string]any{"next": experimentalTemplate}, false)

		require.ErrorContains(t, err, "experimental mode not enabled")

		assert.True(t, *cfg.Templates.Settings.Gomplate.Experimental)
	})
}
