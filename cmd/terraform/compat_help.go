package terraform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// FormatCompatFlagsHelpFromMap formats compatibility flags for help output using the
// Description field directly from CompatibilityFlag. This ensures descriptions
// stay in sync with flag definitions.
func FormatCompatFlagsHelpFromMap(flags map[string]compat.CompatibilityFlag) string {
	if len(flags) == 0 {
		return ""
	}

	// Collect flags and sort by name.
	type flagEntry struct {
		name        string
		description string
	}
	entries := make([]flagEntry, 0, len(flags))
	for name, flag := range flags {
		entries = append(entries, flagEntry{name: name, description: flag.Description})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	var builder strings.Builder
	builder.WriteString("\nTerraform/OpenTofu Native Flags:\n")
	builder.WriteString("  These flags are passed through to the underlying terraform/tofu command.\n\n")

	// Find maximum flag length for alignment.
	maxLen := 0
	for _, entry := range entries {
		if len(entry.name) > maxLen {
			maxLen = len(entry.name)
		}
	}

	// Format each flag.
	for _, entry := range entries {
		padding := strings.Repeat(" ", maxLen-len(entry.name)+2)
		builder.WriteString(fmt.Sprintf("      %s%s%s\n", entry.name, padding, entry.description))
	}

	return builder.String()
}

// GetCompatFlagsForCommand returns the compatibility flags for a specific terraform subcommand.
func GetCompatFlagsForCommand(subCommand string) map[string]compat.CompatibilityFlag {
	switch subCommand {
	case "plan":
		return PlanCompatFlags()
	case "apply", "deploy":
		return ApplyCompatFlags()
	case "destroy":
		return DestroyCompatFlags()
	case "init":
		return InitCompatFlags()
	case "validate":
		return ValidateCompatFlags()
	case "refresh":
		return RefreshCompatFlags()
	case "output":
		return OutputCompatFlags()
	case "show":
		return ShowCompatFlags()
	case "state":
		return StateCompatFlags()
	case "import":
		return ImportCompatFlags()
	case "taint":
		return TaintCompatFlags()
	case "untaint":
		return UntaintCompatFlags()
	case "fmt":
		return FmtCompatFlags()
	case "graph":
		return GraphCompatFlags()
	case "force-unlock":
		return ForceUnlockCompatFlags()
	case "get":
		return GetCompatFlags()
	case "test":
		return TestCompatFlags()
	case "console":
		return ConsoleCompatFlags()
	case "workspace":
		return WorkspaceCompatFlags()
	case "providers":
		return ProvidersCompatFlags()
	default:
		return TerraformCompatFlags()
	}
}
