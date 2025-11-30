package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformGlobalCompatFlags returns TRUE global terraform flags.
// These can be used before any subcommand (e.g., `terraform -chdir=./foo plan`).
// These flags are NOT parsed by Cobra but are passed through to terraform/tofu.
func TerraformGlobalCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.TerraformGlobalCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-chdir":   {Behavior: compat.AppendToSeparated, Description: "Switch to a different working directory before executing the given subcommand"},
		"-help":    {Behavior: compat.AppendToSeparated, Description: "Show terraform help output"},
		"-version": {Behavior: compat.AppendToSeparated, Description: "Show terraform version"},
	}
}

// CommonSubcommandFlags returns flags that are common across many terraform subcommands.
// These are NOT global terraform options - they only apply to specific subcommands like plan, apply, destroy.
// These flags are NOT parsed by Cobra but are passed through to terraform/tofu.
func CommonSubcommandFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.CommonSubcommandFlags")()

	return map[string]compat.CompatibilityFlag{
		"-var":              {Behavior: compat.AppendToSeparated, Description: "Set a value for one of the input variables"},
		"-var-file":         {Behavior: compat.AppendToSeparated, Description: "Load variable values from the given file"},
		"-target":           {Behavior: compat.AppendToSeparated, Description: "Target specific resources for planning/applying"},
		"-lock":             {Behavior: compat.AppendToSeparated, Description: "Lock the state file when locking is supported (default: true)"},
		"-lock-timeout":     {Behavior: compat.AppendToSeparated, Description: "Duration to retry a state lock (default: 0s)"},
		"-input":            {Behavior: compat.AppendToSeparated, Description: "Ask for input for variables if not directly set (default: true)"},
		"-no-color":         {Behavior: compat.AppendToSeparated, Description: "Disable color output in the command output"},
		"-parallelism":      {Behavior: compat.AppendToSeparated, Description: "Limit the number of concurrent operations (default: 10)"},
		"-refresh":          {Behavior: compat.AppendToSeparated, Description: "Update state prior to checking for differences (default: true)"},
		"-compact-warnings": {Behavior: compat.AppendToSeparated, Description: "Show warnings in a more compact form"},
	}
}

