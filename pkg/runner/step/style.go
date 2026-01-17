package step

import (
	"context"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// StyleHandler applies terminal styling to text (like gum style).
type StyleHandler struct {
	BaseHandler
}

func init() {
	Register(&StyleHandler{
		BaseHandler: NewBaseHandler("style", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
func (h *StyleHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.StyleHandler.Validate")()

	return h.ValidateRequired(step, "content", step.Content)
}

// Execute applies styling and displays the content.
func (h *StyleHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.StyleHandler.Execute")()

	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			Err()
	}

	// Render markdown if enabled.
	//nolint:nestif // Width calculation requires nested conditionals.
	if step.Markdown {
		// Calculate content width accounting for border and padding.
		renderWidth := step.Width
		if renderWidth > 0 {
			// Account for border (1 char per side).
			if step.Border != "" && step.Border != "none" {
				renderWidth -= 2
			}
			// Account for horizontal padding.
			spacing := parseSpacing(step.Padding)
			renderWidth -= (spacing.Right + spacing.Left)
		}

		rendered, err := h.renderMarkdown(content, renderWidth)
		if err == nil {
			content = strings.TrimSpace(rendered)
		}
	}

	// Build the style from step configuration.
	style := h.buildStyle(step)

	// Render the styled content.
	output := style.Render(content)

	if err := ui.Writeln(output); err != nil {
		return nil, errUtils.Build(errUtils.ErrWriteToStream).
			WithCause(err).
			WithContext("step", step.Name).
			Err()
	}

	return NewStepResult(content), nil
}

// renderMarkdown renders markdown content with optional width constraint.
func (h *StyleHandler) renderMarkdown(content string, width int) (string, error) {
	var opts []glamour.TermRendererOption

	if width > 0 {
		opts = append(opts, glamour.WithWordWrap(width))
	}

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

// buildStyle creates a lipgloss style from step configuration.
//
//nolint:gocognit,cyclop,revive,funlen // Complex styling options require multiple conditionals.
func (h *StyleHandler) buildStyle(step *schema.WorkflowStep) lipgloss.Style {
	style := lipgloss.NewStyle()

	// Colors.
	if step.Foreground != "" {
		style = style.Foreground(lipgloss.Color(step.Foreground))
	}
	if step.Background != "" {
		style = style.Background(lipgloss.Color(step.Background))
	}

	// Border.
	//nolint:nestif // Border configuration needs nested color handling.
	if step.Border != "" && step.Border != "none" {
		border := getBorderStyle(step.Border)
		style = style.Border(border)

		// Border colors.
		if step.BorderForeground != "" {
			style = style.BorderForeground(lipgloss.Color(step.BorderForeground))
		} else {
			// Default to theme border color.
			styles := theme.GetCurrentStyles()
			if styles != nil {
				style = style.BorderForeground(styles.TableBorder.GetForeground())
			}
		}
		if step.BorderBackground != "" {
			style = style.BorderBackground(lipgloss.Color(step.BorderBackground))
		}
	}

	// Padding (supports "1", "1 2", "1 2 3", "1 2 3 4" formats).
	if step.Padding != "" {
		spacing := parseSpacing(step.Padding)
		style = style.Padding(spacing.Top, spacing.Right, spacing.Bottom, spacing.Left)
	}

	// Margin (supports "1", "1 2", "1 2 3", "1 2 3 4" formats).
	if step.Margin != "" {
		spacing := parseSpacing(step.Margin)
		style = style.Margin(spacing.Top, spacing.Right, spacing.Bottom, spacing.Left)
	}

	// Dimensions.
	if step.Width > 0 {
		style = style.Width(step.Width)
	}
	if step.Height > 0 {
		style = style.Height(step.Height)
	}

	// Alignment.
	if step.Align != "" {
		switch strings.ToLower(step.Align) {
		case "center":
			style = style.Align(lipgloss.Center)
		case "right":
			style = style.Align(lipgloss.Right)
		case "left":
			style = style.Align(lipgloss.Left)
		}
	}

	// Text decorations.
	if step.Bold {
		style = style.Bold(true)
	}
	if step.Italic {
		style = style.Italic(true)
	}
	if step.Underline {
		style = style.Underline(true)
	}
	if step.Strikethrough {
		style = style.Strikethrough(true)
	}
	if step.Faint {
		style = style.Faint(true)
	}

	return style
}

// getBorderStyle returns the lipgloss border for the given style name.
func getBorderStyle(name string) lipgloss.Border {
	switch strings.ToLower(name) {
	case "normal":
		return lipgloss.NormalBorder()
	case "thick":
		return lipgloss.ThickBorder()
	case "double":
		return lipgloss.DoubleBorder()
	case "hidden":
		return lipgloss.HiddenBorder()
	case "rounded", "":
		return lipgloss.RoundedBorder()
	default:
		return lipgloss.RoundedBorder()
	}
}

// Spacing represents padding or margin values for top, right, bottom, left.
type Spacing struct {
	Top    int
	Right  int
	Bottom int
	Left   int
}

// parseSpacing parses a spacing string like "1", "1 2", or "1 2 1 2".
// Returns a Spacing struct with top, right, bottom, left values.
func parseSpacing(s string) Spacing {
	parts := strings.Fields(s)
	values := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			v = 0
		}
		values[i] = v
	}

	switch len(values) {
	case 1:
		// All sides same.
		return Spacing{values[0], values[0], values[0], values[0]}
	case 2:
		// Vertical, horizontal.
		return Spacing{values[0], values[1], values[0], values[1]}
	case 3:
		// Top, horizontal, bottom.
		return Spacing{values[0], values[1], values[2], values[1]}
	case 4:
		// Top, right, bottom, left.
		return Spacing{values[0], values[1], values[2], values[3]}
	default:
		return Spacing{0, 0, 0, 0}
	}
}
