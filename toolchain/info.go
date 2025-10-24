package toolchain

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
)

// InfoExec handles the core logic for retrieving and formatting tool information.
func InfoExec(toolName, outputFormat string) error {
	defer perf.Track(nil, "toolchain.RunInfo")()

	// Create installer inside the function
	installer := NewInstaller()

	// Parse tool name to get owner/repo
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return fmt.Errorf("failed to resolve tool '%s': %w", toolName, err)
	}

	// Get a real version from tool-versions file or use a default
	version := "1.11.4" // Use a real version instead of "latest"

	// Try to get the latest installed version from tool-versions file
	if toolVersions, err := LoadToolVersions(GetToolVersionsFilePath()); err == nil {
		if versions, exists := toolVersions.Tools[toolName]; exists && len(versions) > 0 {
			version = versions[len(versions)-1] // Use the most recent version
		}
	}

	// Find the tool configuration
	tool, err := installer.findTool(owner, repo, version)
	if err != nil {
		return fmt.Errorf("failed to find tool %s: %w", toolName, err)
	}

	// Get evaluated YAML with templates processed
	evaluatedYAML, err := getEvaluatedToolYAML(tool, version, installer)
	if err != nil {
		return fmt.Errorf("failed to get evaluated YAML: %w", err)
	}

	// Display output based on format
	if outputFormat == "yaml" {
		fmt.Print(evaluatedYAML)
	} else {
		// Table format (default)
		table := formatToolInfoAsTable(toolContext{Name: toolName, Owner: owner, Repo: repo, Tool: tool, Version: version, Installer: installer})
		fmt.Print(table)
	}

	return nil
}

type toolContext struct {
	Name      string
	Owner     string
	Repo      string
	Version   string
	Tool      *Tool
	Installer *Installer
}

func formatToolInfoAsTable(ctx toolContext) string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Width(15)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	var rows []string

	rows = append(rows, labelStyle.Render("Tool:")+valueStyle.Render(ctx.Name))
	rows = append(rows, labelStyle.Render("Owner/Repo:")+valueStyle.Render(ctx.Owner+"/"+ctx.Repo))
	rows = append(rows, labelStyle.Render("Type:")+valueStyle.Render(ctx.Tool.Type))
	rows = append(rows, labelStyle.Render("Repository:")+valueStyle.Render(ctx.Tool.RepoOwner+"/"+ctx.Tool.RepoName))
	rows = append(rows, labelStyle.Render("Version:")+valueStyle.Render(ctx.Version))

	if ctx.Tool.Format != "" {
		rows = append(rows, labelStyle.Render("Format:")+valueStyle.Render(ctx.Tool.Format))
	}
	if ctx.Tool.BinaryName != "" {
		rows = append(rows, labelStyle.Render("Binary Name:")+valueStyle.Render(ctx.Tool.BinaryName))
	}

	if ctx.Tool.Asset != "" {
		rows = append(rows, labelStyle.Render("Asset Template:")+valueStyle.Render(ctx.Tool.Asset))
	}
	if ctx.Tool.URL != "" && ctx.Tool.URL != ctx.Tool.Asset {
		rows = append(rows, labelStyle.Render("URL Template:")+valueStyle.Render(ctx.Tool.URL))
	}

	if ctx.Tool.Asset != "" || ctx.Tool.URL != "" {
		processedURL, err := ctx.Installer.buildAssetURL(ctx.Tool, ctx.Version)
		if err == nil {
			rows = append(rows, labelStyle.Render("Processed URL:")+valueStyle.Render(processedURL))
		} else {
			rows = append(rows, labelStyle.Render("Processed URL:")+valueStyle.Render("Error: "+err.Error()))
		}
	}

	for _, file := range ctx.Tool.Files {
		rows = append(rows, labelStyle.Render("File:")+valueStyle.Render(file.Src+" -> "+file.Name))
	}

	for _, override := range ctx.Tool.Overrides {
		rows = append(rows, labelStyle.Render("Override:")+valueStyle.Render(override.GOOS+"/"+override.GOARCH+": "+override.Asset))
	}

	return strings.Join(rows, "\n") + "\n"
}
