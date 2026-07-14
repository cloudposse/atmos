package atmos

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// paramTool filters toolchain list output to a single configured tool.
const paramTool = "tool"

// ToolchainListTool lists tools configured in the project's .tool-versions file.
type ToolchainListTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewToolchainListTool creates a new toolchain list tool.
func NewToolchainListTool(atmosConfig *schema.AtmosConfiguration) *ToolchainListTool {
	return &ToolchainListTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ToolchainListTool) Name() string {
	return "atmos_toolchain_list"
}

// Description returns the tool description.
func (t *ToolchainListTool) Description() string {
	return "List tools declared in the project's .tool-versions file, their configured versions " +
		"(default and additional), and whether each version is installed locally. Read-only."
}

// Parameters returns the tool parameters.
func (t *ToolchainListTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramTool,
			Description: "Show only this configured tool name (omit to show all configured tools).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// toolchainVersionEntry describes a single configured tool/version pair.
type toolchainVersionEntry struct {
	Tool      string `yaml:"tool" json:"tool"`
	Version   string `yaml:"version" json:"version"`
	Default   bool   `yaml:"default" json:"default"`
	Installed bool   `yaml:"installed" json:"installed"`
}

// Execute runs the tool.
func (t *ToolchainListTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	toolFilter, _ := params[paramTool].(string)

	toolVersionsFile := toolchain.GetToolVersionsFilePath()
	toolVersions, err := toolchain.LoadToolVersions(toolVersionsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &tools.Result{
				Success: true,
				Output:  fmt.Sprintf("No .tool-versions file found at %s; no tools configured.", toolVersionsFile),
				Data: map[string]interface{}{
					"tool_versions_file": toolVersionsFile,
					"tools":              []toolchainVersionEntry{},
				},
			}, nil
		}
		return &tools.Result{Success: false, Error: err}, err
	}

	entries := buildToolchainVersionEntries(toolVersions, toolFilter)

	return buildToolchainListResult(toolVersionsFile, entries), nil
}

// buildToolchainVersionEntries resolves installation status for every configured tool/version,
// optionally narrowed to a single tool name.
func buildToolchainVersionEntries(toolVersions *toolchain.ToolVersions, toolFilter string) []toolchainVersionEntry {
	installer := toolchain.NewInstaller()
	resolver := installer.GetResolver()

	names := make([]string, 0, len(toolVersions.Tools))
	for name := range toolVersions.Tools {
		if toolFilter != "" && name != toolFilter {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var entries []toolchainVersionEntry
	for _, name := range names {
		versions := toolVersions.Tools[name]
		owner, repo := resolveToolchainOwnerRepo(name, toolVersions, resolver)
		for i, version := range versions {
			entries = append(entries, toolchainVersionEntry{
				Tool:      name,
				Version:   version,
				Default:   i == 0,
				Installed: toolchainVersionInstalled(installer, owner, repo, version),
			})
		}
	}
	return entries
}

// resolveToolchainOwnerRepo resolves a configured tool name to its owner/repo, honoring aliases.
// Returns empty strings when resolution fails (e.g. an unknown tool/alias).
func resolveToolchainOwnerRepo(name string, toolVersions *toolchain.ToolVersions, resolver toolchain.ToolResolver) (string, string) {
	resolvedKey, _, found := toolchain.LookupToolVersion(name, toolVersions, resolver)
	if !found {
		return "", ""
	}
	owner, repo, err := resolver.Resolve(resolvedKey)
	if err != nil {
		return "", ""
	}
	return owner, repo
}

// toolchainVersionInstalled reports whether a specific tool version has a binary installed locally.
func toolchainVersionInstalled(installer *toolchain.Installer, owner, repo, version string) bool {
	if owner == "" || repo == "" {
		return false
	}
	_, err := installer.FindBinaryPath(owner, repo, version)
	return err == nil
}

// buildToolchainListResult formats toolchain entries into a tools.Result.
func buildToolchainListResult(toolVersionsFile string, entries []toolchainVersionEntry) *tools.Result {
	var output strings.Builder
	fmt.Fprintf(&output, "Configured Tools (%s):\n\n", toolVersionsFile)

	if len(entries) == 0 {
		output.WriteString("(none)\n")
	}

	for _, e := range entries {
		tags := make([]string, 0, 2)
		if e.Default {
			tags = append(tags, "default")
		}
		if e.Installed {
			tags = append(tags, "installed")
		} else {
			tags = append(tags, "not installed")
		}
		fmt.Fprintf(&output, "  - %s@%s (%s)\n", e.Tool, e.Version, strings.Join(tags, ", "))
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"tool_versions_file": toolVersionsFile,
			"tools":              entries,
		},
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *ToolchainListTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ToolchainListTool) IsRestricted() bool {
	return false
}