// PlanCompatFlags returns compatibility flags specific to terraform plan.
func PlanCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.PlanCompatFlags")()

	flags := CommonSubcommandFlags()
	// Plan-specific flags.
	flags["-destroy"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Create a plan to destroy all remote objects"}
	flags["-refresh-only"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Create a plan to update state only (no resource changes)"}
	flags["-replace"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Force replacement of a particular resource instance"}
	flags["-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Write the plan to the given path"}
	flags["-detailed-exitcode"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Return detailed exit codes (0=success, 1=error, 2=changes)"}
	flags["-generate-config-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Write HCL for resources to import"}
	flags["-json"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Output plan in a machine-readable JSON format"}
	return flags
}

// ApplyCompatFlags returns compatibility flags specific to terraform apply.
func ApplyCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.ApplyCompatFlags")()

	flags := CommonSubcommandFlags()
	// Apply-specific flags.
	flags["-auto-approve"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Skip interactive approval of plan before applying"}
	flags["-backup"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to backup the existing state file"}
	flags["-destroy"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Destroy all remote objects managed by the configuration"}
	flags["-refresh-only"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Update state only, no resource changes"}
	flags["-replace"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Force replacement of a particular resource instance"}
	flags["-json"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Output apply results in JSON format"}
	flags["-state"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to read and save state"}
	flags["-state-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to write updated state"}
	return flags
}

// DestroyCompatFlags returns compatibility flags specific to terraform destroy.
func DestroyCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.DestroyCompatFlags")()

	flags := CommonSubcommandFlags()
	// Destroy-specific flags.
	flags["-auto-approve"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Skip interactive approval before destroying"}
	flags["-backup"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to backup the existing state file"}
	flags["-json"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Output destroy results in JSON format"}
	flags["-state"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to read and save state"}
	flags["-state-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to write updated state"}
	return flags
}

// InitCompatFlags returns compatibility flags specific to terraform init.
func InitCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.InitCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-backend":               {Behavior: compat.AppendToSeparated, Description: "Configure backend for this configuration (default: true)"},
		"-backend-config":        {Behavior: compat.AppendToSeparated, Description: "Backend configuration to merge with configuration file"},
		"-force-copy":            {Behavior: compat.AppendToSeparated, Description: "Suppress prompts about copying state data"},
		"-from-module":           {Behavior: compat.AppendToSeparated, Description: "Copy contents of the given module into the target directory"},
		"-get":                   {Behavior: compat.AppendToSeparated, Description: "Download any modules for this configuration (default: true)"},
		"-input":                 {Behavior: compat.AppendToSeparated, Description: "Ask for input if necessary (default: true)"},
		"-lock":                  {Behavior: compat.AppendToSeparated, Description: "Lock the state file (default: true)"},
		"-lock-timeout":          {Behavior: compat.AppendToSeparated, Description: "Duration to retry a state lock"},
		"-no-color":              {Behavior: compat.AppendToSeparated, Description: "Disable color output"},
		"-plugin-dir":            {Behavior: compat.AppendToSeparated, Description: "Directory containing plugin binaries"},
		"-reconfigure":           {Behavior: compat.AppendToSeparated, Description: "Reconfigure backend, ignoring any saved configuration"},
		"-migrate-state":         {Behavior: compat.AppendToSeparated, Description: "Migrate state to new backend"},
		"-upgrade":               {Behavior: compat.AppendToSeparated, Description: "Upgrade modules and plugins"},
		"-lockfile":              {Behavior: compat.AppendToSeparated, Description: "Set dependency lockfile mode"},
		"-ignore-remote-version": {Behavior: compat.AppendToSeparated, Description: "Ignore version constraints in remote state"},
	}
}

// ValidateCompatFlags returns compatibility flags specific to terraform validate.
func ValidateCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.ValidateCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-json":           {Behavior: compat.AppendToSeparated, Description: "Output validation results in JSON format"},
		"-no-color":       {Behavior: compat.AppendToSeparated, Description: "Disable color output"},
		"-no-tests":       {Behavior: compat.AppendToSeparated, Description: "Skip test file validation"},
		"-test-directory": {Behavior: compat.AppendToSeparated, Description: "Directory containing test files"},
	}
}

// RefreshCompatFlags returns compatibility flags specific to terraform refresh.
func RefreshCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.RefreshCompatFlags")()

	flags := CommonSubcommandFlags()
	flags["-backup"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to backup the existing state file"}
	flags["-state"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to read and save state"}
	flags["-state-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to write updated state"}
	return flags
}

// OutputCompatFlags returns compatibility flags specific to terraform output.
func OutputCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.OutputCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-json":     {Behavior: compat.AppendToSeparated, Description: "Output in JSON format"},
		"-raw":      {Behavior: compat.AppendToSeparated, Description: "Output raw string value without quotes"},
		"-no-color": {Behavior: compat.AppendToSeparated, Description: "Disable color output"},
		"-state":    {Behavior: compat.AppendToSeparated, Description: "Path to the state file"},
	}
}

// ShowCompatFlags returns compatibility flags specific to terraform show.
func ShowCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.ShowCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-json":     {Behavior: compat.AppendToSeparated, Description: "Output in JSON format"},
		"-no-color": {Behavior: compat.AppendToSeparated, Description: "Disable color output"},
	}
}

// StateCompatFlags returns compatibility flags specific to terraform state commands.
func StateCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.StateCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-state":        {Behavior: compat.AppendToSeparated, Description: "Path to the state file"},
		"-state-out":    {Behavior: compat.AppendToSeparated, Description: "Path to write updated state"},
		"-backup":       {Behavior: compat.AppendToSeparated, Description: "Path to backup state file"},
		"-lock":         {Behavior: compat.AppendToSeparated, Description: "Lock the state file"},
		"-lock-timeout": {Behavior: compat.AppendToSeparated, Description: "Duration to retry state lock"},
	}
}

// ImportCompatFlags returns compatibility flags specific to terraform import.
func ImportCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.ImportCompatFlags")()

	flags := CommonSubcommandFlags()
	flags["-config"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to directory of Terraform configuration files"}
	flags["-allow-missing-config"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Allow import when no resource configuration block exists"}
	flags["-state"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to read and save state"}
	flags["-state-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated, Description: "Path to write updated state"}
	return flags
}

// TaintCompatFlags returns compatibility flags specific to terraform taint.
func TaintCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.TaintCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-allow-missing": {Behavior: compat.AppendToSeparated, Description: "Succeed even if the resource is missing"},
		"-lock":          {Behavior: compat.AppendToSeparated, Description: "Lock the state file"},
		"-lock-timeout":  {Behavior: compat.AppendToSeparated, Description: "Duration to retry state lock"},
		"-state":         {Behavior: compat.AppendToSeparated, Description: "Path to the state file"},
		"-state-out":     {Behavior: compat.AppendToSeparated, Description: "Path to write updated state"},
	}
}

// UntaintCompatFlags returns compatibility flags specific to terraform untaint.
func UntaintCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.UntaintCompatFlags")()

	return TaintCompatFlags() // Same flags as taint.
}

// FmtCompatFlags returns compatibility flags specific to terraform fmt.
func FmtCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.FmtCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-list":      {Behavior: compat.AppendToSeparated, Description: "List files with formatting differences (default: true)"},
		"-write":     {Behavior: compat.AppendToSeparated, Description: "Write formatted files (default: true)"},
		"-diff":      {Behavior: compat.AppendToSeparated, Description: "Display differences"},
		"-check":     {Behavior: compat.AppendToSeparated, Description: "Return non-zero exit code if formatting needed"},
		"-no-color":  {Behavior: compat.AppendToSeparated, Description: "Disable color output"},
		"-recursive": {Behavior: compat.AppendToSeparated, Description: "Process files in subdirectories"},
	}
}

// GraphCompatFlags returns compatibility flags specific to terraform graph.
func GraphCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.GraphCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-type":         {Behavior: compat.AppendToSeparated, Description: "Type of graph to output (plan, plan-refresh-only, plan-destroy, apply)"},
		"-draw-cycles":  {Behavior: compat.AppendToSeparated, Description: "Highlight cycles in the graph"},
		"-module-depth": {Behavior: compat.AppendToSeparated, Description: "Depth of modules to show in output"},
		"-plan":         {Behavior: compat.AppendToSeparated, Description: "Use the given plan file"},
	}
}

