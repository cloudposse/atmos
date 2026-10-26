package atmos

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// commandUsageMarkdown maps a dotted command path to the raw usage-example
// markdown embedded in the cmd/markdown package.
//
// Background: most commands' `Example` text is wired onto the *cobra.Command
// lazily, only when `--help` is actually rendered for that exact command
// (see cmd/root.go's SetHelpFunc, which sets `command.Example =
// exampleContent.Content` from an unexported map keyed by content name).
// That map is populated from a generic, comprehensive `//go:embed
// markdown/*` in cmd/markdown_help.go -- but that file lives in package
// `cmd`, which pkg/ai/tools/atmos cannot import at all (it would recreate
// the exact cmd -> cmd/mcp/server -> pkg/ai/tools/atmos -> cmd cycle
// discussed on ListCommandsTool / CommandNode).
//
// The `cmd/markdown` package (cmd/markdown/content.go) is a plain,
// non-"internal" leaf package (it only imports "embed"), so importing it
// here is safe -- no visibility rule applies and no cycle results, since
// nothing upstream of pkg/ai/tools/atmos imports cmd/markdown. It
// separately embeds a SMALL SUBSET of the same *.md files as individually
// named exported vars, for the handful of commands that read their own
// usage text directly at construction time (about, devcontainer, git, cast
// render) rather than relying on cmd/root.go's lazy wiring. This map
// captures that known subset by convention. It is NOT exhaustive -- most
// commands (e.g. "vendor pull") have no entry here, and
// resolveCommandExample falls back to reporting that no example is
// available for those. The cmd/markdown/content.go file itself is
// intentionally left untouched (out of scope for this change).
var commandUsageMarkdown = map[string]string{
	"about":                markdown.AboutMarkdown,
	"devcontainer":         markdown.DevcontainerUsageMarkdown,
	"devcontainer start":   markdown.DevcontainerStartUsageMarkdown,
	"devcontainer stop":    markdown.DevcontainerStopUsageMarkdown,
	"devcontainer attach":  markdown.DevcontainerAttachUsageMarkdown,
	"devcontainer list":    markdown.DevcontainerListUsageMarkdown,
	"devcontainer logs":    markdown.DevcontainerLogsUsageMarkdown,
	"devcontainer exec":    markdown.DevcontainerExecUsageMarkdown,
	"devcontainer remove":  markdown.DevcontainerRemoveUsageMarkdown,
	"devcontainer rebuild": markdown.DevcontainerRebuildUsageMarkdown,
	"devcontainer config":  markdown.DevcontainerConfigUsageMarkdown,
	"devcontainer shell":   markdown.DevcontainerShellUsageMarkdown,
	"git":                  markdown.AtmosGitMarkdown,
	"cast render":          markdown.CastRenderUsageMarkdown,
}

// CommandHelpTool returns detailed help (long description, usage examples,
// and flags) for a single Atmos CLI command, resolved from the same
// injected CommandNode tree used by ListCommandsTool. See CommandNode's
// doc comment for the import-cycle / visibility design behind that seam.
type CommandHelpTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewCommandHelpTool creates a new command help tool.
func NewCommandHelpTool(atmosConfig *schema.AtmosConfiguration) *CommandHelpTool {
	return &CommandHelpTool{atmosConfig: atmosConfig}
}

// Name returns the tool name.
func (t *CommandHelpTool) Name() string {
	return "atmos_command_help"
}

// Description returns the tool description.
func (t *CommandHelpTool) Description() string {
	return "Get detailed help for a specific Atmos CLI command: its long description, usage examples " +
		"(when available), and flags. Use atmos_list_commands first to discover the dotted command path " +
		"(e.g. \"vendor pull\", \"stack config set\"), then pass it here instead of shelling out to `--help`."
}

