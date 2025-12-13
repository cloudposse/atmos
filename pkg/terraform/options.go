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
	ProcessingOptions
}

// PlanfileOptions holds the options for generating a Terraform planfile.
type PlanfileOptions struct {
	Component            string
	Stack                string
	Format               string
	File                 string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
}
