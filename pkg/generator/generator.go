// Package generator provides a unified interface for generating Terraform configuration files.
// This includes varfiles, provider overrides, required_providers blocks, and backend configuration.
package generator

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Format represents the output format for generated files.
type Format string

const (
	// FormatJSON generates JSON output (.tf.json files).
	FormatJSON Format = "json"
	// FormatHCL generates HCL output (.tf files).
	FormatHCL Format = "hcl"
)

// Generator is the interface for Terraform file generators.
type Generator interface {
	// Name returns the unique identifier for this generator.
	Name() string

	// Generate produces the Terraform configuration content.
	// Returns a map structure suitable for JSON/HCL serialization.
	Generate(ctx context.Context, genCtx *GeneratorContext) (map[string]any, error)

	// Validate checks if the generator context has sufficient data.
	Validate(genCtx *GeneratorContext) error

	// DefaultFilename returns the default output filename.
	DefaultFilename() string

	// ShouldGenerate returns true if this generator should run.
	// Based on whether relevant config exists.
	ShouldGenerate(genCtx *GeneratorContext) bool
}

// GeneratorContext provides component and stack context to generators.
type GeneratorContext struct {
	// AtmosConfig holds the Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration

	// StackInfo holds the processed stack and component information.
	StackInfo *schema.ConfigAndStacksInfo

	// Component is the component name.
	Component string

	// Stack is the stack name.
	Stack string

	// ComponentPath is the path to the component directory.
	ComponentPath string

	// WorkingDir is the directory where generated files will be written.
	WorkingDir string

	// VarsSection contains the component variables.
	VarsSection map[string]any

	// ProvidersSection contains the provider configuration.
	ProvidersSection map[string]any

	// RequiredVersion is the Terraform version constraint (e.g., ">= 1.10.1").
	RequiredVersion string

	// RequiredProviders maps provider names to their configuration.
	// Example: {"aws": {"source": "hashicorp/aws", "version": "~> 5.0"}}.
	RequiredProviders map[string]map[string]any

	// BackendType is the Terraform backend type (e.g., "s3", "gcs").
	BackendType string

	// BackendConfig contains the backend configuration.
	BackendConfig map[string]any

	// DryRun when true, prevents file writes.
	DryRun bool

	// Format specifies the output format (JSON or HCL).
	Format Format
}

// NewGeneratorContext creates a GeneratorContext from ConfigAndStacksInfo.
func NewGeneratorContext(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	workingDir string,
) *GeneratorContext {
	return &GeneratorContext{
		AtmosConfig:       atmosConfig,
		StackInfo:         info,
		Component:         info.ComponentFromArg,
		Stack:             info.Stack,
		ComponentPath:     info.ComponentFolderPrefix,
		WorkingDir:        workingDir,
		VarsSection:       info.ComponentVarsSection,
		ProvidersSection:  info.ComponentProvidersSection,
		BackendType:       info.ComponentBackendType,
		BackendConfig:     info.ComponentBackendSection,
		DryRun:            info.DryRun,
		Format:            FormatJSON,
		RequiredVersion:   info.RequiredVersion,
		RequiredProviders: info.RequiredProviders,
	}
}

// NewGeneratorContextWithOptions creates a GeneratorContext with functional options.
func NewGeneratorContextWithOptions(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	workingDir string,
	opts ...Option,
) *GeneratorContext {
	ctx := NewGeneratorContext(atmosConfig, info, workingDir)
	ApplyOptions(ctx, opts...)
	return ctx
}