// Parameters returns the tool parameters.
func (t *CommandHelpTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "command",
			Description: "Dotted command path to look up, e.g. \"vendor pull\" or \"stack config set\".",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute runs the tool.
func (t *CommandHelpTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	command, ok := params["command"].(string)
	command = strings.TrimSpace(command)
	if !ok || command == "" {
		err := fmt.Errorf("%w: command", errUtils.ErrAIToolParameterRequired)
		return &tools.Result{Success: false, Error: err}, err
	}

	roots, err := commandTreeRoots()
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	node, err := findNodeByPath(roots, command)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return buildCommandHelpResult(command, node), nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *CommandHelpTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *CommandHelpTool) IsRestricted() bool {
	return false
}

// resolveCommandExample returns the best available usage-example text for
// node along with where it came from ("cobra" or "markdown"), or ("", "")
// if none is available. See the commandUsageMarkdown doc comment for why
// the markdown fallback only covers a small, known subset of commands.
func resolveCommandExample(path string, node *CommandNode) (example, source string) {
	if node.Example != "" {
		return node.Example, "cobra"
	}
	if content, ok := commandUsageMarkdown[path]; ok && content != "" {
		return content, "markdown"
	}
	return "", ""
}

// buildCommandHelpResult formats a resolved command node's help into a tools.Result.
func buildCommandHelpResult(path string, node *CommandNode) *tools.Result {
	long := node.Long
	if long == "" {
		long = node.Short
	}

	example, exampleSource := resolveCommandExample(path, node)

	var output strings.Builder
	fmt.Fprintf(&output, "atmos %s\n\n", path)
	if node.Short != "" {
		fmt.Fprintf(&output, "%s\n\n", node.Short)
	}
	if long != "" && long != node.Short {
		fmt.Fprintf(&output, "%s\n\n", long)
	}

	if example != "" {
		output.WriteString("Examples:\n")
		output.WriteString(example)
		output.WriteString("\n\n")
	} else {
		output.WriteString("Examples: none available for this command.\n\n")
	}

	writeCommandFlagsText(&output, node.Flags)

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"command":        path,
			"use":            node.Use,
			"short":          node.Short,
			"long":           long,
			"example":        example,
			"example_source": exampleSource,
			"flags":          buildCommandFlagsData(node.Flags),
		},
	}
}

// writeCommandFlagsText appends the human-readable "Flags:" section (or a
// none-defined fallback) for flags to output.
func writeCommandFlagsText(output *strings.Builder, flags []flagInfo) {
	if len(flags) == 0 {
		output.WriteString("Flags: none defined directly on this command.\n")
		return
	}

	output.WriteString("Flags:\n")
	for _, f := range flags {
		output.WriteString(formatCommandFlagLine(f))
	}
}

// formatCommandFlagLine renders a single flag as one "Flags:" line, choosing
// the layout based on whether it has a shorthand and/or a default value.
func formatCommandFlagLine(f flagInfo) string {
	switch {
	case f.Shorthand != "" && f.Default != "":
		return fmt.Sprintf("  -%s, --%-20s %s (default %s)\n", f.Shorthand, f.Name, f.Description, f.Default)
	case f.Shorthand != "":
		return fmt.Sprintf("  -%s, --%-20s %s\n", f.Shorthand, f.Name, f.Description)
	case f.Default != "":
		return fmt.Sprintf("      --%-20s %s (default %s)\n", f.Name, f.Description, f.Default)
	default:
		return fmt.Sprintf("      --%-20s %s\n", f.Name, f.Description)
	}
}

// buildCommandFlagsData converts flags into the structured Data["flags"] payload.
func buildCommandFlagsData(flags []flagInfo) []map[string]interface{} {
	flagsData := make([]map[string]interface{}, 0, len(flags))
	for _, f := range flags {
		flagsData = append(flagsData, map[string]interface{}{
			"name":        f.Name,
			"shorthand":   f.Shorthand,
			"description": f.Description,
			"default":     f.Default,
		})
	}
	return flagsData
}
