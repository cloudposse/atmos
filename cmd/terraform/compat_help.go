package terraform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// CompatFlagDescription provides a description for a compatibility flag.
type CompatFlagDescription struct {
	Flag        string
	Description string
}

// TerraformCompatFlagDescriptions returns descriptions for common terraform compatibility flags.
// These are used to generate help text for terraform commands.
func TerraformCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-var", Description: "Set a value for one of the input variables"},
		{Flag: "-var-file", Description: "Load variable values from the given file"},
		{Flag: "-target", Description: "Target specific resources for planning/applying"},
		{Flag: "-lock", Description: "Lock the state file when locking is supported (default: true)"},
		{Flag: "-lock-timeout", Description: "Duration to retry a state lock (default: 0s)"},
		{Flag: "-input", Description: "Ask for input for variables if not directly set (default: true)"},
		{Flag: "-no-color", Description: "Disable color output in the command output"},
		{Flag: "-parallelism", Description: "Limit the number of concurrent operations (default: 10)"},
		{Flag: "-refresh", Description: "Update state prior to checking for differences (default: true)"},
		{Flag: "-compact-warnings", Description: "Show warnings in a more compact form"},
	}
}

// PlanCompatFlagDescriptions returns descriptions for plan-specific compatibility flags.
func PlanCompatFlagDescriptions() []CompatFlagDescription {
	base := TerraformCompatFlagDescriptions()
	planFlags := []CompatFlagDescription{
		{Flag: "-destroy", Description: "Create a plan to destroy all remote objects"},
		{Flag: "-refresh-only", Description: "Create a plan to update state only (no resource changes)"},
		{Flag: "-replace", Description: "Force replacement of a particular resource instance"},
		{Flag: "-out", Description: "Write the plan to the given path"},
		{Flag: "-detailed-exitcode", Description: "Return detailed exit codes (0=success, 1=error, 2=changes)"},
		{Flag: "-generate-config-out", Description: "Write HCL for resources to import"},
		{Flag: "-json", Description: "Output plan in a machine-readable JSON format"},
	}
	return append(base, planFlags...)
}

// ApplyCompatFlagDescriptions returns descriptions for apply-specific compatibility flags.
func ApplyCompatFlagDescriptions() []CompatFlagDescription {
	base := TerraformCompatFlagDescriptions()
	applyFlags := []CompatFlagDescription{
		{Flag: "-auto-approve", Description: "Skip interactive approval of plan before applying"},
		{Flag: "-backup", Description: "Path to backup the existing state file"},
		{Flag: "-destroy", Description: "Destroy all remote objects managed by the configuration"},
		{Flag: "-refresh-only", Description: "Update state only, no resource changes"},
		{Flag: "-replace", Description: "Force replacement of a particular resource instance"},
		{Flag: "-json", Description: "Output apply results in JSON format"},
		{Flag: "-state", Description: "Path to read and save state"},
		{Flag: "-state-out", Description: "Path to write updated state"},
	}
	return append(base, applyFlags...)
}

// DestroyCompatFlagDescriptions returns descriptions for destroy-specific compatibility flags.
func DestroyCompatFlagDescriptions() []CompatFlagDescription {
	base := TerraformCompatFlagDescriptions()
	destroyFlags := []CompatFlagDescription{
		{Flag: "-auto-approve", Description: "Skip interactive approval before destroying"},
		{Flag: "-backup", Description: "Path to backup the existing state file"},
		{Flag: "-json", Description: "Output destroy results in JSON format"},
		{Flag: "-state", Description: "Path to read and save state"},
		{Flag: "-state-out", Description: "Path to write updated state"},
	}
	return append(base, destroyFlags...)
}

// InitCompatFlagDescriptions returns descriptions for init-specific compatibility flags.
func InitCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-backend", Description: "Configure backend for this configuration (default: true)"},
		{Flag: "-backend-config", Description: "Backend configuration to merge with configuration file"},
		{Flag: "-force-copy", Description: "Suppress prompts about copying state data"},
		{Flag: "-from-module", Description: "Copy contents of the given module into the target directory"},
		{Flag: "-get", Description: "Download any modules for this configuration (default: true)"},
		{Flag: "-input", Description: "Ask for input if necessary (default: true)"},
		{Flag: "-lock", Description: "Lock the state file (default: true)"},
		{Flag: "-lock-timeout", Description: "Duration to retry a state lock"},
		{Flag: "-no-color", Description: "Disable color output"},
		{Flag: "-plugin-dir", Description: "Directory containing plugin binaries"},
		{Flag: "-reconfigure", Description: "Reconfigure backend, ignoring any saved configuration"},
		{Flag: "-migrate-state", Description: "Migrate state to new backend"},
		{Flag: "-upgrade", Description: "Upgrade modules and plugins"},
		{Flag: "-lockfile", Description: "Set dependency lockfile mode"},
		{Flag: "-ignore-remote-version", Description: "Ignore version constraints in remote state"},
	}
}

// ValidateCompatFlagDescriptions returns descriptions for validate-specific compatibility flags.
func ValidateCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-json", Description: "Output validation results in JSON format"},
		{Flag: "-no-color", Description: "Disable color output"},
	}
}

// RefreshCompatFlagDescriptions returns descriptions for refresh-specific compatibility flags.
func RefreshCompatFlagDescriptions() []CompatFlagDescription {
	return TerraformCompatFlagDescriptions()
}

// OutputCompatFlagDescriptions returns descriptions for output-specific compatibility flags.
func OutputCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-json", Description: "Output in JSON format"},
		{Flag: "-raw", Description: "Output raw string value without quotes"},
		{Flag: "-no-color", Description: "Disable color output"},
		{Flag: "-state", Description: "Path to the state file"},
	}
}

