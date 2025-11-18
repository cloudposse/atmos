package show

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/profile"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	newline = "\n"
)

// RenderProfile renders a single profile with detailed information.
func RenderProfile(p *profile.ProfileInfo) (string, error) {
	defer perf.Track(nil, "profile.show.RenderProfile")()

	var output strings.Builder
	styles := theme.GetCurrentStyles()

	// Profile header.
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetPrimaryColor())).
		Bold(true).
		Underline(true)

	output.WriteString(headerStyle.Render("PROFILE: " + p.Name))
	output.WriteString(newline)
	output.WriteString(newline)

	// Location information.
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetPrimaryColor())).
		Bold(true)

	output.WriteString(labelStyle.Render("Location Type: "))
	output.WriteString(p.LocationType)
	output.WriteString(newline)

	output.WriteString(labelStyle.Render("Path:          "))
	output.WriteString(p.Path)
	output.WriteString(newline)
	output.WriteString(newline)

	// Metadata (if available).
	if p.Metadata != nil {
		renderMetadata(&output, p.Metadata, &headerStyle, &labelStyle, styles)
	}

	// Files.
	output.WriteString(headerStyle.Render("FILES"))
	output.WriteString(newline)
	output.WriteString(newline)

	if len(p.Files) == 0 {
		output.WriteString(styles.Muted.Render("  No configuration files found"))
		output.WriteString(newline)
	} else {
		fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetSuccessColor()))
		for _, file := range p.Files {
			output.WriteString("  ")
			output.WriteString(fileStyle.Render("✓ "))
			output.WriteString(file)
			output.WriteString(newline)
		}
	}

	output.WriteString(newline)

	// Usage hint.
	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetPrimaryColor())).
		Italic(true)

	output.WriteString(hintStyle.Render(fmt.Sprintf("Use with: atmos --profile %s <command>", p.Name)))
	output.WriteString(newline)

	return output.String(), nil
}

// renderMetadata renders profile metadata to the output builder.
func renderMetadata(output *strings.Builder, metadata *schema.ConfigMetadata, headerStyle *lipgloss.Style, labelStyle *lipgloss.Style, styles *theme.StyleSet) {
	output.WriteString(headerStyle.Render("METADATA"))
	output.WriteString(newline)
	output.WriteString(newline)

	if metadata.Name != "" {
		output.WriteString(labelStyle.Render("Name:        "))
		output.WriteString(metadata.Name)
		output.WriteString(newline)
	}

	if metadata.Description != "" {
		output.WriteString(labelStyle.Render("Description: "))
		output.WriteString(metadata.Description)
		output.WriteString(newline)
	}

	if metadata.Version != "" {
		output.WriteString(labelStyle.Render("Version:     "))
		output.WriteString(metadata.Version)
		output.WriteString(newline)
	}

	if len(metadata.Tags) > 0 {
		output.WriteString(labelStyle.Render("Tags:        "))
		output.WriteString(strings.Join(metadata.Tags, ", "))
		output.WriteString(newline)
	}

	if metadata.Deprecated {
		output.WriteString(labelStyle.Render("Status:      "))
		output.WriteString(styles.Warning.Render("DEPRECATED"))
		output.WriteString(newline)
	}

	output.WriteString(newline)
}
