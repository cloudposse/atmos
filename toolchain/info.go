package toolchain

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// InfoExec handles the core logic for retrieving and formatting tool information
func InfoExec(toolName, outputFormat string) error {
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
		table := formatToolInfoAsTable(toolName, owner, repo, tool, version, installer)
		fmt.Print(table)
	}

	return nil
}

// formatToolInfoAsTable formats tool information as a proper table using lipgloss
func formatToolInfoAsTable(toolName, owner, repo string, tool *Tool, version string, installer *Installer) string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Width(15)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	// Build table rows
	var rows []string

	// Basic info - always show these
	rows = append(rows, labelStyle.Render("Tool:")+valueStyle.Render(toolName))
	rows = append(rows, labelStyle.Render("Owner/Repo:")+valueStyle.Render(owner+"/"+repo))
	rows = append(rows, labelStyle.Render("Type:")+valueStyle.Render(tool.Type))
	rows = append(rows, labelStyle.Render("Repository:")+valueStyle.Render(tool.RepoOwner+"/"+tool.RepoName))
	rows = append(rows, labelStyle.Render("Version:")+valueStyle.Render(version))

	// Optional fields - show if present
	if tool.Format != "" {
		rows = append(rows, labelStyle.Render("Format:")+valueStyle.Render(tool.Format))
	}
	if tool.BinaryName != "" {
		rows = append(rows, labelStyle.Render("Binary Name:")+valueStyle.Render(tool.BinaryName))
	}

	// Template info - show raw templates
	if tool.Asset != "" {
		rows = append(rows, labelStyle.Render("Asset Template:")+valueStyle.Render(tool.Asset))
	}
	if tool.URL != "" && tool.URL != tool.Asset {
		rows = append(rows, labelStyle.Render("URL Template:")+valueStyle.Render(tool.URL))
	}

	// Processed URL - show evaluated template
	if tool.Asset != "" || tool.URL != "" {
		processedURL, err := installer.buildAssetURL(tool, version)
		if err == nil {
			rows = append(rows, labelStyle.Render("Processed URL:")+valueStyle.Render(processedURL))
		} else {
			rows = append(rows, labelStyle.Render("Processed URL:")+valueStyle.Render("Error: "+err.Error()))
		}
	}

	// Files
	if len(tool.Files) > 0 {
		for _, file := range tool.Files {
			rows = append(rows, labelStyle.Render("File:")+valueStyle.Render(file.Src+" -> "+file.Name))
		}
	}

	// Overrides
	if len(tool.Overrides) > 0 {
		for _, override := range tool.Overrides {
			rows = append(rows, labelStyle.Render("Override:")+valueStyle.Render(override.GOOS+"/"+override.GOARCH+": "+override.Asset))
		}
	}

	return strings.Join(rows, "\n")
}
