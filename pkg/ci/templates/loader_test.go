package templates

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed testdata/templates/*.md
var rawTestEmbeddedTemplates embed.FS

// testEmbeddedFS returns the test embedded templates with the correct path structure.
// The loader expects "templates/plan.md", so we strip the "testdata" prefix.
func testEmbeddedFS() fs.FS {
	sub, _ := fs.Sub(rawTestEmbeddedTemplates, "testdata")
	return sub
}

// TestLoaderLoad tests template loading with various override scenarios.
func TestLoaderLoad(t *testing.T) {
	// Create temp directory for custom templates.
	tmpDir := t.TempDir()

	// Create custom template files.
	customDir := filepath.Join(tmpDir, "custom")
	terraformDir := filepath.Join(customDir, "terraform")
	require.NoError(t, os.MkdirAll(terraformDir, 0o755))

	// Create custom templates.
	err := os.WriteFile(
		filepath.Join(terraformDir, "plan.md"),
		[]byte("# Custom Plan Template\n{{ .Component }}"),
		0o644,
	)
	require.NoError(t, err)

	err = os.WriteFile(
		filepath.Join(customDir, "explicit-plan.md"),
		[]byte("# Explicit Override\n{{ .Stack }}"),
		0o644,
	)
	require.NoError(t, err)

	tests := []struct {
		name          string
		config        *schema.AtmosConfiguration
		componentType string
		command       string
		wantContains  string
		wantErr       bool
	}{
		{
			name:          "nil config uses embedded default",
			config:        nil,
			componentType: "terraform",
			command:       "plan",
			wantContains:  "Test Plan Template", // From testdata/templates/plan.md.
		},
		{
			name: "base_path convention override",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Templates: schema.CITemplatesConfig{
						BasePath: customDir,
					},
				},
			},
			componentType: "terraform",
			command:       "plan",
			wantContains:  "Custom Plan Template",
		},
		{
			name: "explicit file override takes precedence",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Templates: schema.CITemplatesConfig{
						BasePath: customDir,
						Terraform: map[string]string{
							"plan": "explicit-plan.md",
						},
					},
				},
			},
			componentType: "terraform",
			command:       "plan",
			wantContains:  "Explicit Override",
		},
		{
			name: "fallback to embedded when file not found",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Templates: schema.CITemplatesConfig{
						BasePath: customDir,
					},
				},
			},
			componentType: "terraform",
			command:       "apply", // No custom apply template exists.
			wantContains:  "Test Apply Template",
		},
		{
			name: "relative base_path resolved from atmos base",
			config: &schema.AtmosConfiguration{
				BasePath: tmpDir,
				CI: schema.CIConfig{
					Templates: schema.CITemplatesConfig{
						BasePath: "custom", // Relative to tmpDir.
					},
				},
			},
			componentType: "terraform",
			command:       "plan",
			wantContains:  "Custom Plan Template",
		},
	}

	testFS := testEmbeddedFS()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader(tt.config)
			content, err := loader.Load(tt.componentType, tt.command, testFS)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Contains(t, content, tt.wantContains)
		})
	}
}

// TestLoaderRender tests template rendering.
func TestLoaderRender(t *testing.T) {
	loader := NewLoader(nil)

	tests := []struct {
		name         string
		template     string
		context      map[string]any
		wantContains string
		wantErr      bool
	}{
		{
			name:         "simple variable substitution",
			template:     "Component: {{ .Component }}",
			context:      map[string]any{"Component": "vpc"},
			wantContains: "Component: vpc",
		},
		{
			name:         "nested variable",
			template:     "Count: {{ .Resources.Create }}",
			context:      map[string]any{"Resources": map[string]int{"Create": 5}},
			wantContains: "Count: 5",
		},
		{
			name:         "conditional",
			template:     "{{ if .HasChanges }}Changes!{{ else }}No changes{{ end }}",
			context:      map[string]any{"HasChanges": true},
			wantContains: "Changes!",
		},
		{
			name:     "invalid template syntax",
			template: "{{ .Invalid }",
			context:  map[string]any{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := loader.Render(tt.template, tt.context)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Contains(t, result, tt.wantContains)
		})
	}
}

// TestLoaderLoadAndRender tests the combined load and render function.
func TestLoaderLoadAndRender(t *testing.T) {
	loader := NewLoader(nil)

	ctx := map[string]any{
		"Component": "vpc",
		"Stack":     "dev-us-east-1",
	}

	result, err := loader.LoadAndRender("terraform", "plan", testEmbeddedFS(), ctx)
	require.NoError(t, err)

	// Should contain content from default template.
	assert.Contains(t, result, "vpc")
	assert.Contains(t, result, "dev-us-east-1")
}

// TestLoaderGetComponentOverrides tests override map lookup.
func TestLoaderGetComponentOverrides(t *testing.T) {
	config := &schema.AtmosConfiguration{
		CI: schema.CIConfig{
			Templates: schema.CITemplatesConfig{
				Terraform: map[string]string{"plan": "custom-plan.md"},
				Helmfile:  map[string]string{"diff": "custom-diff.md"},
			},
		},
	}

	loader := NewLoader(config)

	tests := []struct {
		componentType string
		wantKey       string
		wantValue     string
	}{
		{"terraform", "plan", "custom-plan.md"},
		{"helmfile", "diff", "custom-diff.md"},
		{"unknown", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.componentType, func(t *testing.T) {
			overrides := loader.getComponentOverrides(tt.componentType)
			if tt.wantKey == "" {
				assert.Nil(t, overrides)
			} else {
				require.NotNil(t, overrides)
				assert.Equal(t, tt.wantValue, overrides[tt.wantKey])
			}
		})
	}
}

// TestLoaderResolvePath tests path resolution logic.
func TestLoaderResolvePath(t *testing.T) {
	// Platform-specific absolute paths.
	// On Windows, absolute paths require a drive letter (e.g., C:\).
	// On Unix, absolute paths start with / (e.g., /absolute/path).
	var absPath, absTemplatePath, absCustomPath, absAtmosPath string
	if runtime.GOOS == "windows" {
		absPath = "C:\\absolute\\path\\template.md"
		absTemplatePath = "C:\\absolute\\path\\template.md"
		absCustomPath = "C:\\custom"
		absAtmosPath = "C:\\atmos"
	} else {
		absPath = "/absolute/path/template.md"
		absTemplatePath = "/absolute/path/template.md"
		absCustomPath = "/custom"
		absAtmosPath = "/atmos"
	}

	tests := []struct {
		name      string
		basePath  string
		atmosBase string
		filename  string
		want      string
	}{
		{
			name:     "absolute path unchanged",
			basePath: absCustomPath,
			filename: absPath,
			want:     absTemplatePath,
		},
		{
			name:     "relative resolved from basePath",
			basePath: absCustomPath,
			filename: "template.md",
			want:     filepath.Join(absCustomPath, "template.md"),
		},
		{
			name:      "relative resolved from atmosBase when no basePath",
			basePath:  "",
			atmosBase: absAtmosPath,
			filename:  "template.md",
			want:      filepath.Join(absAtmosPath, "template.md"),
		},
		{
			name:     "relative unchanged when no bases",
			basePath: "",
			filename: "template.md",
			want:     "template.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config *schema.AtmosConfiguration
			if tt.atmosBase != "" {
				config = &schema.AtmosConfiguration{BasePath: tt.atmosBase}
			}

			loader := &Loader{
				atmosConfig: config,
				basePath:    tt.basePath,
			}

			result := loader.resolvePath(tt.filename)
			assert.Equal(t, tt.want, result)
		})
	}
}
