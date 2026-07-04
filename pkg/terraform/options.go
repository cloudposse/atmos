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
	// WithSecrets, when true, writes resolved secret values into the generated varfile.
	// By default secrets are omitted from the varfile so plaintext secrets never hit disk.
	WithSecrets bool
	ProcessingOptions
}

// ShellOptions holds options for the terraform shell command.
type ShellOptions struct {
	Component string
	Stack     string
	DryRun    bool
	Identity  string // AWS identity to use for authentication (from --identity flag).
	// WithSecrets, when true, exports secret-bearing variables into the interactive shell
	// as TF_VAR_<name> environment variables. By default they are not exported (and never
	// written to the on-disk varfile), so terraform inside the shell will not see them.
	WithSecrets bool
	// SkipInit, when true, skips `terraform init` before launching the shell (from --skip-init).
	// Workspace selection is unaffected and stays governed by the workspaces_enabled setting.
	SkipInit bool
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
