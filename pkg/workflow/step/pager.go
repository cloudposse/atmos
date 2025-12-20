package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// PagerHandler displays content in a scrollable pager.
type PagerHandler struct {
	BaseHandler
}

func init() {
	Register(&PagerHandler{
		BaseHandler: NewBaseHandler("pager", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
// Either content or path must be provided.
func (h *PagerHandler) Validate(step *schema.WorkflowStep) error {
	if step.Content == "" && step.Path == "" {
		return h.ValidateRequired(step, "content or path", "")
	}
	return nil
}

// Execute displays content in a pager.
func (h *PagerHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, resolvedPath, err := h.loadContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Render markdown if explicitly enabled or if file is a markdown file.
	if h.shouldRenderMarkdown(step, resolvedPath) {
		if rendered, renderErr := h.renderMarkdown(content); renderErr == nil {
			content = rendered
		}
	}

	title, err := h.resolveTitle(step, vars)
	if err != nil {
		return nil, err
	}

	// Create pager with pager enabled.
	p := pager.NewWithAtmosConfig(true)
	if err := p.Run(title, content); err != nil {
		return nil, fmt.Errorf("step '%s': pager failed: %w", step.Name, err)
	}

	return NewStepResult(content), nil
}

// loadContent loads content from file path or inline content field.
func (h *PagerHandler) loadContent(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (string, string, error) {
	if step.Path == "" {
		content, err := h.ResolveContent(ctx, step, vars)
		return content, "", err
	}

	resolvedPath, err := vars.Resolve(step.Path)
	if err != nil {
		return "", "", fmt.Errorf("step '%s': failed to resolve path: %w", step.Name, err)
	}

	content, err := h.readFile(resolvedPath, step)
	if err != nil {
		return "", "", err
	}

	return content, resolvedPath, nil
}

// resolveTitle resolves the pager title from step config.
func (h *PagerHandler) resolveTitle(step *schema.WorkflowStep, vars *Variables) (string, error) {
	if step.Title == "" {
		if step.Path != "" {
			return filepath.Base(step.Path), nil
		}
		return "", nil
	}

	title, err := vars.Resolve(step.Title)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve title: %w", step.Name, err)
	}
	return title, nil
}

// shouldRenderMarkdown determines if content should be rendered as markdown.
func (h *PagerHandler) shouldRenderMarkdown(step *schema.WorkflowStep, path string) bool {
	// Explicit markdown flag takes precedence.
	if step.Markdown {
		return true
	}
	// Auto-detect markdown files by extension.
	if path != "" {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".md" || ext == ".markdown"
	}
	return false
}

// renderMarkdown renders markdown content using glamour.
func (h *PagerHandler) renderMarkdown(content string) (string, error) {
	var opts []glamour.TermRendererOption

	// Use theme-aware styles.
	if glamourStyle, err := theme.GetGlamourStyleForTheme(theme.DefaultThemeName); err == nil {
		opts = append(opts, glamour.WithStylesFromJSONBytes(glamourStyle))
	}

	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return content, err
	}

	return renderer.Render(content)
}

// readFile reads content from a file path.
func (h *PagerHandler) readFile(path string, step *schema.WorkflowStep) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to read file '%s': %w", step.Name, path, err)
	}

	return string(data), nil
}
