package step

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// notableInspectLabels are OCI image labels worth surfacing in the rendered
// metadata (rendered in this order, when present).
var notableInspectLabels = []struct {
	key   string
	label string
}{
	{"org.opencontainers.image.title", "Title"},
	{"org.opencontainers.image.version", "Version"},
	{"org.opencontainers.image.revision", "Revision"},
	{"org.opencontainers.image.source", "Source"},
}

// validateInspectAction checks the configuration of an `inspect` container step.
func (h *ContainerHandler) validateInspectAction(step *schema.WorkflowStep) error {
	inspect := effectiveInspectStep(step)
	if err := h.ValidateRequired(step, "inspect.image", inspect.Image); err != nil {
		return err
	}
	if !isValidContainerRuntime(inspect.Provider) {
		return invalidContainerField(step, "inspect.provider", inspect.Provider, "Provider must be `docker`, `podman`, or empty for auto-detect")
	}
	return nil
}

func (h *ContainerHandler) executeInspect(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	inspect := effectiveInspectStep(step)
	image, err := resolveOptional(vars, inspect.Image, "inspect.image", step.Name)
	if err != nil {
		return nil, err
	}

	if step.DryRun {
		ui.Writeln(fmt.Sprintf("inspect image %s", image))
		return NewStepResult(image).WithMetadata(exitCodeMetadata, 0).WithMetadata("image", image), nil
	}

	runtime, err := container.DetectRuntimeWithPreferenceAndRecovery(ctx, strings.TrimSpace(inspect.Provider), inspect.RuntimeAutoStart)
	if err != nil {
		return nil, err
	}
	applyRuntimeEnv(runtime, vars)

	info, err := runtime.ImageInspect(ctx, image)
	if err != nil {
		return NewStepResult(image).WithMetadata(exitCodeMetadata, 1).WithError(err.Error()), err
	}

	ui.Writeln(renderImageInspect(image, info))

	return NewStepResult(image).
		WithMetadata(exitCodeMetadata, 0).
		WithMetadata("image", image).
		WithMetadata("image_id", info.ID).
		WithMetadata("repo_digests", info.RepoDigests).
		WithMetadata("size", info.Size), nil
}

func effectiveInspectStep(step *schema.WorkflowStep) schema.ContainerInspectStep {
	inspect := schema.ContainerInspectStep{}
	if step.Inspect != nil {
		inspect = *step.Inspect
	}
	if inspect.Image == "" {
		inspect.Image = step.Image
	}
	if inspect.Provider == "" {
		inspect.Provider = step.Provider
	}
	inspect.RuntimeAutoStart = inspect.RuntimeAutoStart || step.RuntimeAutoStart
	return inspect
}

// renderImageInspect builds a curated borderless two-column view of image metadata.
// It selects the fields a person actually wants after a build (identity, provenance,
// size, platform) rather than dumping the full inspect JSON. Keys are bold and
// left-aligned; the column width is driven by the longest plain key so values
// start at a consistent offset.
func renderImageInspect(image string, info *container.ImageInfo) string {
	rows := collectInspectRows(info)

	// Determine column width from the longest plain key (no ANSI).
	colWidth := 0
	for _, r := range rows {
		if w := lipgloss.Width(r.key); w > colWidth {
			colWidth = w
		}
	}

	// Obtain theme styles for bold keys and styled title; degrade gracefully when
	// styles are unavailable (non-TTY, CI, no-color).
	styles := theme.GetCurrentStyles()

	// Build title line.
	var b strings.Builder
	if styles != nil {
		fmt.Fprintf(&b, "%s %s\n\n", styles.Label.Render("Image:"), styles.Title.Render(image))
	} else {
		fmt.Fprintf(&b, "Image: %s\n\n", image)
	}

	// Build each data row. Padding is applied to the plain key text; the bold
	// style is then wrapped around the padded string so ANSI codes do not
	// distort the column alignment.
	for _, r := range rows {
		plainKey := r.key + strings.Repeat(" ", colWidth-lipgloss.Width(r.key))
		var styledKey string
		if styles != nil {
			styledKey = styles.Label.Render(plainKey)
		} else {
			styledKey = plainKey
		}
		fmt.Fprintf(&b, "%s  %s\n", styledKey, r.value)
	}

	return b.String()
}

// inspectRow is a rendered key/value pair for the inspect view.
type inspectRow struct{ key, value string }

// collectInspectRows selects the curated, non-empty image-metadata fields to
// render (identity, provenance, size, platform, notable OCI labels).
func collectInspectRows(info *container.ImageInfo) []inspectRow {
	var rows []inspectRow
	add := func(key, value string) {
		if value != "" {
			rows = append(rows, inspectRow{key, value})
		}
	}

	add("ID", shortDigest(info.ID))
	add("Digest", shortDigest(firstString(info.RepoDigests)))
	if len(info.RepoTags) > 0 {
		add("Tags", strings.Join(info.RepoTags, ", "))
	}
	add("Created", formatInspectTime(info.Created))
	add("Size", humanizeBytes(info.Size))
	add("Platform", platformString(info.Os, info.Architecture))
	if info.Layers > 0 {
		add("Layers", fmt.Sprintf("%d", info.Layers))
	}
	if len(info.Labels) > 0 {
		add("Labels", fmt.Sprintf("%d", len(info.Labels)))
	}
	for _, l := range notableInspectLabels {
		add(l.label, info.Labels[l.key])
	}
	return rows
}

// shortDigest truncates a `sha256:<hex>` reference (optionally prefixed with a
// repository, as in RepoDigests) to a readable 12-hex-character form.
func shortDigest(value string) string {
	if value == "" {
		return ""
	}
	const shortLen = 12
	if i := strings.Index(value, "sha256:"); i >= 0 {
		hex := value[i+len("sha256:"):]
		if len(hex) > shortLen {
			hex = hex[:shortLen]
		}
		return "sha256:" + hex
	}
	// A bare hex digest (podman's `Id` has no algorithm prefix) — truncate it too.
	if len(value) > shortLen && isHexString(value) {
		return value[:shortLen]
	}
	return value
}

// isHexString reports whether s consists solely of hexadecimal characters.
func isHexString(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return s != ""
}

// formatInspectTime normalizes the inspect `Created` timestamp to a stable
// absolute UTC string, falling back to the raw value when it cannot be parsed.
func formatInspectTime(value string) string {
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC().Format("2006-01-02 15:04:05 UTC")
		}
	}
	return value
}

// humanizeBytes renders a byte count in binary (KiB-style) units.
func humanizeBytes(b int64) string {
	if b <= 0 {
		return ""
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// platformString joins OS and architecture as `os/arch`, tolerating either being empty.
func platformString(os, arch string) string {
	switch {
	case os != "" && arch != "":
		return os + "/" + arch
	case os != "":
		return os
	default:
		return arch
	}
}
