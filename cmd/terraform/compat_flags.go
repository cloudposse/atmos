package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// TerraformCompatFlags returns the compatibility flags for terraform commands.
// These flags are NOT parsed by Cobra but are passed through to terraform/tofu.
// They are documented in help output to inform users about available options.
func TerraformCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		// Common terraform flags that should pass through.
		"-var":              {Behavior: compat.AppendToSeparated},
		"-var-file":         {Behavior: compat.AppendToSeparated},
		"-target":           {Behavior: compat.AppendToSeparated},
		"-lock":             {Behavior: compat.AppendToSeparated},
		"-lock-timeout":     {Behavior: compat.AppendToSeparated},
		"-input":            {Behavior: compat.AppendToSeparated},
		"-no-color":         {Behavior: compat.AppendToSeparated},
		"-parallelism":      {Behavior: compat.AppendToSeparated},
		"-refresh":          {Behavior: compat.AppendToSeparated},
		"-compact-warnings": {Behavior: compat.AppendToSeparated},
	}
}

// PlanCompatFlags returns compatibility flags specific to terraform plan.
func PlanCompatFlags() map[string]compat.CompatibilityFlag {
	flags := TerraformCompatFlags()
	// Plan-specific flags.
	flags["-destroy"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-refresh-only"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-replace"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-detailed-exitcode"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-generate-config-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-json"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	return flags
}

// ApplyCompatFlags returns compatibility flags specific to terraform apply.
func ApplyCompatFlags() map[string]compat.CompatibilityFlag {
	flags := TerraformCompatFlags()
	// Apply-specific flags.
	flags["-auto-approve"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-backup"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-destroy"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-refresh-only"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-replace"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-json"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-state"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-state-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	return flags
}

// DestroyCompatFlags returns compatibility flags specific to terraform destroy.
func DestroyCompatFlags() map[string]compat.CompatibilityFlag {
	flags := TerraformCompatFlags()
	// Destroy-specific flags.
	flags["-auto-approve"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-backup"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-json"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-state"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-state-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	return flags
}

// InitCompatFlags returns compatibility flags specific to terraform init.
func InitCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-backend":               {Behavior: compat.AppendToSeparated},
		"-backend-config":        {Behavior: compat.AppendToSeparated},
		"-force-copy":            {Behavior: compat.AppendToSeparated},
		"-from-module":           {Behavior: compat.AppendToSeparated},
		"-get":                   {Behavior: compat.AppendToSeparated},
		"-input":                 {Behavior: compat.AppendToSeparated},
		"-lock":                  {Behavior: compat.AppendToSeparated},
		"-lock-timeout":          {Behavior: compat.AppendToSeparated},
		"-no-color":              {Behavior: compat.AppendToSeparated},
		"-plugin-dir":            {Behavior: compat.AppendToSeparated},
		"-reconfigure":           {Behavior: compat.AppendToSeparated},
		"-migrate-state":         {Behavior: compat.AppendToSeparated},
		"-upgrade":               {Behavior: compat.AppendToSeparated},
		"-lockfile":              {Behavior: compat.AppendToSeparated},
		"-ignore-remote-version": {Behavior: compat.AppendToSeparated},
	}
}

// ValidateCompatFlags returns compatibility flags specific to terraform validate.
func ValidateCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-json":           {Behavior: compat.AppendToSeparated},
		"-no-color":       {Behavior: compat.AppendToSeparated},
		"-no-tests":       {Behavior: compat.AppendToSeparated},
		"-test-directory": {Behavior: compat.AppendToSeparated},
	}
}

// RefreshCompatFlags returns compatibility flags specific to terraform refresh.
func RefreshCompatFlags() map[string]compat.CompatibilityFlag {
	flags := TerraformCompatFlags()
	flags["-backup"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-state"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-state-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	return flags
}

// OutputCompatFlags returns compatibility flags specific to terraform output.
func OutputCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-json":     {Behavior: compat.AppendToSeparated},
		"-raw":      {Behavior: compat.AppendToSeparated},
		"-no-color": {Behavior: compat.AppendToSeparated},
		"-state":    {Behavior: compat.AppendToSeparated},
	}
}

// ShowCompatFlags returns compatibility flags specific to terraform show.
func ShowCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-json":     {Behavior: compat.AppendToSeparated},
		"-no-color": {Behavior: compat.AppendToSeparated},
	}
}