// ForceUnlockCompatFlags returns compatibility flags specific to terraform force-unlock.
func ForceUnlockCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.ForceUnlockCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-force": {Behavior: compat.AppendToSeparated, Description: "Don't ask for confirmation"},
	}
}

// GetCompatFlags returns compatibility flags specific to terraform get.
func GetCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.GetCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-update":         {Behavior: compat.AppendToSeparated, Description: "Check for and download updated modules"},
		"-no-color":       {Behavior: compat.AppendToSeparated, Description: "Disable color output"},
		"-test-directory": {Behavior: compat.AppendToSeparated, Description: "Directory containing test files"},
	}
}

// TestCompatFlags returns compatibility flags specific to terraform test.
func TestCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.TestCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-filter":         {Behavior: compat.AppendToSeparated, Description: "Filter test files to run"},
		"-json":           {Behavior: compat.AppendToSeparated, Description: "Output results in JSON format"},
		"-no-color":       {Behavior: compat.AppendToSeparated, Description: "Disable color output"},
		"-test-directory": {Behavior: compat.AppendToSeparated, Description: "Directory containing test files"},
		"-var":            {Behavior: compat.AppendToSeparated, Description: "Set a variable in the test"},
		"-var-file":       {Behavior: compat.AppendToSeparated, Description: "Load variable values from the given file"},
		"-verbose":        {Behavior: compat.AppendToSeparated, Description: "Print the plan for each test"},
	}
}

// ConsoleCompatFlags returns compatibility flags specific to terraform console.
func ConsoleCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.ConsoleCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-state":    {Behavior: compat.AppendToSeparated, Description: "Path to the state file"},
		"-var":      {Behavior: compat.AppendToSeparated, Description: "Set a variable in the console"},
		"-var-file": {Behavior: compat.AppendToSeparated, Description: "Load variable values from the given file"},
		"-plan":     {Behavior: compat.AppendToSeparated, Description: "Use the given plan file"},
	}
}

// WorkspaceCompatFlags returns compatibility flags specific to terraform workspace commands.
func WorkspaceCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.WorkspaceCompatFlags")()

	return map[string]compat.CompatibilityFlag{
		"-lock":         {Behavior: compat.AppendToSeparated, Description: "Lock the state file"},
		"-lock-timeout": {Behavior: compat.AppendToSeparated, Description: "Duration to retry state lock"},
		"-state":        {Behavior: compat.AppendToSeparated, Description: "Path to the state file"},
	}
}

// ProvidersCompatFlags returns compatibility flags specific to terraform providers.
// Note: terraform providers has no special flags beyond standard terraform flags.
func ProvidersCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.ProvidersCompatFlags")()

	return map[string]compat.CompatibilityFlag{}
}

// AllTerraformCompatFlags returns a combined set of all terraform compatibility flags.
// This is used for preprocessing in Execute() to identify terraform pass-through flags
// before Cobra parses the command line. By combining all possible flags, we can
// correctly separate pass-through flags regardless of which subcommand is being called.
func AllTerraformCompatFlags() map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "terraform.AllTerraformCompatFlags")()

	flags := make(map[string]compat.CompatibilityFlag)

	// Merge all subcommand-specific flags.
	mergeFlags := func(src map[string]compat.CompatibilityFlag) {
		for k, v := range src {
			flags[k] = v
		}
	}

	// Include global terraform flags (-chdir, -help, -version).
	mergeFlags(TerraformGlobalCompatFlags())
	mergeFlags(CommonSubcommandFlags())
	mergeFlags(PlanCompatFlags())
	mergeFlags(ApplyCompatFlags())
	mergeFlags(DestroyCompatFlags())
	mergeFlags(InitCompatFlags())
	mergeFlags(ValidateCompatFlags())
	mergeFlags(RefreshCompatFlags())
	mergeFlags(OutputCompatFlags())
	mergeFlags(ShowCompatFlags())
	mergeFlags(StateCompatFlags())
	mergeFlags(ImportCompatFlags())
	mergeFlags(TaintCompatFlags())
	mergeFlags(UntaintCompatFlags())
	mergeFlags(FmtCompatFlags())
	mergeFlags(GraphCompatFlags())
	mergeFlags(ForceUnlockCompatFlags())
	mergeFlags(GetCompatFlags())
	mergeFlags(TestCompatFlags())
	mergeFlags(ConsoleCompatFlags())
	mergeFlags(WorkspaceCompatFlags())
	mergeFlags(ProvidersCompatFlags())

	return flags
}
