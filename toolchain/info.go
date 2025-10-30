package toolchain

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// InfoExec handles the core logic for retrieving and formatting tool information.
func InfoExec(toolName, outputFormat string) error {
	defer perf.Track(nil, "toolchain.InfoExec")()

	ctx := context.Background()

	// Create installer inside the function.
	installer := NewInstaller()

	// Parse tool name to get owner/repo.
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return fmt.Errorf("failed to resolve tool '%s': %w", toolName, err)
	}

	// Get installed versions from tool-versions file.
	installedVersions := []string{}
	defaultVersion := ""
	if toolVersions, err := LoadToolVersions(GetToolVersionsFilePath()); err == nil {
		if versions, exists := toolVersions.Tools[toolName]; exists && len(versions) > 0 {
			installedVersions = versions
			defaultVersion = versions[len(versions)-1] // Last one is default.
		}
	}

	// Use default version or pick a reasonable one.
	version := defaultVersion
	if version == "" {
		version = "latest" // Fallback.
	}

	// Find the tool configuration.
	tool, err := installer.findTool(owner, repo, version)
	if err != nil {
		return fmt.Errorf("failed to find tool %s: %w", toolName, err)
	}

	// Get registry metadata to show which registry this came from.
	var registryName string
	reg := NewAquaRegistry()
	if meta, err := reg.GetMetadata(ctx); err == nil {
		registryName = meta.Name
	}

	// Try to get available versions from GitHub.
	availableVersions := []string{}
	if ghVersions, err := fetchGitHubVersions(owner, repo); err == nil {
		// Show latest 10 versions.
		limit := 10
		if len(ghVersions) < limit {
			limit = len(ghVersions)
		}
		for i := 0; i < limit; i++ {
			availableVersions = append(availableVersions, ghVersions[i].version)
		}
	}

	// Display output based on format.
	if outputFormat == "yaml" {
		evaluatedYAML, err := getEvaluatedToolYAML(tool, version, installer)
		if err != nil {
			return fmt.Errorf("failed to get evaluated YAML: %w", err)
		}
		fmt.Print(evaluatedYAML)
	} else {
		// Enhanced table format (default).
		markdown := formatEnhancedToolInfo(&toolContext{
			Name:              toolName,
			Owner:             owner,
			Repo:              repo,
			Tool:              tool,
			Version:           version,
			Installer:         installer,
			Registry:          registryName,
			InstalledVersions: installedVersions,
			AvailableVersions: availableVersions,
			DefaultVersion:    defaultVersion,
		})
		u.PrintfMarkdownToTUI(markdown)
	}

	return nil
}

type toolContext struct {
	Name              string
	Owner             string
	Repo              string
	Version           string
	Tool              *registry.Tool
	Installer         *Installer
	Registry          string
	InstalledVersions []string
	AvailableVersions []string
	DefaultVersion    string
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

// formatEnhancedToolInfo formats tool information with registry data and version info.
func formatEnhancedToolInfo(ctx *toolContext) string {
	var output strings.Builder

	formatToolHeader(&output, ctx)
	formatInstalledVersions(&output, ctx)
	formatAvailableVersions(&output, ctx)
	formatInstallExamples(&output, ctx)

	return output.String()
}

func formatToolHeader(output *strings.Builder, ctx *toolContext) {
	fmt.Fprintf(output, "**Tool:** %s/%s\n", ctx.Owner, ctx.Repo)
	if ctx.Registry != "" {
		fmt.Fprintf(output, "Registry: %s\n", ctx.Registry)
	}
	fmt.Fprintf(output, "Type: %s\n\n", ctx.Tool.Type)

	if ctx.Tool.RepoOwner != "" && ctx.Tool.RepoName != "" {
		fmt.Fprintf(output, "Repository: https://github.com/%s/%s\n\n", ctx.Tool.RepoOwner, ctx.Tool.RepoName)
	}
}

func formatInstalledVersions(output *strings.Builder, ctx *toolContext) {
	if len(ctx.InstalledVersions) == 0 {
		output.WriteString("No versions installed\n\n")
		return
	}

	output.WriteString("**Installed Versions:**\n")
	for _, v := range ctx.InstalledVersions {
		marker := " "
		if v == ctx.DefaultVersion {
			marker = theme.Styles.Checkmark.String()
		}
		fmt.Fprintf(output, "  %s %s", marker, v)
		if v == ctx.DefaultVersion {
			output.WriteString(" (default)")
		}
		output.WriteString("\n")
	}
	output.WriteString("\n")
}

func formatAvailableVersions(output *strings.Builder, ctx *toolContext) {
	if len(ctx.AvailableVersions) == 0 {
		return
	}

	output.WriteString("**Available Versions** (latest 10):\n")
	for _, v := range ctx.AvailableVersions {
		installed := isVersionInstalled(v, ctx.InstalledVersions)
		marker := " "
		if installed {
			marker = theme.Styles.Checkmark.String()
		}
		fmt.Fprintf(output, "  %s %s", marker, v)
		if installed {
			output.WriteString(" (installed)")
		}
		output.WriteString("\n")
	}
	output.WriteString("\n")
}

func isVersionInstalled(version string, installedVersions []string) bool {
	for _, iv := range installedVersions {
		if iv == version {
			return true
		}
	}
	return false
}

func formatInstallExamples(output *strings.Builder, ctx *toolContext) {
	output.WriteString("**Install:**\n")
	fmt.Fprintf(output, "  atmos toolchain install %s@%s\n", ctx.Name, ctx.Version)
	if len(ctx.AvailableVersions) > 0 {
		fmt.Fprintf(output, "  atmos toolchain install %s@%s\n", ctx.Name, ctx.AvailableVersions[0])
	}
	fmt.Fprintf(output, "  atmos toolchain install %s@latest\n", ctx.Name)
}

// getEvaluatedToolYAML returns the tool configuration as YAML with all templates evaluated.
func getEvaluatedToolYAML(tool *registry.Tool, version string, installer *Installer) (string, error) {
	// Build the asset URL to evaluate templates (if Asset is set)
	assetURL := ""
	var err error
	if tool.Asset != "" {
		assetURL, err = installer.buildAssetURL(tool, version)
		if err != nil {
			return "", fmt.Errorf("failed to build asset URL: %w", err)
		}
	}

	// Create a copy of the tool with evaluated templates
	evaluatedTool := struct {
		Type         string              `yaml:"type"`
		RepoOwner    string              `yaml:"repo_owner"`
		RepoName     string              `yaml:"repo_name"`
		Asset        string              `yaml:"asset"`
		URL          string              `yaml:"url,omitempty"`
		Format       string              `yaml:"format,omitempty"`
		BinaryName   string              `yaml:"binary_name,omitempty"`
		Files        []registry.File     `yaml:"files,omitempty"`
		Overrides    []registry.Override `yaml:"overrides,omitempty"`
		Version      string              `yaml:"version"`
		ProcessedURL string              `yaml:"processed_url,omitempty"`
	}{
		Type:         tool.Type,
		RepoOwner:    tool.RepoOwner,
		RepoName:     tool.RepoName,
		Asset:        assetURL, // Use evaluated URL instead of template
		URL:          tool.URL,
		Format:       tool.Format,
		BinaryName:   tool.BinaryName,
		Files:        tool.Files,
		Overrides:    tool.Overrides,
		Version:      version,
		ProcessedURL: assetURL,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(evaluatedTool)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool to YAML: %w", err)
	}

	return string(data), nil
}