// ShowCompatFlagDescriptions returns descriptions for show-specific compatibility flags.
func ShowCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-json", Description: "Output in JSON format"},
		{Flag: "-no-color", Description: "Disable color output"},
	}
}

// StateCompatFlagDescriptions returns descriptions for state-specific compatibility flags.
func StateCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-state", Description: "Path to the state file"},
		{Flag: "-lock", Description: "Lock the state file"},
		{Flag: "-lock-timeout", Description: "Duration to retry state lock"},
		{Flag: "-backup", Description: "Path to backup state file"},
	}
}

// ImportCompatFlagDescriptions returns descriptions for import-specific compatibility flags.
func ImportCompatFlagDescriptions() []CompatFlagDescription {
	base := TerraformCompatFlagDescriptions()
	importFlags := []CompatFlagDescription{
		{Flag: "-config", Description: "Path to directory of Terraform configuration files"},
		{Flag: "-allow-missing-config", Description: "Allow import when no resource configuration block exists"},
	}
	return append(base, importFlags...)
}

// TaintCompatFlagDescriptions returns descriptions for taint-specific compatibility flags.
func TaintCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-allow-missing", Description: "Succeed even if the resource is missing"},
		{Flag: "-lock", Description: "Lock the state file"},
		{Flag: "-lock-timeout", Description: "Duration to retry state lock"},
		{Flag: "-state", Description: "Path to the state file"},
		{Flag: "-state-out", Description: "Path to write updated state"},
	}
}

// UntaintCompatFlagDescriptions returns descriptions for untaint-specific compatibility flags.
func UntaintCompatFlagDescriptions() []CompatFlagDescription {
	return TaintCompatFlagDescriptions()
}

// FmtCompatFlagDescriptions returns descriptions for fmt-specific compatibility flags.
func FmtCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-list", Description: "List files with formatting differences (default: true)"},
		{Flag: "-write", Description: "Write formatted files (default: true)"},
		{Flag: "-diff", Description: "Display differences"},
		{Flag: "-check", Description: "Return non-zero exit code if formatting needed"},
		{Flag: "-recursive", Description: "Process files in subdirectories"},
		{Flag: "-no-color", Description: "Disable color output"},
	}
}

// GraphCompatFlagDescriptions returns descriptions for graph-specific compatibility flags.
func GraphCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-plan", Description: "Use the given plan file"},
		{Flag: "-draw-cycles", Description: "Highlight cycles in the graph"},
		{Flag: "-type", Description: "Type of graph to output (plan, plan-refresh-only, plan-destroy, apply)"},
		{Flag: "-module-depth", Description: "Depth of modules to show in output"},
	}
}

// ForceUnlockCompatFlagDescriptions returns descriptions for force-unlock-specific compatibility flags.
func ForceUnlockCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-force", Description: "Don't ask for confirmation"},
	}
}

// GetCompatFlagDescriptions returns descriptions for get-specific compatibility flags.
func GetCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-update", Description: "Check for and download updated modules"},
		{Flag: "-no-color", Description: "Disable color output"},
	}
}

// TestCompatFlagDescriptions returns descriptions for test-specific compatibility flags.
func TestCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-filter", Description: "Filter test files to run"},
		{Flag: "-json", Description: "Output results in JSON format"},
		{Flag: "-no-color", Description: "Disable color output"},
		{Flag: "-test-directory", Description: "Directory containing test files"},
		{Flag: "-verbose", Description: "Print the plan for each test"},
	}
}

// ConsoleCompatFlagDescriptions returns descriptions for console-specific compatibility flags.
func ConsoleCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-state", Description: "Path to the state file"},
		{Flag: "-plan", Description: "Use the given plan file"},
		{Flag: "-var", Description: "Set a variable in the console"},
		{Flag: "-var-file", Description: "Load variable values from the given file"},
	}
}

// WorkspaceCompatFlagDescriptions returns descriptions for workspace-specific compatibility flags.
func WorkspaceCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{
		{Flag: "-lock", Description: "Lock the state file"},
		{Flag: "-lock-timeout", Description: "Duration to retry state lock"},
		{Flag: "-state", Description: "Path to the state file"},
	}
}

// ProvidersCompatFlagDescriptions returns descriptions for providers-specific compatibility flags.
// Note: terraform providers has no special flags beyond standard terraform flags.
func ProvidersCompatFlagDescriptions() []CompatFlagDescription {
	return []CompatFlagDescription{}
}

// FormatCompatFlagsHelp formats compatibility flag descriptions for help output.
func FormatCompatFlagsHelp(descriptions []CompatFlagDescription) string {
	if len(descriptions) == 0 {
		return ""
	}

	// Sort by flag name.
	sorted := make([]CompatFlagDescription, len(descriptions))
	copy(sorted, descriptions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Flag < sorted[j].Flag
	})

	var builder strings.Builder
	builder.WriteString("\nTerraform/OpenTofu Native Flags:\n")
	builder.WriteString("  These flags are passed through to the underlying terraform/tofu command.\n\n")

	// Find maximum flag length for alignment.
	maxLen := 0
	for _, desc := range sorted {
		if len(desc.Flag) > maxLen {
			maxLen = len(desc.Flag)
		}
	}

	// Format each flag.
	for _, desc := range sorted {
		padding := strings.Repeat(" ", maxLen-len(desc.Flag)+2)
		builder.WriteString(fmt.Sprintf("      %s%s%s\n", desc.Flag, padding, desc.Description))
	}

	return builder.String()
}

// GetCompatFlagsForCommand returns the compatibility flags for a specific terraform subcommand.
func GetCompatFlagsForCommand(subCommand string) map[string]compat.CompatibilityFlag {
	switch subCommand {
	case "plan":
		return PlanCompatFlags()
	case "apply":
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