// StateCompatFlags returns compatibility flags specific to terraform state commands.
func StateCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-state":        {Behavior: compat.AppendToSeparated},
		"-state-out":    {Behavior: compat.AppendToSeparated},
		"-backup":       {Behavior: compat.AppendToSeparated},
		"-lock":         {Behavior: compat.AppendToSeparated},
		"-lock-timeout": {Behavior: compat.AppendToSeparated},
	}
}

// ImportCompatFlags returns compatibility flags specific to terraform import.
func ImportCompatFlags() map[string]compat.CompatibilityFlag {
	flags := TerraformCompatFlags()
	flags["-config"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-allow-missing-config"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-state"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	flags["-state-out"] = compat.CompatibilityFlag{Behavior: compat.AppendToSeparated}
	return flags
}

// TaintCompatFlags returns compatibility flags specific to terraform taint.
func TaintCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-allow-missing": {Behavior: compat.AppendToSeparated},
		"-lock":          {Behavior: compat.AppendToSeparated},
		"-lock-timeout":  {Behavior: compat.AppendToSeparated},
		"-state":         {Behavior: compat.AppendToSeparated},
		"-state-out":     {Behavior: compat.AppendToSeparated},
	}
}

// UntaintCompatFlags returns compatibility flags specific to terraform untaint.
func UntaintCompatFlags() map[string]compat.CompatibilityFlag {
	return TaintCompatFlags() // Same flags as taint.
}

// FmtCompatFlags returns compatibility flags specific to terraform fmt.
func FmtCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-list":      {Behavior: compat.AppendToSeparated},
		"-write":     {Behavior: compat.AppendToSeparated},
		"-diff":      {Behavior: compat.AppendToSeparated},
		"-check":     {Behavior: compat.AppendToSeparated},
		"-no-color":  {Behavior: compat.AppendToSeparated},
		"-recursive": {Behavior: compat.AppendToSeparated},
	}
}

// GraphCompatFlags returns compatibility flags specific to terraform graph.
func GraphCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-type":         {Behavior: compat.AppendToSeparated},
		"-draw-cycles":  {Behavior: compat.AppendToSeparated},
		"-module-depth": {Behavior: compat.AppendToSeparated},
		"-plan":         {Behavior: compat.AppendToSeparated},
	}
}

// ForceUnlockCompatFlags returns compatibility flags specific to terraform force-unlock.
func ForceUnlockCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-force": {Behavior: compat.AppendToSeparated},
	}
}

// GetCompatFlags returns compatibility flags specific to terraform get.
func GetCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-update":         {Behavior: compat.AppendToSeparated},
		"-no-color":       {Behavior: compat.AppendToSeparated},
		"-test-directory": {Behavior: compat.AppendToSeparated},
	}
}

// TestCompatFlags returns compatibility flags specific to terraform test.
func TestCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-filter":         {Behavior: compat.AppendToSeparated},
		"-json":           {Behavior: compat.AppendToSeparated},
		"-no-color":       {Behavior: compat.AppendToSeparated},
		"-test-directory": {Behavior: compat.AppendToSeparated},
		"-var":            {Behavior: compat.AppendToSeparated},
		"-var-file":       {Behavior: compat.AppendToSeparated},
		"-verbose":        {Behavior: compat.AppendToSeparated},
	}
}

// ConsoleCompatFlags returns compatibility flags specific to terraform console.
func ConsoleCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-state":    {Behavior: compat.AppendToSeparated},
		"-var":      {Behavior: compat.AppendToSeparated},
		"-var-file": {Behavior: compat.AppendToSeparated},
		"-plan":     {Behavior: compat.AppendToSeparated},
	}
}

// WorkspaceCompatFlags returns compatibility flags specific to terraform workspace commands.
func WorkspaceCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-lock":         {Behavior: compat.AppendToSeparated},
		"-lock-timeout": {Behavior: compat.AppendToSeparated},
		"-state":        {Behavior: compat.AppendToSeparated},
	}
}

// ProvidersCompatFlags returns compatibility flags specific to terraform providers.
func ProvidersCompatFlags() map[string]compat.CompatibilityFlag {
	return map[string]compat.CompatibilityFlag{
		"-test-directory": {Behavior: compat.AppendToSeparated},
	}
}
