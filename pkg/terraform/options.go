// Package terraform provides terraform-specific types and utilities for Atmos.
// This package contains shared data structures used by cmd/terraform and internal/exec.
// for terraform operations.
package terraform

// ProcessingOptions holds common options for template and function processing.
// This struct is embedded in command-specific options to avoid repetition.
type ProcessingOptions struct {
	ProcessTemplates bool
	ProcessFunctions bool
	Skip             []string
}

// CleanOptions holds options for the terraform clean command.
type CleanOptions struct {
	Component    string
	Stack        string
	Force        bool
	Everything   bool
	SkipLockFile bool
	DryRun       bool
	Cache        bool // Clean shared plugin cache directory.
}

// GenerateBackendOptions holds options for generating Terraform backend configs.
type GenerateBackendOptions struct {
	Component string
	Stack     string
	ProcessingOptions
}

// VarfileOptions holds options for generating Terraform varfiles.
type VarfileOptions struct {
	Component string
	Stack     string
	File      string
	ProcessingOptions
}

// ShellOptions holds options for the terraform shell command.
type ShellOptions struct {
	Component string
	Stack     string
	DryRun    bool
	Identity  string // AWS identity to use for authentication (from --identity flag).
	ProcessingOptions
}

// ReusePlanMode controls whether `atmos terraform generate planfile` reuses
// the binary planfile produced by a prior `atmos terraform plan` invocation
// instead of running a fresh plan. The zero value is ReusePlanNever, which
// preserves the historical behavior of always running a fresh plan.
type ReusePlanMode string

const (
	// ReusePlanNever always runs a fresh `terraform plan` (default).
	ReusePlanNever ReusePlanMode = "never"

	// ReusePlanAuto reuses the canonical binary planfile when it exists and
	// the staleness gates pass; otherwise falls back to a fresh plan.
	ReusePlanAuto ReusePlanMode = "auto"

	// ReusePlanAlways requires reuse of the canonical binary planfile and
	// returns an error if it is missing or stale.
	ReusePlanAlways ReusePlanMode = "always"
)

// PlanfileOptions holds the options for generating a Terraform planfile.
type PlanfileOptions struct {
	Component            string
	Stack                string
	Format               string
	File                 string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string

	// ReusePlan controls whether to reuse the binary planfile produced by a
	// prior `atmos terraform plan`. The zero value ("") is treated as
	// ReusePlanNever for backward compatibility — existing callers that do
	// not set this field get the original "always replan" behavior.
	ReusePlan ReusePlanMode
}
