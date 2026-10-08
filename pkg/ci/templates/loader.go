// Package templates provides CI summary template loading and rendering.
package templates

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// containerComponentType is the component type of container image summaries.
const containerComponentType = "container"

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
	content, err := l.loadFromConfigOverride(componentType, command)
	if err != nil {
		return "", err
	}
	if content != "" {
		return content, nil
	}

	// 2. Check base_path directory by convention.
	if content = l.loadFromBasePath(componentType, command); content != "" {
		return content, nil
	}

	// 3. Fall back to embedded default.
	return l.loadFromEmbedded(componentType, command, defaultTemplates)
}

// loadFromConfigOverride attempts to load template from config overrides.
//
// A configured override that cannot be read falls through to the next layer for the native
// plugin component types, which have always tolerated it. The container component is strict: a
// configured ci.templates.container.<command> file that is missing is an error, so a typo in the
// path cannot silently revert to the embedded default. The embedded default is used for the
// container only when the key is unset.
func (l *Loader) loadFromConfigOverride(componentType, command string) (string, error) {
	if l.atmosConfig == nil {
		return "", nil
	}

	overrides := l.getComponentOverrides(componentType)
	if overrides == nil {
		return "", nil
	}

	filename, ok := overrides[command]
	if !ok || filename == "" {
		return "", nil
	}

	path := l.resolvePath(filename)
	content, err := os.ReadFile(path)
	if err != nil {
		if componentType == containerComponentType {
			return "", l.overrideNotFoundError(componentType, command, filename, path, err)
		}
		return "", nil
	}
	return string(content), nil
}

// overrideNotFoundError describes a configured template override that cannot be read.
func (l *Loader) overrideNotFoundError(componentType, command, filename, path string, cause error) error {
	return errUtils.Build(errUtils.ErrCITemplateNotFound).
		WithCause(cause).
		WithExplanation(fmt.Sprintf("The template configured as ci.templates.%s.%s does not exist at %s (ci.templates.base_path: %s)", componentType, command, path, l.basePathDescription())).
		WithContext("configured", filename).
		WithContext("resolved_path", path).
		WithContext("ci.templates.base_path", l.basePathDescription()).
		WithHint("Create the file, fix the path, or remove the key to use the built-in template").
		Err()
}

// basePathDescription returns the effective templates base path for error messages.
func (l *Loader) basePathDescription() string {
	if l.basePath == "" {
		return "(unset)"
	}
	return l.basePath
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

// Render renders a template with the given context. A missing map key renders as "<no value>",
// which native plugin templates rely on; script-authored report templates use RenderStrict.
func (l *Loader) Render(templateContent string, ctx any) (string, error) {
	defer perf.Track(l.atmosConfig, "templates.Loader.Render")()

	return l.render(templateContent, ctx)
}

// RenderStrict renders a template and fails on a missing map key, naming the key, so a typo in a
// script's data keys or a template placeholder cannot silently render "<no value>".
func (l *Loader) RenderStrict(templateContent string, ctx any) (string, error) {
	defer perf.Track(l.atmosConfig, "templates.Loader.RenderStrict")()

	return l.render(templateContent, ctx, "missingkey=error")
}

// render parses and executes a template with the given template options.
func (l *Loader) render(templateContent string, ctx any, options ...string) (string, error) {
	tmpl, err := template.New("ci-summary").Funcs(templateFuncs).Option(options...).Parse(templateContent)
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
	case "helm":
		return cfg.Helm
	case "helmfile":
		return cfg.Helmfile
	case containerComponentType:
		return cfg.Container
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
