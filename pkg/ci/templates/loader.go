// Package templates provides CI summary template loading and rendering.
package templates

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// templateFuncs provides custom template functions for CI summary templates.
var templateFuncs = template.FuncMap{
	"replace": strings.ReplaceAll,
}

// Loader loads CI templates with layered override support.
type Loader struct {
	atmosConfig *schema.AtmosConfiguration
	basePath    string
}

// NewLoader creates a new template loader.
func NewLoader(atmosConfig *schema.AtmosConfiguration) *Loader {
	defer perf.Track(atmosConfig, "templates.NewLoader")()

	basePath := ""
	if atmosConfig != nil && atmosConfig.CI.Templates.BasePath != "" {
		basePath = atmosConfig.CI.Templates.BasePath
		// Resolve relative to atmos.yaml location if not absolute.
		if !filepath.IsAbs(basePath) && atmosConfig.BasePath != "" {
			basePath = filepath.Join(atmosConfig.BasePath, basePath)
		}
	}

	return &Loader{
		atmosConfig: atmosConfig,
		basePath:    basePath,
	}
}

// Load returns template content for a component type and command.
// Override precedence:
// 1. Explicit file from config (e.g., ci.templates.terraform.plan)
// 2. Convention-based file from base_path (e.g., {base_path}/terraform/plan.md)
// 3. Embedded default from provider.
func (l *Loader) Load(componentType, command string, defaultTemplates fs.FS) (string, error) {
	defer perf.Track(l.atmosConfig, "templates.Loader.Load")()

	// 1. Check explicit override from config.
	if content := l.loadFromConfigOverride(componentType, command); content != "" {
		return content, nil
	}

	// 2. Check base_path directory by convention.
	if content := l.loadFromBasePath(componentType, command); content != "" {
		return content, nil
	}

	// 3. Fall back to embedded default.
	return l.loadFromEmbedded(componentType, command, defaultTemplates)
}

// loadFromConfigOverride attempts to load template from config overrides.
func (l *Loader) loadFromConfigOverride(componentType, command string) string {
	if l.atmosConfig == nil {
		return ""
	}

	overrides := l.getComponentOverrides(componentType)
	if overrides == nil {
		return ""
	}

	filename, ok := overrides[command]
	if !ok || filename == "" {
		return ""
	}

	path := l.resolvePath(filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

// loadFromBasePath attempts to load template from base path by convention.
func (l *Loader) loadFromBasePath(componentType, command string) string {
	if l.basePath == "" {
		return ""
	}

	path := filepath.Join(l.basePath, componentType, command+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

// loadFromEmbedded loads template from embedded filesystem.
func (l *Loader) loadFromEmbedded(componentType, command string, defaultTemplates fs.FS) (string, error) {
	content, err := fs.ReadFile(defaultTemplates, "templates/"+command+".md")
	if err != nil {
		return "", errUtils.Build(errUtils.ErrFileNotFound).
			WithCause(err).
			WithExplanation("Template not found").
			WithContext("component_type", componentType).
			WithContext("command", command).
			WithHint("Check that the template exists in the provider's embedded templates").
			Err()
	}
	return string(content), nil
}

// Render renders a template with the given context.
func (l *Loader) Render(templateContent string, ctx any) (string, error) {
	defer perf.Track(l.atmosConfig, "templates.Loader.Render")()

	tmpl, err := template.New("ci-summary").Funcs(templateFuncs).Parse(templateContent)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to parse template").
			Err()
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to execute template").
			Err()
	}

	return buf.String(), nil
}

// LoadAndRender loads a template and renders it with the given context.
func (l *Loader) LoadAndRender(componentType, command string, defaultTemplates fs.FS, ctx any) (string, error) {
	defer perf.Track(l.atmosConfig, "templates.Loader.LoadAndRender")()

	content, err := l.Load(componentType, command, defaultTemplates)
	if err != nil {
		return "", err
	}

	return l.Render(content, ctx)
}

// getComponentOverrides returns override configuration for a component type.
func (l *Loader) getComponentOverrides(componentType string) map[string]string {
	if l.atmosConfig == nil {
		return nil
	}

	cfg := l.atmosConfig.CI.Templates

	switch componentType {
	case "terraform":
		return cfg.Terraform
	case "helmfile":
		return cfg.Helmfile
	default:
		return nil
	}
}

// resolvePath resolves a template path, making it absolute if relative.
func (l *Loader) resolvePath(filename string) string {
	if filepath.IsAbs(filename) {
		return filename
	}

	if l.basePath != "" {
		return filepath.Join(l.basePath, filename)
	}

	if l.atmosConfig != nil && l.atmosConfig.BasePath != "" {
		return filepath.Join(l.atmosConfig.BasePath, filename)
	}

	return filename
}
